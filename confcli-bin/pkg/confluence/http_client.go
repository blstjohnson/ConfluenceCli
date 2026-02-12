package confluence

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

	"confcli/pkg/api"
	"confcli/internal/logging"
)

// HTTPClient handles HTTP communication with the Confluence API
type HTTPClient struct {
	BaseURL        *url.URL
	HTTPClient     *http.Client
	AuthType       string
	Token          string
	Username       string
	Password       string
	ImpersonateAs  string // User to impersonate
	UseDomainAuth  bool   // Use current domain user for authentication
	ReadOnly       bool
	Logger         *logging.Logger
	APIPrefix      string // API path prefix (e.g., "/rest/api" for Server, "/api" for Cloud)
	SessionCookie  string // Session cookie for browser-based auth
	SAMLAuthCookie string // SAML auth cookie for identity provider
}

// NewHTTPClient creates a new HTTP client for API communication
func NewHTTPClient(options *api.ClientOptions) (*HTTPClient, error) {
	baseURL, err := url.Parse(options.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Confluence URL: %w", err)
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
	if strings.Contains(strings.ToLower(options.BaseURL), "atlassian.net") {
		// Confluence Cloud URL pattern
		apiPrefix = "/api"
	} else {
		// Assume Confluence Server/Data Center
		apiPrefix = "/rest/api"
	}

	// Create the client instance
	c := &HTTPClient{
		BaseURL:        baseURL,
		HTTPClient:     httpClient,
		AuthType:       options.AuthType,
		Token:          options.Token,
		Username:       options.Username,
		ImpersonateAs:  options.ImpersonateAs,
		UseDomainAuth:  options.UseDomainAuth,
		Password:       options.Password,
		ReadOnly:       options.ReadOnly,
		Logger:         logging.NewLogger(),
		APIPrefix:      apiPrefix,
		SessionCookie:  options.SessionCookie,
		SAMLAuthCookie: options.SAMLAuthCookie,
	}

	// Pre-populate the cookie jar with configured cookies
	c.setCookiesFromConfig()

	// Debug logging to see what cookies were loaded
	c.Logger.Debug("SessionCookie loaded: '%s'", c.SessionCookie)
	c.Logger.Debug("SAMLAuthCookie loaded: '%s'", c.SAMLAuthCookie)

	return c, nil
}

// makeRequest performs an HTTP request to the Confluence API
func (c *HTTPClient) makeRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
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

	// Check if this is a write operation and we's in read-only mode
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
func (c *HTTPClient) setAuthHeader(req *http.Request) error {
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

// setCookiesFromConfig adds configured cookies to the HTTP client's cookie jar
func (c *HTTPClient) setCookiesFromConfig() {
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

// GetPageRaw retrieves a page by its ID (raw API call without business logic)
func (c *HTTPClient) GetPageRaw(ctx context.Context, id int) (*http.Response, error) {
	path := fmt.Sprintf("%s/content/%d", c.APIPrefix, id)
	return c.makeRequest(ctx, "GET", path, nil)
}

// GetPageByTitleRaw retrieves a page by its space key and title (raw API call without business logic)
func (c *HTTPClient) GetPageByTitleRaw(ctx context.Context, spaceKey, title string) (*http.Response, error) {
	params := url.Values{}
	params.Add("space", spaceKey)
	params.Add("title", title)

	path := c.APIPrefix + "/content?" + params.Encode()
	return c.makeRequest(ctx, "GET", path, nil)
}

// GetPageContentRaw retrieves the content of a page (raw API call without business logic)
func (c *HTTPClient) GetPageContentRaw(ctx context.Context, id interface{}, format string) (*http.Response, error) {
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

	// Use only the specific format requested with single expansion
	expansion := fmt.Sprintf("body.%s", format)
	params := url.Values{}
	params.Add("expand", expansion)

	path := fmt.Sprintf("%s/content/%s?%s", c.APIPrefix, idStr, params.Encode())
	return c.makeRequest(ctx, "GET", path, nil)
}

// GetPageChildrenRaw retrieves the children of a page (raw API call without business logic)
func (c *HTTPClient) GetPageChildrenRaw(ctx context.Context, id int) (*http.Response, error) {
	path := fmt.Sprintf("%s/content/%d/child/page", c.APIPrefix, id)
	return c.makeRequest(ctx, "GET", path, nil)
}

// GetSpaceRaw retrieves a space by its key (raw API call without business logic)
func (c *HTTPClient) GetSpaceRaw(ctx context.Context, key string) (*http.Response, error) {
	path := fmt.Sprintf("%s/space/%s", c.APIPrefix, key)
	return c.makeRequest(ctx, "GET", path, nil)
}

// GetAllPagesInSpaceRaw retrieves all pages in a space (raw API call without business logic)
func (c *HTTPClient) GetAllPagesInSpaceRaw(ctx context.Context, spaceKey string) (*http.Response, error) {
	params := url.Values{}
	params.Add("space", spaceKey)
	params.Add("start", "0")
	params.Add("limit", "100")

	path := c.APIPrefix + "/content?" + params.Encode()
	return c.makeRequest(ctx, "GET", path, nil)
}

// SearchRaw searches for pages using CQL (raw API call without business logic)
func (c *HTTPClient) SearchRaw(ctx context.Context, cql string, limit int) (*http.Response, error) {
	params := url.Values{}
	params.Add("cql", cql)
	if limit > 0 {
		params.Add("limit", fmt.Sprintf("%d", limit))
	}

	path := c.APIPrefix + "/search?" + params.Encode()
	return c.makeRequest(ctx, "GET", path, nil)
}

// CreatePageRaw creates a new page (raw API call without business logic)
func (c *HTTPClient) CreatePageRaw(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*http.Response, error) {
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

	return c.makeRequest(ctx, "POST", c.APIPrefix+"/content", bytes.NewBuffer(jsonData))
}

// UpdatePageRaw updates an existing page (raw API call without business logic)
func (c *HTTPClient) UpdatePageRaw(ctx context.Context, id int, content string, versionComment string) (*http.Response, error) {
	if c.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot update page")
	}

	// First get the current page to retrieve its version and title
	resp, err := c.GetPageRaw(ctx, id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get current page: API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var currentPage struct {
		ID      interface{} `json:"id"`
		Title   string      `json:"title"`
		Version struct {
			Number int    `json:"number"`
			Message string `json:"message"`
		} `json:"version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&currentPage); err != nil {
		return nil, err
	}

	// Prepare the request body
	pageData := map[string]interface{}{
		"id":    fmt.Sprintf("%d", id),
		"type":  "page",
		"title": currentPage.Title,
		"version": map[string]interface{}{
			"number":  currentPage.Version.Number + 1, // Increment version number
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
	return c.makeRequest(ctx, "PUT", path, bytes.NewBuffer(jsonData))
}

// DeletePageRaw deletes a page (raw API call without business logic)
func (c *HTTPClient) DeletePageRaw(ctx context.Context, id int) (*http.Response, error) {
	if c.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot delete page")
	}

	path := fmt.Sprintf("%s/content/%d", c.APIPrefix, id)
	return c.makeRequest(ctx, "DELETE", path, nil)
}

// AddCommentRaw adds a comment to a page (raw API call without business logic)
func (c *HTTPClient) AddCommentRaw(ctx context.Context, pageID int, text string, parentCommentID *int) (*http.Response, error) {
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

	return c.makeRequest(ctx, "POST", c.APIPrefix+"/content", bytes.NewBuffer(jsonData))
}

// AddLabelRaw adds a label to a page (raw API call without business logic)
func (c *HTTPClient) AddLabelRaw(ctx context.Context, pageID int, labelName string) (*http.Response, error) {
	if c.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot add label")
	}

	labelData := map[string]interface{}{
		"prefix": "global",
		"name":   labelName,
	}

	jsonData, err := json.Marshal(labelData)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/content/%d/label", c.APIPrefix, pageID)
	return c.makeRequest(ctx, "POST", path, bytes.NewBuffer(jsonData))
}

// GetPageWithExpansionsRaw retrieves a page with specified expansions (raw API call without business logic)
func (c *HTTPClient) GetPageWithExpansionsRaw(ctx context.Context, id interface{}, expansions []string) (*http.Response, error) {
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

	// Join expansions with commas
	expandParam := strings.Join(expansions, ",")
	params := url.Values{}
	params.Add("expand", expandParam)

	path := fmt.Sprintf("%s/content/%s?%s", c.APIPrefix, idStr, params.Encode())
	return c.makeRequest(ctx, "GET", path, nil)
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