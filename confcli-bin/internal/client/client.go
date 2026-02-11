package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
	
	"confcli/internal/cache"
)

const (
	// APIVersion refers to the Confluence REST API version
	APIVersion = "v2" // Using v2 for Cloud, fallback to v1 for Server
)

// Client represents a Confluence API client
type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client
	AuthType   string
	Token      string
	Username   string
	Password   string
	ReadOnly   bool
	Cache      *cache.Cache
}

// Page represents a Confluence page
type Page struct {
	ID          int                    `json:"id,omitempty"`
	Title       string                 `json:"title,omitempty"`
	SpaceID     int                    `json:"spaceId,omitempty"`
	Status      string                 `json:"status,omitempty"`
	CreatedAt   time.Time              `json:"createdAt,omitempty"`
	UpdatedAt   time.Time              `json:"updatedAt,omitempty"`
	Version     Version                `json:"version,omitempty"`
	AuthorID    string                 `json:"authorId,omitempty"`
	Body        map[string]interface{} `json:"body,omitempty"`
	Links       map[string]string      `json:"_links,omitempty"`
	Ancestors   []Page                 `json:"ancestors,omitempty"`
	Descendants []Page                 `json:"descendants,omitempty"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	Labels      []Label                `json:"labels,omitempty"`
	Comments    []Comment              `json:"comments,omitempty"`
}

// Version represents the version of a page
type Version struct {
	Number    int       `json:"number,omitempty"`
	Message   string    `json:"message,omitempty"`
	MinorEdit bool      `json:"minorEdit,omitempty"`
	AuthorID  string    `json:"authorId,omitempty"`
	UpdatedAt time.Time `json:"when,omitempty"`
}

// Attachment represents a Confluence attachment
type Attachment struct {
	ID        string    `json:"id,omitempty"`
	Title     string    `json:"title,omitempty"`
	Filename  string    `json:"filename,omitempty"`
	FileSize  int64     `json:"fileSize,omitempty"`
	MediaType string    `json:"mediaType,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
	Links     map[string]string `json:"_links,omitempty"`
}

// Label represents a Confluence label
type Label struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// Comment represents a Confluence comment
type Comment struct {
	ID        string    `json:"id,omitempty"`
	Body      map[string]interface{} `json:"body,omitempty"`
	AuthorID  string    `json:"authorId,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
	ParentID  string    `json:"parentId,omitempty"`
	PageID    string    `json:"pageId,omitempty"`
}

// Space represents a Confluence space
type Space struct {
	ID          int    `json:"id,omitempty"`
	Key         string `json:"key,omitempty"`
	Name        string `json:"name,omitempty"`
	Description map[string]interface{} `json:"description,omitempty"`
	HomepageID  int    `json:"homepageId,omitempty"`
	Status      string `json:"status,omitempty"`
	Links       map[string]string `json:"_links,omitempty"`
}

// SearchResult represents a search result
type SearchResult struct {
	ID    int    `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	Type  string `json:"type,omitempty"`
	Space Space  `json:"space,omitempty"`
}

// NewClient creates a new Confluence API client
func NewClient() (*Client, error) {
	baseURLStr := viper.GetString("url")
	if baseURLStr == "" {
		return nil, fmt.Errorf("Confluence URL is not configured. Please set it using 'confcli config set url <your_confluence_url>'")
	}

	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, fmt.Errorf("invalid Confluence URL: %w", err)
	}

	// Create cache
	cacheTTL := viper.GetInt("cache_ttl")
	if cacheTTL == 0 {
		cacheTTL = 5 // Default to 5 minutes
	}
	pageCache, err := cache.NewCache(cacheTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	client := &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		AuthType:   viper.GetString("auth_type"),
		Token:      viper.GetString("token"),
		Username:   viper.GetString("username"),
		Password:   "", // Password is typically not stored, maybe read from environment
		ReadOnly:   viper.GetBool("read_only"),
		Cache:      pageCache,
	}

	return client, nil
}

// makeRequest performs an HTTP request to the Confluence API
func (c *Client) makeRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	// Construct the full URL
	fullURL := c.BaseURL.ResolveReference(&url.URL{Path: path})
	
	req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), body)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set authentication header
	if err := c.setAuthHeader(req); err != nil {
		return nil, err
	}

	// Check if this is a write operation and we're in read-only mode
	if c.ReadOnly && (method == "POST" || method == "PUT" || method == "DELETE") {
		return nil, fmt.Errorf("read-only mode enabled: cannot perform %s operation", method)
	}

	// Perform the request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// setAuthHeader sets the appropriate authentication header based on the auth type
func (c *Client) setAuthHeader(req *http.Request) error {
	switch strings.ToLower(c.AuthType) {
	case "bearer":
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
	case "basic":
		if c.Username != "" && c.Token != "" {
			req.SetBasicAuth(c.Username, c.Token)
		}
	default:
		return fmt.Errorf("unsupported auth type: %s", c.AuthType)
	}
	return nil
}

// GetPage retrieves a page by its ID
func (c *Client) GetPage(ctx context.Context, id int) (*Page, error) {
	cacheKey := fmt.Sprintf("page:%d", id)
	
	// Try to get from cache first
	if c.Cache != nil {
		cachedValue, found, err := c.Cache.Get(cacheKey)
		if err == nil && found {
			// Convert cached interface{} back to Page
			if pageData, ok := cachedValue.(map[string]interface{}); ok {
				// Convert map back to Page struct
				jsonData, err := json.Marshal(pageData)
				if err == nil {
					var page Page
					if err := json.Unmarshal(jsonData, &page); err == nil {
						return &page, nil
					}
				}
			}
		}
	}
	
	path := fmt.Sprintf("/api/content/%d", id)
	
	resp, err := c.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}

	// Cache the result
	if c.Cache != nil {
		if err := c.Cache.Set(cacheKey, page); err != nil {
			// Log error but continue
		}
	}

	return &page, nil
}

// GetPageByTitle retrieves a page by its space key and title
func (c *Client) GetPageByTitle(ctx context.Context, spaceKey, title string) (*Page, error) {
	cacheKey := fmt.Sprintf("page_by_title:%s:%s", spaceKey, title)
	
	// Try to get from cache first
	if c.Cache != nil {
		cachedValue, found, err := c.Cache.Get(cacheKey)
		if err == nil && found {
			// Convert cached interface{} back to Page
			if pageData, ok := cachedValue.(map[string]interface{}); ok {
				// Convert map back to Page struct
				jsonData, err := json.Marshal(pageData)
				if err == nil {
					var page Page
					if err := json.Unmarshal(jsonData, &page); err == nil {
						return &page, nil
					}
				}
			}
		}
	}
	
	params := url.Values{}
	params.Add("space", spaceKey)
	params.Add("title", title)
	
	path := "/api/content?" + params.Encode()
	
	resp, err := c.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []Page `json:"results"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Results) == 0 {
		return nil, fmt.Errorf("page with title '%s' in space '%s' not found", title, spaceKey)
	}

	page := &result.Results[0]
	
	// Cache the result
	if c.Cache != nil {
		if err := c.Cache.Set(cacheKey, *page); err != nil {
			// Log error but continue
		}
	}

	return page, nil
}

// GetPageContent retrieves the content of a page in the specified format
func (c *Client) GetPageContent(ctx context.Context, id int, format string) (string, error) {
	cacheKey := fmt.Sprintf("page_content:%d:%s", id, format)
	
	// Try to get from cache first
	if c.Cache != nil {
		cachedValue, found, err := c.Cache.Get(cacheKey)
		if err == nil && found {
			if content, ok := cachedValue.(string); ok {
				return content, nil
			}
		}
	}
	
	expansions := []string{"body.view", "body.storage", "body.editor", "body.export_view", "body.styled_view"}
	
	expandParam := strings.Join(expansions, ",")
	params := url.Values{}
	params.Add("expand", expandParam)
	
	path := fmt.Sprintf("/api/content/%d?%s", id, params.Encode())
	
	resp, err := c.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var page Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return "", err
	}

	// Return content in the requested format
	bodyContent, ok := page.Body[format]
	if !ok {
		// If the requested format is not available, return the first available format
		for _, f := range []string{"storage", "view", "editor", "export_view", "styled_view"} {
			if content, exists := page.Body[f]; exists {
				if contentMap, ok := content.(map[string]interface{}); ok {
					if value, ok := contentMap["value"].(string); ok {
						// Cache the result
						if c.Cache != nil {
							if err := c.Cache.Set(cacheKey, value); err != nil {
								// Log error but continue
							}
						}
						return value, nil
					}
				}
			}
		}
		return "", fmt.Errorf("content in format '%s' not available", format)
	}

	if contentMap, ok := bodyContent.(map[string]interface{}); ok {
		if value, ok := contentMap["value"].(string); ok {
			// Cache the result
			if c.Cache != nil {
				if err := c.Cache.Set(cacheKey, value); err != nil {
					// Log error but continue
				}
			}
			return value, nil
		}
	}

	return "", fmt.Errorf("could not extract content in format '%s'", format)
}

// GetPageChildren retrieves the children of a page
func (c *Client) GetPageChildren(ctx context.Context, id int) ([]Page, error) {
	cacheKey := fmt.Sprintf("page_children:%d", id)
	
	// Try to get from cache first
	if c.Cache != nil {
		cachedValue, found, err := c.Cache.Get(cacheKey)
		if err == nil && found {
			// Convert cached interface{} back to []Page
			if pagesData, ok := cachedValue.([]interface{}); ok {
				// Convert array of maps back to []Page struct
				var pages []Page
				for _, pageData := range pagesData {
					if pageMap, ok := pageData.(map[string]interface{}); ok {
						jsonData, err := json.Marshal(pageMap)
						if err == nil {
							var page Page
							if err := json.Unmarshal(jsonData, &page); err == nil {
								pages = append(pages, page)
							}
						}
					}
				}
				if len(pages) > 0 {
					return pages, nil
				}
			}
		}
	}
	
	path := fmt.Sprintf("/api/content/%d/child/page", id)
	
	resp, err := c.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []Page `json:"results"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Cache the result
	if c.Cache != nil {
		if err := c.Cache.Set(cacheKey, result.Results); err != nil {
			// Log error but continue
		}
	}

	return result.Results, nil
}

// GetDescendants retrieves all descendants of a page up to a certain depth
func (c *Client) GetDescendants(ctx context.Context, id int, depth int) ([]Page, error) {
	cacheKey := fmt.Sprintf("descendants:%d:%d", id, depth)
	
	// Try to get from cache first
	if c.Cache != nil {
		cachedValue, found, err := c.Cache.Get(cacheKey)
		if err == nil && found {
			// Convert cached interface{} back to []Page
			if pagesData, ok := cachedValue.([]interface{}); ok {
				// Convert array of maps back to []Page struct
				var pages []Page
				for _, pageData := range pagesData {
					if pageMap, ok := pageData.(map[string]interface{}); ok {
						jsonData, err := json.Marshal(pageMap)
						if err == nil {
							var page Page
							if err := json.Unmarshal(jsonData, &page); err == nil {
								pages = append(pages, page)
							}
						}
					}
				}
				if len(pages) > 0 {
					return pages, nil
				}
			}
		}
	}
	
	// This is a simplified implementation - a full implementation would require recursion
	// to get all descendants up to the specified depth
	pages := make([]Page, 0)
	
	// Get the initial page (we'll use it to validate the ID exists)
	_, err := c.GetPage(ctx, id)
	if err != nil {
		return nil, err
	}
	
	// Get children of the initial page
	children, err := c.GetPageChildren(ctx, id)
	if err != nil {
		return nil, err
	}
	
	pages = append(pages, children...)
	
	// If depth > 1, recursively get children of children
	if depth > 1 || depth == 0 { // 0 means unlimited depth
		for _, child := range children {
			childDescendants, err := c.getDescendantsRecursive(ctx, child.ID, depth, 2)
			if err != nil {
				// Log the error but continue with other pages
				continue
			}
			pages = append(pages, childDescendants...)
		}
	}
	
	// Cache the result
	if c.Cache != nil {
		if err := c.Cache.Set(cacheKey, pages); err != nil {
			// Log error but continue
		}
	}
	
	return pages, nil
}

// getDescendantsRecursive is a helper function to recursively get descendants
func (c *Client) getDescendantsRecursive(ctx context.Context, id int, maxDepth, currentDepth int) ([]Page, error) {
	if maxDepth > 0 && currentDepth > maxDepth {
		return []Page{}, nil
	}
	
	children, err := c.GetPageChildren(ctx, id)
	if err != nil {
		return nil, err
	}
	
	result := children
	
	// Recursively get children of children
	for _, child := range children {
		childDescendants, err := c.getDescendantsRecursive(ctx, child.ID, maxDepth, currentDepth+1)
		if err != nil {
			// Log the error but continue with other pages
			continue
		}
		result = append(result, childDescendants...)
	}
	
	return result, nil
}

// GetSpaceRootPages retrieves the root pages of a space
func (c *Client) GetSpaceRootPages(ctx context.Context, spaceKey string) ([]Page, error) {
	cacheKey := fmt.Sprintf("space_root_pages:%s", spaceKey)
	
	// Try to get from cache first
	if c.Cache != nil {
		cachedValue, found, err := c.Cache.Get(cacheKey)
		if err == nil && found {
			// Convert cached interface{} back to []Page
			if pagesData, ok := cachedValue.([]interface{}); ok {
				// Convert array of maps back to []Page struct
				var pages []Page
				for _, pageData := range pagesData {
					if pageMap, ok := pageData.(map[string]interface{}); ok {
						jsonData, err := json.Marshal(pageMap)
						if err == nil {
							var page Page
							if err := json.Unmarshal(jsonData, &page); err == nil {
								pages = append(pages, page)
							}
						}
					}
				}
				if len(pages) > 0 {
					return pages, nil
				}
			}
		}
	}
	
	// First get the space to find its homepage
	space, err := c.GetSpace(ctx, spaceKey)
	if err != nil {
		return nil, err
	}
	
	// Get the homepage and its children
	homepage, err := c.GetPage(ctx, space.HomepageID)
	if err != nil {
		return nil, err
	}
	
	// Get children of the homepage (these are typically root pages)
	children, err := c.GetPageChildren(ctx, homepage.ID)
	if err != nil {
		return nil, err
	}
	
	// Add the homepage itself as a root page
	rootPages := append([]Page{*homepage}, children...)
	
	// Cache the result
	if c.Cache != nil {
		if err := c.Cache.Set(cacheKey, rootPages); err != nil {
			// Log error but continue
		}
	}
	
	return rootPages, nil
}

// GetSpace retrieves a space by its key
func (c *Client) GetSpace(ctx context.Context, key string) (*Space, error) {
	cacheKey := fmt.Sprintf("space:%s", key)
	
	// Try to get from cache first
	if c.Cache != nil {
		cachedValue, found, err := c.Cache.Get(cacheKey)
		if err == nil && found {
			// Convert cached interface{} back to Space
			if spaceData, ok := cachedValue.(map[string]interface{}); ok {
				// Convert map back to Space struct
				jsonData, err := json.Marshal(spaceData)
				if err == nil {
					var space Space
					if err := json.Unmarshal(jsonData, &space); err == nil {
						return &space, nil
					}
				}
			}
		}
	}
	
	path := fmt.Sprintf("/api/space/%s", key)
	
	resp, err := c.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var space Space
	if err := json.NewDecoder(resp.Body).Decode(&space); err != nil {
		return nil, err
	}

	// Cache the result
	if c.Cache != nil {
		if err := c.Cache.Set(cacheKey, space); err != nil {
			// Log error but continue
		}
	}

	return &space, nil
}

// GetAllPagesInSpace retrieves all pages in a space
func (c *Client) GetAllPagesInSpace(ctx context.Context, spaceKey string) ([]Page, error) {
	cacheKey := fmt.Sprintf("all_pages_in_space:%s", spaceKey)
	
	// Try to get from cache first
	if c.Cache != nil {
		cachedValue, found, err := c.Cache.Get(cacheKey)
		if err == nil && found {
			// Convert cached interface{} back to []Page
			if pagesData, ok := cachedValue.([]interface{}); ok {
				// Convert array of maps back to []Page struct
				var pages []Page
				for _, pageData := range pagesData {
					if pageMap, ok := pageData.(map[string]interface{}); ok {
						jsonData, err := json.Marshal(pageMap)
						if err == nil {
							var page Page
							if err := json.Unmarshal(jsonData, &page); err == nil {
								pages = append(pages, page)
							}
						}
					}
				}
				if len(pages) > 0 {
					return pages, nil
				}
			}
		}
	}
	
	allPages := make([]Page, 0)
	
	// Use pagination to get all pages
	start := 0
	limit := 100 // Max limit for Confluence API
	
	for {
		params := url.Values{}
		params.Add("space", spaceKey)
		params.Add("start", fmt.Sprintf("%d", start))
		params.Add("limit", fmt.Sprintf("%d", limit))
		
		path := "/api/content?" + params.Encode()
		
		resp, err := c.makeRequest(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		var result struct {
			Results  []Page `json:"results"`
			Start    int    `json:"start,omitempty"`
			Limit    int    `json:"limit,omitempty"`
			Size     int    `json:"size,omitempty"`
			Next     string `json:"_links.next,omitempty"`
		}
		
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}

		allPages = append(allPages, result.Results...)

		// Check if we've retrieved all pages
		if len(result.Results) < limit || (result.Start+result.Limit) >= result.Size {
			break
		}

		start += limit
	}
	
	// Cache the result
	if c.Cache != nil {
		if err := c.Cache.Set(cacheKey, allPages); err != nil {
			// Log error but continue
		}
	}
	
	return allPages, nil
}

// Search searches for pages using CQL
func (c *Client) Search(ctx context.Context, cql string, limit int) ([]SearchResult, error) {
	cacheKey := fmt.Sprintf("search:%s:%d", cql, limit)
	
	// Try to get from cache first
	if c.Cache != nil {
		cachedValue, found, err := c.Cache.Get(cacheKey)
		if err == nil && found {
			// Convert cached interface{} back to []SearchResult
			if resultsData, ok := cachedValue.([]interface{}); ok {
				// Convert array of maps back to []SearchResult struct
				var results []SearchResult
				for _, resultData := range resultsData {
					if resultMap, ok := resultData.(map[string]interface{}); ok {
						jsonData, err := json.Marshal(resultMap)
						if err == nil {
							var result SearchResult
							if err := json.Unmarshal(jsonData, &result); err == nil {
								results = append(results, result)
							}
						}
					}
				}
				if len(results) > 0 {
					return results, nil
				}
			}
		}
	}
	
	params := url.Values{}
	params.Add("cql", cql)
	if limit > 0 {
		params.Add("limit", fmt.Sprintf("%d", limit))
	}
	
	path := "/api/search?" + params.Encode()
	
	resp, err := c.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []SearchResult `json:"results"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Cache the result
	if c.Cache != nil {
		if err := c.Cache.Set(cacheKey, result.Results); err != nil {
			// Log error but continue
		}
	}

	return result.Results, nil
}

// CreatePage creates a new page
func (c *Client) CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*Page, error) {
	if c.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot create page")
	}

	// Prepare the request body
	pageData := map[string]interface{}{
		"type":  "page",
		"title": title,
		"space": map[string]interface{}{
			"key": spaceKey,
		},
		"body": map[string]interface{}{
			"storage": map[string]interface{}{
				"value":          content,
				"representation": format,
			},
		},
	}

	if parentID != nil {
		pageData["ancestors"] = []map[string]interface{}{
			{"id": *parentID},
		}
	}

	jsonData, err := json.Marshal(pageData)
	if err != nil {
		return nil, err
	}

	resp, err := c.makeRequest(ctx, "POST", "/api/content", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var newPage Page
	if err := json.NewDecoder(resp.Body).Decode(&newPage); err != nil {
		return nil, err
	}

	return &newPage, nil
}

// UpdatePage updates an existing page
func (c *Client) UpdatePage(ctx context.Context, id int, content string, versionComment string) (*Page, error) {
	if c.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot update page")
	}

	// First get the current page to retrieve its version
	currentPage, err := c.GetPage(ctx, id)
	if err != nil {
		return nil, err
	}

	// Prepare the request body
	pageData := map[string]interface{}{
		"id":    fmt.Sprintf("%d", id),
		"type":  "page",
		"title": currentPage.Title,
		"version": map[string]interface{}{
			"number":  currentPage.Version.Number + 1,
			"message": versionComment,
		},
		"body": map[string]interface{}{
			"storage": map[string]interface{}{
				"value":          content,
				"representation": "storage",
			},
		},
	}

	jsonData, err := json.Marshal(pageData)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/api/content/%d", id)
	resp, err := c.makeRequest(ctx, "PUT", path, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var updatedPage Page
	if err := json.NewDecoder(resp.Body).Decode(&updatedPage); err != nil {
		return nil, err
	}

	return &updatedPage, nil
}

// DeletePage deletes a page
func (c *Client) DeletePage(ctx context.Context, id int) error {
	if c.ReadOnly {
		return fmt.Errorf("read-only mode enabled: cannot delete page")
	}

	path := fmt.Sprintf("/api/content/%d", id)
	resp, err := c.makeRequest(ctx, "DELETE", path, nil)
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

// AddComment adds a comment to a page
func (c *Client) AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*Comment, error) {
	if c.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot add comment")
	}

	commentData := map[string]interface{}{
		"type": "comment",
		"body": map[string]interface{}{
			"storage": map[string]interface{}{
				"value":          text,
				"representation": "storage",
			},
		},
	}

	if parentCommentID != nil {
		commentData["container"] = map[string]interface{}{
			"id":   fmt.Sprintf("%d", *parentCommentID),
			"type": "comment",
		}
	} else {
		commentData["container"] = map[string]interface{}{
			"id":   fmt.Sprintf("%d", pageID),
			"type": "page",
		}
	}

	jsonData, err := json.Marshal(commentData)
	if err != nil {
		return nil, err
	}

	resp, err := c.makeRequest(ctx, "POST", "/api/content", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var newComment Comment
	if err := json.NewDecoder(resp.Body).Decode(&newComment); err != nil {
		return nil, err
	}

	return &newComment, nil
}

// AddLabel adds a label to a page
func (c *Client) AddLabel(ctx context.Context, pageID int, labelName string) error {
	if c.ReadOnly {
		return fmt.Errorf("read-only mode enabled: cannot add label")
	}

	labelData := map[string]interface{}{
		"prefix": "global",
		"name":   labelName,
	}

	jsonData, err := json.Marshal(labelData)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/content/%d/label", pageID)
	resp, err := c.makeRequest(ctx, "POST", path, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}