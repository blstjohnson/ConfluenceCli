package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/viper"

	"confcli/internal/cache"
	"confcli/internal/config"
	"confcli/internal/logging"
)

const (
	// APIVersion refers to the Confluence REST API version
	APIVersion = "v2" // Using v2 for Cloud, fallback to v1 for Server
)

// Client represents a Confluence API client
type Client struct {
	BaseURL        *url.URL
	HTTPClient     *http.Client
	AuthType       string
	Token          string
	Username       string
	Password       string
	ImpersonateAs  string // User to impersonate
	UseDomainAuth  bool   // Use current domain user for authentication
	ReadOnly       bool
	Cache          *cache.Cache
	Logger         *logging.Logger
	APIPrefix      string // API path prefix (e.g., "/rest/api" for Server, "/api" for Cloud)
	SessionCookie  string // Session cookie for browser-based auth
	SAMLAuthCookie string // SAML auth cookie for identity provider
}

// PageID represents a Confluence page ID that can be either string or int
type PageID struct {
	Value interface{}
}

func (p *PageID) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as integer first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		p.Value = i
		return nil
	}
	
	// If that fails, try to unmarshal as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		p.Value = s
		return nil
	}
	
	return fmt.Errorf("cannot unmarshal %s into PageID", data)
}

func (p PageID) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Value)
}

func (p PageID) String() string {
	switch v := p.Value.(type) {
	case int:
		return fmt.Sprintf("%d", v)
	case string:
		return v
	default:
		return ""
	}
}

func (p PageID) Int() (int, bool) {
	if v, ok := p.Value.(int); ok {
		return v, true
	}
	return 0, false
}

// IntOrString returns the ID as an integer if it's stored as an integer,
// otherwise returns it as a string representation
func (p PageID) IntOrString() interface{} {
	if v, ok := p.Value.(int); ok {
		return v
	}
	return p.String()
}

// Page represents a Confluence page
type Page struct {
	ID          PageID                 `json:"id,omitempty"`
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

	// Create a cookie jar to store session cookies
	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	// Create HTTP client with appropriate timeout and cookie jar
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     cookieJar,
	}

	// Determine API prefix based on URL (Confluence Server typically uses /rest/api, Cloud uses /api)
	apiPrefix := "/api" // Default to Cloud API
	if strings.Contains(strings.ToLower(baseURLStr), "atlassian.net") {
		// Confluence Cloud URL pattern
		apiPrefix = "/api"
	} else {
		// Assume Confluence Server/Data Center
		apiPrefix = "/rest/api"
	}

	// Get current profile name
	currentProfile := viper.GetString("current_profile")
	if currentProfile == "" {
		currentProfile = config.DefaultProfileName
	}

	// For domain authentication, we'll use the default transport which can leverage system credentials
	// on Windows systems when properly configured
	client := &Client{
		BaseURL:        baseURL,
		HTTPClient:     httpClient,
		AuthType:       viper.GetString("auth_type"),
		Token:          viper.GetString("token"),
		Username:       viper.GetString("username"),
		ImpersonateAs:  viper.GetString("impersonate_as"), // User to impersonate
		UseDomainAuth:  viper.GetBool("use_domain_auth"),  // Use current domain user for authentication
		Password:       "", // Password is typically not stored, maybe read from environment
		ReadOnly:       viper.GetBool("read_only"),
		Cache:          pageCache,
		Logger:         logging.NewLogger(),
		APIPrefix:      apiPrefix,
		SessionCookie:  viper.GetString(fmt.Sprintf("profiles.%s.session_cookie", currentProfile)),  // Session cookie for browser-based auth
		SAMLAuthCookie: viper.GetString(fmt.Sprintf("profiles.%s.saml_auth_cookie", currentProfile)), // SAML auth cookie for identity provider
	}

	// Pre-populate the cookie jar with configured cookies
	client.setCookiesFromConfig()
	
	// Debug logging to see what cookies were loaded
	client.Logger.Debug("SessionCookie loaded: '%s'", client.SessionCookie)
	client.Logger.Debug("SAMLAuthCookie loaded: '%s'", client.SAMLAuthCookie)

	return client, nil
}

// setCookiesFromConfig adds configured cookies to the HTTP client's cookie jar
func (c *Client) setCookiesFromConfig() {
	// Add session cookie if configured
	if c.SessionCookie != "" {
		parts := strings.SplitN(c.SessionCookie, "=", 2)
		if len(parts) == 2 {
			// Extract just the hostname (without port if present)
			host := c.BaseURL.Host
			// If the host contains a port, we should strip it for cookie domain
			if colonIndex := strings.LastIndex(host, ":"); colonIndex != -1 {
				host = host[:colonIndex]
			}
			
			cookie := &http.Cookie{
				Name:   parts[0],
				Value:  parts[1],
				Path:   "/",
				Domain: host,
			}
			cookies := []*http.Cookie{cookie}
			// Use the full URL to set cookies properly
			c.HTTPClient.Jar.SetCookies(c.BaseURL, cookies)
		}
	}

	// Add SAML auth cookie if configured
	if c.SAMLAuthCookie != "" {
		parts := strings.SplitN(c.SAMLAuthCookie, "=", 2)
		if len(parts) == 2 {
			// Extract just the hostname (without port if present)
			host := c.BaseURL.Host
			// If the host contains a port, we should strip it for cookie domain
			if colonIndex := strings.LastIndex(host, ":"); colonIndex != -1 {
				host = host[:colonIndex]
			}
			
			cookie := &http.Cookie{
				Name:   parts[0],
				Value:  parts[1],
				Path:   "/", // Root path to ensure cookie is sent with all requests including API calls
				Domain: host,
			}
			cookies := []*http.Cookie{cookie}
			// Use the full URL to set cookies properly
			c.HTTPClient.Jar.SetCookies(c.BaseURL, cookies)
		}
	}
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

	// For browser authentication, manually add cookies to the request
	// since the cookie jar might not be working properly with the domain
	if strings.ToLower(c.AuthType) == "browser" {
		c.Logger.Debug("Browser auth detected, adding cookies to request")
		var cookies []string
		if c.SessionCookie != "" {
			c.Logger.Debug("Adding session cookie: %s", c.SessionCookie)
			cookies = append(cookies, c.SessionCookie)
		}
		if c.SAMLAuthCookie != "" && c.SAMLAuthCookie != c.SessionCookie {
			c.Logger.Debug("Adding SAML auth cookie: %s", c.SAMLAuthCookie)
			cookies = append(cookies, c.SAMLAuthCookie)
		}
		
		if len(cookies) > 0 {
			cookieHeader := strings.Join(cookies, "; ")
			c.Logger.Debug("Setting Cookie header: %s", cookieHeader)
			req.Header.Set("Cookie", cookieHeader)
		} else {
			c.Logger.Debug("No cookies to add")
		}
	} else {
		c.Logger.Debug("Auth type is %s, not browser auth", c.AuthType)
	}

	// Log the request if debug is enabled
	if c.Logger.IsDebugEnabled() {
		requestDump, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			c.Logger.Debug("Failed to dump request: %v", err)
		} else {
			c.Logger.Debug("HTTP Request:\n%s", string(requestDump))
		}
	}

	// Perform the request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Log the response if debug is enabled
	if c.Logger.IsDebugEnabled() {
		responseDump, err := httputil.DumpResponse(resp, true)
		if err != nil {
			c.Logger.Debug("Failed to dump response: %v", err)
		} else {
			c.Logger.Debug("HTTP Response:\n%s", string(responseDump))
		}
	}

	return resp, nil
}

// setAuthHeader sets the appropriate authentication header based on the auth type
func (c *Client) setAuthHeader(req *http.Request) error {
	// If using domain authentication, skip setting explicit auth headers
	// The system will handle authentication automatically using current user credentials
	if c.UseDomainAuth {
		// Still add impersonation header if configured
		if c.ImpersonateAs != "" {
			req.Header.Set("X-AsUser", c.ImpersonateAs)
		}
		return nil
	}

	// Handle browser-based authentication
	if strings.ToLower(c.AuthType) == "browser" {
		// For browser-based auth, cookies are automatically handled by the HTTP client's cookie jar
		// The cookies were pre-loaded in setCookiesFromConfig()
		// Add impersonation header if configured
		if c.ImpersonateAs != "" {
			req.Header.Set("X-AsUser", c.ImpersonateAs)
		}
		return nil
	}

	// Handle traditional authentication methods
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

	// Add impersonation header if impersonation is configured
	if c.ImpersonateAs != "" {
		req.Header.Set("X-AsUser", c.ImpersonateAs)
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

	path := fmt.Sprintf("%s/content/%d", c.APIPrefix, id)

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
	
	path := c.APIPrefix + "/content?" + params.Encode()

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
func (c *Client) GetPageContent(ctx context.Context, id interface{}, format string) (string, error) {
	// Convert the ID to string for the URL path
	var idStr string
	switch v := id.(type) {
	case int:
		idStr = fmt.Sprintf("%d", v)
	case string:
		idStr = v
	default:
		return "", fmt.Errorf("invalid ID type: %T", id)
	}
	
	cacheKey := fmt.Sprintf("page_content:%s:%s", idStr, format)

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

	path := fmt.Sprintf("%s/content/%s?%s", c.APIPrefix, idStr, params.Encode())

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
	
	path := fmt.Sprintf("%s/content/%d/child/page", c.APIPrefix, id)

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
			childID, ok := child.ID.Int()
			if !ok {
				// Skip if the ID is not an integer
				continue
			}
			childDescendants, err := c.getDescendantsRecursive(ctx, childID, depth, 2)
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
		childID, ok := child.ID.Int()
		if !ok {
			// Skip if the ID is not an integer
			continue
		}
		childDescendants, err := c.getDescendantsRecursive(ctx, childID, maxDepth, currentDepth+1)
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
	homepageID, ok := homepage.ID.Int()
	if !ok {
		return nil, fmt.Errorf("homepage ID is not an integer: %v", homepage.ID)
	}
	children, err := c.GetPageChildren(ctx, homepageID)
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
	
	path := fmt.Sprintf("%s/space/%s", c.APIPrefix, key)

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
		
		path := c.APIPrefix + "/content?" + params.Encode()

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
	
	path := c.APIPrefix + "/search?" + params.Encode()

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
			{"id": fmt.Sprintf("%d", *parentID)},
		}
	}

	jsonData, err := json.Marshal(pageData)
	if err != nil {
		return nil, err
	}

	resp, err := c.makeRequest(ctx, "POST", c.APIPrefix+"/content", bytes.NewBuffer(jsonData))
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

	path := fmt.Sprintf("%s/content/%d", c.APIPrefix, id)
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

	path := fmt.Sprintf("%s/content/%d", c.APIPrefix, id)
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

	resp, err := c.makeRequest(ctx, "POST", c.APIPrefix+"/content", bytes.NewBuffer(jsonData))
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

	path := fmt.Sprintf("%s/content/%d/label", c.APIPrefix, pageID)
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

// AuthenticateViaBrowser opens the browser to authenticate the user and captures session cookies
func (c *Client) AuthenticateViaBrowser(ctx context.Context) error {
	loginURL := c.BaseURL.String()

	c.Logger.Info("Opening browser for authentication...")
	c.Logger.Info("Please log in to Confluence in the browser.")
	c.Logger.Info("After successful login, close the browser window.")

	// Open the login URL in the default browser
	err := openBrowser(loginURL)
	if err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	c.Logger.Info("Browser opened. Please complete the login process.")
	c.Logger.Info("Waiting for authentication to complete...")

	// Wait for user to complete authentication
	// In a real implementation, we might have a callback server or polling mechanism
	// For now, we'll just wait for a moment and assume the user has logged in
	time.Sleep(10 * time.Second)

	// Test the authentication by making a simple request to get current user info
	// Confluence Server typically uses /rest/api/user/current
	// Confluence Cloud might use a different endpoint
	testPath := fmt.Sprintf("%s/user/current", c.APIPrefix)
	resp, err := c.makeRequest(ctx, "GET", testPath, nil)
	if err != nil {
		// If /user/current fails, try a simple content request as fallback
		testPath = fmt.Sprintf("%s/content?limit=1", c.APIPrefix)
		resp, err = c.makeRequest(ctx, "GET", testPath, nil)
		if err != nil {
			return fmt.Errorf("authentication test failed: %w", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// Capture cookies from the response
		cookies := resp.Cookies()
		
		// Look for session cookies in the response
		var sessionCookie, samlAuthCookie *http.Cookie
		for _, cookie := range cookies {
			// Common Confluence session cookie names
			if strings.HasPrefix(cookie.Name, "JSESSIONID") || 
			   strings.HasPrefix(cookie.Name, "seraph.rememberme.cookie") ||
			   strings.Contains(strings.ToLower(cookie.Name), "session") {
				sessionCookie = cookie
			}
			
			// Look for SAML auth cookies
			if strings.Contains(strings.ToLower(cookie.Name), "saml") ||
			   strings.Contains(strings.ToLower(cookie.Name), "auth_") ||
			   strings.Contains(strings.ToLower(cookie.Name), "_auth") {
				samlAuthCookie = cookie
			}
		}
		
		// Store the cookies in the client instance
		if sessionCookie != nil {
			c.SessionCookie = fmt.Sprintf("%s=%s", sessionCookie.Name, sessionCookie.Value)
			c.Logger.Info("Session cookie captured: %s", sessionCookie.Name)
		}
		
		if samlAuthCookie != nil {
			c.SAMLAuthCookie = fmt.Sprintf("%s=%s", samlAuthCookie.Name, samlAuthCookie.Value)
			c.Logger.Info("SAML auth cookie captured: %s", samlAuthCookie.Name)
		}
		
		// Also update the configuration to persist the cookies
		currentProfile := viper.GetString("current_profile")
		if currentProfile == "" {
			currentProfile = config.DefaultProfileName
		}
		
		if sessionCookie != nil {
			viper.Set(fmt.Sprintf("profiles.%s.session_cookie", currentProfile), c.SessionCookie)
		}
		if samlAuthCookie != nil {
			viper.Set(fmt.Sprintf("profiles.%s.saml_auth_cookie", currentProfile), c.SAMLAuthCookie)
		}
		
		// Save the updated configuration
		configFile := viper.ConfigFileUsed()
		if configFile != "" {
			if err := viper.WriteConfig(); err != nil {
				c.Logger.Warn("Could not save cookies to config: %v", err)
			} else {
				c.Logger.Info("Cookies saved to configuration")
			}
		} else {
			c.Logger.Warn("No config file found to save cookies")
		}
		
		c.Logger.Info("Authentication successful!")
		return nil
	} else {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed with status %d: %s", resp.StatusCode, string(body))
	}
}

// GetPageWithExpansions retrieves a page with specified expansions
func (c *Client) GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*Page, error) {
	// Convert the ID to string for the URL path
	var idStr string
	switch v := id.(type) {
	case int:
		idStr = fmt.Sprintf("%d", v)
	case string:
		idStr = v
	default:
		return nil, fmt.Errorf("invalid ID type: %T", id)
	}
	
	cacheKey := fmt.Sprintf("page_with_expansions:%s:%s", idStr, strings.Join(expansions, ","))

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

	// Join expansions with commas
	expandParam := strings.Join(expansions, ",")
	params := url.Values{}
	params.Add("expand", expandParam)

	path := fmt.Sprintf("%s/content/%s?%s", c.APIPrefix, idStr, params.Encode())

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

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) error {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin": // macOS
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	return err
}