package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"confcli/pkg/models"
)

const scrollVersionsAPIPrefix = "/rest/scroll-versions/1.0"

// ScrollVersionsClient wraps the low-level HTTPClient and provides
// typed helpers for the Scroll Versions REST API.
type ScrollVersionsClient struct {
	http *HTTPClient
}

// NewScrollVersionsClient creates a ScrollVersionsClient from an existing HTTPClient.
func NewScrollVersionsClient(httpClient *HTTPClient) *ScrollVersionsClient {
	return &ScrollVersionsClient{http: httpClient}
}

// GetConfig probes the Scroll Versions configuration for a space.
// Returns nil (no error) when the plugin is not installed (404) or the
// caller lacks AdministerSpace/ManageContent permission (403). The caller
// cannot use SV-specific features in either case, so the two are
// indistinguishable from the user's perspective.
func (s *ScrollVersionsClient) GetConfig(ctx context.Context, spaceKey string) (*models.ScrollVersionsConfig, error) {
	path := fmt.Sprintf("%s/config/%s", scrollVersionsAPIPrefix, spaceKey)
	resp, err := s.http.MakeRequest(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("scroll versions config request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("scroll versions config: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var cfg models.ScrollVersionsConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode scroll versions config: %w", err)
	}
	return &cfg, nil
}

// GetVersions returns all defined versions for a space. Returns an empty
// slice (no error) when the plugin is not installed (404) or the caller
// lacks the required permissions (403), mirroring GetConfig.
func (s *ScrollVersionsClient) GetVersions(ctx context.Context, spaceKey string) ([]models.ScrollVersion, error) {
	path := fmt.Sprintf("%s/versions/%s", scrollVersionsAPIPrefix, spaceKey)
	resp, err := s.http.MakeRequest(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("scroll versions list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("scroll versions list: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var versions []models.ScrollVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("failed to decode scroll versions: %w", err)
	}
	return versions, nil
}

// GetPageTree returns the page tree for a space filtered by version.
// parentScrollPageId and parentPageId can be empty to get root-level pages.
func (s *ScrollVersionsClient) GetPageTree(ctx context.Context, spaceKey, versionID string) ([]models.ScrollPageTreeNode, error) {
	path := fmt.Sprintf("%s/pagetree/%s", scrollVersionsAPIPrefix, spaceKey)
	params := url.Values{}
	params.Set("versionId", versionID)
	// Request top-level pages by leaving parent params empty
	params.Set("parentScrollPageId", "")
	params.Set("parentPageId", "")
	params.Set("expandedScrollPageId", "")
	params.Set("expandedConfluencePageId", "")
	params.Set("isShowToplevelPages", "true")

	resp, err := s.http.MakeRequest(ctx, "GET", path, params, nil)
	if err != nil {
		return nil, fmt.Errorf("scroll versions pagetree request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("scroll versions pagetree: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	// The API can return a single node or an array depending on the request.
	// When requesting top-level with isShowToplevelPages=true, it typically
	// returns a single root container (id=0, type="container", no
	// scrollPageId) whose children are the actual top-level pages.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read pagetree response: %w", err)
	}

	var singleNode models.ScrollPageTreeNode
	if err := json.Unmarshal(bodyBytes, &singleNode); err == nil {
		// Root container wrapper: unwrap to its children.
		if singleNode.ScrollPageID == "" && len(singleNode.Children) > 0 {
			return singleNode.Children, nil
		}
		if singleNode.ScrollPageID != "" {
			return []models.ScrollPageTreeNode{singleNode}, nil
		}
	}

	var nodes []models.ScrollPageTreeNode
	if err := json.Unmarshal(bodyBytes, &nodes); err != nil {
		return nil, fmt.Errorf("failed to decode pagetree response: %w", err)
	}
	return nodes, nil
}

// GetPageTreeChildren returns the children of a specific page in the version tree.
func (s *ScrollVersionsClient) GetPageTreeChildren(ctx context.Context, spaceKey, versionID, parentScrollPageID string, parentPageID int64) ([]models.ScrollPageTreeNode, error) {
	path := fmt.Sprintf("%s/pagetree/%s", scrollVersionsAPIPrefix, spaceKey)
	params := url.Values{}
	params.Set("versionId", versionID)
	params.Set("parentScrollPageId", parentScrollPageID)
	params.Set("parentPageId", fmt.Sprintf("%d", parentPageID))
	params.Set("expandedScrollPageId", "")
	params.Set("expandedConfluencePageId", "")

	resp, err := s.http.MakeRequest(ctx, "GET", path, params, nil)
	if err != nil {
		return nil, fmt.Errorf("scroll versions pagetree children request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("scroll versions pagetree children: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read pagetree children response: %w", err)
	}

	// Try as single node first
	var singleNode models.ScrollPageTreeNode
	if err := json.Unmarshal(bodyBytes, &singleNode); err == nil && singleNode.ScrollPageID != "" {
		return singleNode.Children, nil
	}

	var nodes []models.ScrollPageTreeNode
	if err := json.Unmarshal(bodyBytes, &nodes); err != nil {
		return nil, fmt.Errorf("failed to decode pagetree children response: %w", err)
	}
	return nodes, nil
}

// ResolvePage resolves a Scroll page to its Confluence page for a specific version.
func (s *ScrollVersionsClient) ResolvePage(ctx context.Context, spaceKey, scrollPageID, versionID string) (*models.ScrollPage, error) {
	path := fmt.Sprintf("%s/page/%s/%s/%s", scrollVersionsAPIPrefix, spaceKey, scrollPageID, versionID)
	resp, err := s.http.MakeRequest(ctx, "GET", path, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("scroll versions resolve page request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("scroll versions resolve page: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var page models.ScrollPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("failed to decode scroll page: %w", err)
	}
	return &page, nil
}
