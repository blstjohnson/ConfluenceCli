package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"confcli/pkg/models"
)

// GetRootSnapshot reads authoritative content/label data without CQL indexing
// or one request per child. The cache lives for one client (one sync plan).
func (c *client) GetRootSnapshot(ctx context.Context, space string, rootID int) ([]models.Page, error) {
	key := fmt.Sprintf("%s/%d", space, rootID)
	if pages, ok := c.rootSnapshots[key]; ok {
		return pages, nil
	}
	root, err := c.GetPageWithExpansions(ctx, rootID, []string{"space"})
	if err != nil {
		return nil, err
	}
	if root == nil || root.Space.Key != space {
		return nil, fmt.Errorf("root %d is outside space %q", rootID, space)
	}
	type labels struct {
		Results []models.Label    `json:"results"`
		Size    int               `json:"size"`
		Links   map[string]string `json:"_links"`
	}
	type entry struct {
		models.Page
		Metadata struct {
			Labels *labels `json:"labels"`
		} `json:"metadata"`
	}
	pages := []models.Page{}
	for start := 0; ; {
		params := url.Values{"spaceKey": {space}, "type": {"page"}, "start": {strconv.Itoa(start)}, "limit": {"100"}, "expand": {"space,ancestors,version,metadata.labels"}}
		response, err := c.httpClient.MakeRequest(ctx, http.MethodGet, c.httpClient.APIPrefix+"/content", params, nil)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, fmt.Errorf("root snapshot failed with HTTP %d", response.StatusCode)
		}
		var result struct {
			Results []entry           `json:"results"`
			Links   map[string]string `json:"_links"`
		}
		err = json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, item := range result.Results {
			page := item.Page
			if page.Space.Key != space {
				return nil, fmt.Errorf("snapshot page %v is outside space %q", page.ID, space)
			}
			inside := false
			for _, ancestor := range page.Ancestors {
				if id, ok := ancestor.ID.Int(); ok && id == rootID {
					inside = true
					break
				}
			}
			if !inside {
				continue
			}
			if item.Metadata.Labels == nil {
				return nil, fmt.Errorf("snapshot page %v has no expanded labels", page.ID)
			}
			page.Labels = item.Metadata.Labels.Results
			if item.Metadata.Labels.Links["next"] != "" || item.Metadata.Labels.Size > len(page.Labels) {
				id, ok := page.ID.Int()
				if !ok {
					return nil, fmt.Errorf("invalid snapshot page ID")
				}
				page.Labels, err = c.GetLabels(ctx, id)
				if err != nil {
					return nil, err
				}
			}
			pages = append(pages, page)
		}
		if result.Links["next"] == "" {
			break
		}
		if len(result.Results) == 0 {
			return nil, fmt.Errorf("root snapshot pagination did not advance")
		}
		start += len(result.Results)
	}
	if c.rootSnapshots == nil {
		c.rootSnapshots = map[string][]models.Page{}
	}
	c.rootSnapshots[key] = pages
	return pages, nil
}
