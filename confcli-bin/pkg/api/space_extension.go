package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"confcli/pkg/models"
)

// SpaceExtension provides higher-level space operations built on top of HTTPClient
type SpaceExtension struct {
	client *HTTPClient
}

// NewSpaceExtension creates a new space extension
func NewSpaceExtension(client *HTTPClient) *SpaceExtension {
	return &SpaceExtension{client: client}
}

// FetchSpaceRequest represents a request to fetch a space
type FetchSpaceRequest struct {
	SpaceKey string
}

// FetchSpaceResponse represents the response from fetching a space
type FetchSpaceResponse struct {
	Space *models.Space
}

// FetchSpace fetches a space by its key
func (e *SpaceExtension) FetchSpace(ctx context.Context, req *FetchSpaceRequest) (*FetchSpaceResponse, error) {
	path := fmt.Sprintf("%s/space/%s", e.client.APIPrefix, req.SpaceKey)
	resp, err := e.client.MakeRequest(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var space models.Space
	if err := json.NewDecoder(resp.Body).Decode(&space); err != nil {
		return nil, err
	}

	return &FetchSpaceResponse{Space: &space}, nil
}

// FetchAllPagesInSpaceRequest represents a request to fetch all pages in a space
type FetchAllPagesInSpaceRequest struct {
	SpaceKey string
	Start    int
	Limit    int
	Expansions []string // Optional: expansions to fetch for each page (e.g., "body.storage", "version")
}

// FailedPage records a page that could not be fetched during a batch operation
type FailedPage struct {
	PageID int
	Title  string
	Err    error
}

// FetchAllPagesInSpaceResponse represents the response from fetching all pages in a space
type FetchAllPagesInSpaceResponse struct {
	Pages       []models.Page
	FailedPages []FailedPage
	Start       int
	Limit       int
	Size        int
	HasMore     bool
}

// FetchAllPagesInSpace fetches all pages in a space with pagination
// It first fetches metadata for all pages, then fetches full content for each page
func (e *SpaceExtension) FetchAllPagesInSpace(ctx context.Context, req *FetchAllPagesInSpaceRequest) (*FetchAllPagesInSpaceResponse, error) {
	params := url.Values{}
	params.Add("start", fmt.Sprintf("%d", req.Start))
	params.Add("limit", fmt.Sprintf("%d", req.Limit))

	// Use the correct endpoint: /rest/api/space/{spaceKey}/content
	path := fmt.Sprintf("%s/space/%s/content", e.client.APIPrefix, req.SpaceKey)
	resp, err := e.client.MakeRequest(ctx, "GET", path, params, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// The API returns: {"page":{"results":[...],"start":0,"limit":100,"size":100}}
	var result struct {
		Page *struct {
			Results []struct {
				ID    interface{} `json:"id"`
				Type  string      `json:"type"`
				Title string      `json:"title"`
				Status string     `json:"status"`
			} `json:"results"`
			Start int `json:"start"`
			Limit int `json:"limit"`
			Size  int `json:"size"`
		} `json:"page"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Page == nil {
		return &FetchAllPagesInSpaceResponse{
			Pages:   []models.Page{},
			Start:   0,
			Limit:   req.Limit,
			Size:    0,
			HasMore: false,
		}, nil
	}

	// Fetch full page content for each page using the page extension
	pageExtension := NewPageExtension(e.client)
	pages := make([]models.Page, 0, len(result.Page.Results))
	var failedPages []FailedPage

	for _, pageMeta := range result.Page.Results {
		// Convert ID to int if possible
		var pageID int
		switch id := pageMeta.ID.(type) {
		case float64:
			pageID = int(id)
		case int:
			pageID = id
		case string:
			// Try to parse string ID as int
			fmt.Sscanf(id, "%d", &pageID)
		}

		if pageID == 0 {
			continue // Skip pages with invalid IDs
		}

		// Fetch full page content using existing page function
		fetchReq := &FetchPageRequest{
			PageID:     pageID,
			Expansions: req.Expansions,
		}
		fetchResp, err := pageExtension.FetchPage(ctx, fetchReq)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch page %d (%s), skipping: %v\n", pageID, pageMeta.Title, err)
			failedPages = append(failedPages, FailedPage{PageID: pageID, Title: pageMeta.Title, Err: err})
			continue
		}

		pages = append(pages, *fetchResp.Page)
	}

	hasMore := result.Page.Size == req.Limit

	return &FetchAllPagesInSpaceResponse{
		Pages:       pages,
		FailedPages: failedPages,
		Start:       result.Page.Start,
		Limit:       result.Page.Limit,
		Size:        result.Page.Size,
		HasMore:     hasMore,
	}, nil
}

// FetchRootPagesInSpaceRequest represents a request to fetch root pages in a space
type FetchRootPagesInSpaceRequest struct {
	SpaceKey   string
	Expansions []string // Optional: expansions to fetch for each page
}

// FetchRootPagesInSpaceResponse represents the response from fetching root pages
type FetchRootPagesInSpaceResponse struct {
	Pages []models.Page
}

// FetchRootPagesInSpace fetches root-level pages in a space
func (e *SpaceExtension) FetchRootPagesInSpace(ctx context.Context, req *FetchRootPagesInSpaceRequest) (*FetchRootPagesInSpaceResponse, error) {
	// For now, return all pages in the space
	// A more sophisticated implementation would filter to only root-level pages
	allPagesResp, err := e.FetchAllPagesInSpace(ctx, &FetchAllPagesInSpaceRequest{
		SpaceKey:   req.SpaceKey,
		Start:      0,
		Limit:      100,
		Expansions: req.Expansions,
	})
	if err != nil {
		return nil, err
	}

	// Filter to only include pages that are direct children of the space homepage
	// or have no ancestors (truly root-level)
	rootPages := make([]models.Page, 0)
	for _, page := range allPagesResp.Pages {
		if len(page.Ancestors) == 0 {
			rootPages = append(rootPages, page)
		}
	}

	return &FetchRootPagesInSpaceResponse{Pages: rootPages}, nil
}
