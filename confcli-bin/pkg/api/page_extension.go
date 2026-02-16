package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"confcli/pkg/models"
)

// PageExtension provides higher-level page operations built on top of HTTPClient
type PageExtension struct {
	client *HTTPClient
}

// NewPageExtension creates a new page extension
func NewPageExtension(client *HTTPClient) *PageExtension {
	return &PageExtension{client: client}
}

// FetchPageRequest represents a request to fetch a page
type FetchPageRequest struct {
	PageID     int
	Expansions []string
}

// FetchPageResponse represents the response from fetching a page
type FetchPageResponse struct {
	Page *models.Page
}

// FetchPage fetches a page with optional expansions
func (e *PageExtension) FetchPage(ctx context.Context, req *FetchPageRequest) (*FetchPageResponse, error) {
	path := fmt.Sprintf("%s/content/%d", e.client.APIPrefix, req.PageID)
	if len(req.Expansions) > 0 {
		params := url.Values{}
		params.Add("expand", strings.Join(req.Expansions, ","))
		path = path + "?" + params.Encode()
	}

	resp, err := e.client.MakeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var page models.Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}

	return &FetchPageResponse{Page: &page}, nil
}

// FindPageByTitleRequest represents a request to find a page by title
type FindPageByTitleRequest struct {
	SpaceKey string
	Title    string
}

// FindPageByTitleResponse represents the response from finding a page by title
type FindPageByTitleResponse struct {
	Page *models.Page
}

// FindPageByTitle finds a page by space key and title
func (e *PageExtension) FindPageByTitle(ctx context.Context, req *FindPageByTitleRequest) (*FindPageByTitleResponse, error) {
	params := url.Values{}
	params.Add("space", req.SpaceKey)
	params.Add("title", req.Title)

	path := e.client.APIPrefix + "/content?" + params.Encode()
	resp, err := e.client.MakeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []models.Page `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Results) == 0 {
		return nil, fmt.Errorf("page with title '%s' in space '%s' not found", req.Title, req.SpaceKey)
	}

	return &FindPageByTitleResponse{Page: &result.Results[0]}, nil
}

// FetchPageContentRequest represents a request to fetch page content
type FetchPageContentRequest struct {
	PageID interface{}
	Format string
}

// FetchPageContentResponse represents the response from fetching page content
type FetchPageContentResponse struct {
	Content string
	Page    *models.Page
}

// FetchPageContent fetches page content in the specified format
func (e *PageExtension) FetchPageContent(ctx context.Context, req *FetchPageContentRequest) (*FetchPageContentResponse, error) {
	idStr := pageIDToString(req.PageID)
	expansion := fmt.Sprintf("body.%s", req.Format)
	params := url.Values{}
	params.Add("expand", expansion)

	path := fmt.Sprintf("%s/content/%s?%s", e.client.APIPrefix, idStr, params.Encode())
	resp, err := e.client.MakeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var page models.Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}

	// Extract content in the requested format
	content, err := extractBodyContent(page.Body, req.Format)
	if err != nil {
		return nil, err
	}

	return &FetchPageContentResponse{
		Content: content,
		Page:    &page,
	}, nil
}

// FetchPageChildrenRequest represents a request to fetch page children
type FetchPageChildrenRequest struct {
	PageID int
}

// FetchPageChildrenResponse represents the response from fetching page children
type FetchPageChildrenResponse struct {
	Children []models.Page
}

// FetchPageChildren fetches children of a page
func (e *PageExtension) FetchPageChildren(ctx context.Context, req *FetchPageChildrenRequest) (*FetchPageChildrenResponse, error) {
	path := fmt.Sprintf("%s/content/%d/child/page", e.client.APIPrefix, req.PageID)
	resp, err := e.client.MakeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []models.Page `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &FetchPageChildrenResponse{Children: result.Results}, nil
}

// CreatePageRequest represents a request to create a page
type CreatePageRequest struct {
	SpaceKey string
	ParentID *int
	Title    string
	Content  string
	Format   string
}

// CreatePageResponse represents the response from creating a page
type CreatePageResponse struct {
	Page *models.Page
}

// CreatePage creates a new page
func (e *PageExtension) CreatePage(ctx context.Context, req *CreatePageRequest) (*CreatePageResponse, error) {
	if e.client.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot create page")
	}

	pageData := map[string]interface{}{
		"type":  "page",
		"title": req.Title,
		"space": map[string]interface{}{
			"key": req.SpaceKey,
		},
		"body": map[string]interface{}{
			"storage": map[string]interface{}{
				"value":          req.Content,
				"representation": req.Format,
			},
		},
	}

	if req.ParentID != nil {
		pageData["ancestors"] = []map[string]interface{}{
			{"id": fmt.Sprintf("%d", *req.ParentID)},
		}
	}

	jsonData, err := json.Marshal(pageData)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.MakeRequest(ctx, "POST", e.client.APIPrefix+"/content", strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var newPage models.Page
	if err := json.NewDecoder(resp.Body).Decode(&newPage); err != nil {
		return nil, err
	}

	return &CreatePageResponse{Page: &newPage}, nil
}

// UpdatePageRequest represents a request to update a page
type UpdatePageRequest struct {
	PageID          int
	Content         string
	VersionComment  string
	Title           string // Optional: if not provided, will fetch current title
	SkipFetchCurrent bool // If true, assumes caller provides correct title
}

// UpdatePageResponse represents the response from updating a page
type UpdatePageResponse struct {
	Page *models.Page
}

// UpdatePage updates an existing page
func (e *PageExtension) UpdatePage(ctx context.Context, req *UpdatePageRequest) (*UpdatePageResponse, error) {
	if e.client.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot update page")
	}

	title := req.Title
	versionNumber := 0

	// Fetch current page if title not provided
	if !req.SkipFetchCurrent {
		currentPage, err := e.fetchCurrentPageInfo(ctx, req.PageID)
		if err != nil {
			return nil, err
		}
		title = currentPage.Title
		versionNumber = currentPage.Version.Number
	}

	pageData := map[string]interface{}{
		"id":    fmt.Sprintf("%d", req.PageID),
		"type":  "page",
		"title": title,
		"version": map[string]interface{}{
			"number":  versionNumber + 1,
			"message": req.VersionComment,
		},
		"body": map[string]interface{}{
			"storage": map[string]interface{}{
				"value":          req.Content,
				"representation": "storage",
			},
		},
	}

	jsonData, err := json.Marshal(pageData)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/content/%d", e.client.APIPrefix, req.PageID)
	resp, err := e.client.MakeRequest(ctx, "PUT", path, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var updatedPage models.Page
	if err := json.NewDecoder(resp.Body).Decode(&updatedPage); err != nil {
		return nil, err
	}

	return &UpdatePageResponse{Page: &updatedPage}, nil
}

// DeletePageRequest represents a request to delete a page
type DeletePageRequest struct {
	PageID int
}

// DeletePage deletes a page
func (e *PageExtension) DeletePage(ctx context.Context, req *DeletePageRequest) error {
	if e.client.ReadOnly {
		return fmt.Errorf("read-only mode enabled: cannot delete page")
	}

	path := fmt.Sprintf("%s/content/%d", e.client.APIPrefix, req.PageID)
	resp, err := e.client.MakeRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// fetchCurrentPageInfo fetches the current page info for update operations
func (e *PageExtension) fetchCurrentPageInfo(ctx context.Context, pageID int) (*models.Page, error) {
	path := fmt.Sprintf("%s/content/%d", e.client.APIPrefix, pageID)
	resp, err := e.client.MakeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get current page: API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var page models.Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}

	return &page, nil
}

// Helper function to convert page ID to string
func pageIDToString(id interface{}) string {
	switch v := id.(type) {
	case int:
		return fmt.Sprintf("%d", v)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// extractBodyContent extracts content from the body map in the specified format
func extractBodyContent(body map[string]interface{}, format string) (string, error) {
	bodyContent, ok := body[format]
	if !ok {
		// If the requested format is not available, return the first available format
		for _, f := range []string{"storage", "view", "export_view", "styled_view"} {
			if content, exists := body[f]; exists {
				if contentMap, ok := content.(map[string]interface{}); ok {
					if value, ok := contentMap["value"].(string); ok {
						return value, nil
					}
				}
			}
		}
		return "", fmt.Errorf("content in format '%s' not available", format)
	}

	if contentMap, ok := bodyContent.(map[string]interface{}); ok {
		if value, ok := contentMap["value"].(string); ok {
			return value, nil
		}
	}

	return "", fmt.Errorf("could not extract content in format '%s'", format)
}

// GetHTTPClient returns the underlying HTTP client
func (e *PageExtension) GetHTTPClient() *HTTPClient {
	return e.client
}
