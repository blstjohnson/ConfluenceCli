package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"confcli/pkg/logging"
)

// HTTPClient handles HTTP communication with the Confluence API
type HTTPClient struct {
	BaseURL        *url.URL
	HTTPClient     *http.Client
	AuthType       string
	Token          string
	Username       string
	Password       string
	ReadOnly       bool
	Logger         *logging.Logger
	APIPrefix      string // API path prefix (e.g., "/rest/api" for Server, "/api" for Cloud)
	SessionCookie  string // Session cookie for browser-based auth
	SAMLAuthCookie string // SAML auth cookie for identity provider
}

// NewHTTPClient creates a new HTTP client for API communication
func NewHTTPClient(options *ClientOptions) (*HTTPClient, error) {
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
	// Set up redirect policy to capture cookies from redirect responses
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     cookieJar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Capture cookies from redirect responses
			// This ensures we capture cookies set during redirects (like SAML auth cookies)
			if len(via) > 0 {
				// Get cookies from the response that caused the redirect
				// This is tricky because we don't have direct access to the response here
				// So we'll just continue with the redirect
			}
			
			// Don't exceed 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
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

// MakeRequest performs an HTTP request to the Confluence API with automatic
// retry for transient failures (timeouts, 5xx, connection reset).
// Up to 3 retries with exponential backoff (1s, 2s, 4s).
//
// JSON-bodied requests go through here; for multipart uploads (attachments)
// use MakeMultipartRequest, which shares the same auth/cookie/retry path.
func (c *HTTPClient) MakeRequest(ctx context.Context, method, path string, queryParams url.Values, body io.Reader) (*http.Response, error) {
	return c.makeRequest(ctx, method, path, queryParams, body, "application/json", nil)
}

// MakeMultipartRequest performs a multipart/form-data request, used for
// attachment uploads. contentType must carry the multipart boundary (obtain
// it from multipart.Writer.FormDataContentType()). The Confluence-required
// "X-Atlassian-Token: no-check" header is added automatically.
func (c *HTTPClient) MakeMultipartRequest(ctx context.Context, method, path string, queryParams url.Values, body io.Reader, contentType string) (*http.Response, error) {
	return c.makeRequest(ctx, method, path, queryParams, body, contentType, map[string]string{
		"X-Atlassian-Token": "no-check",
	})
}

// makeRequest is the shared core for MakeRequest and MakeMultipartRequest.
// bodyContentType is applied as the Content-Type header when a body is
// present; extraHeaders (may be nil) are layered on top.
func (c *HTTPClient) makeRequest(ctx context.Context, method, path string, queryParams url.Values, body io.Reader, bodyContentType string, extraHeaders map[string]string) (*http.Response, error) {
	// Construct the full URL with optional query parameters
	var fullURL *url.URL
	if len(queryParams) > 0 {
		fullURL = c.BaseURL.ResolveReference(&url.URL{Path: path, RawQuery: queryParams.Encode()})
	} else {
		fullURL = c.BaseURL.ResolveReference(&url.URL{Path: path})
	}

	// Check if this is a write operation and we're in read-only mode
	if c.ReadOnly && (method == "POST" || method == "PUT" || method == "DELETE") {
		return nil, fmt.Errorf("read-only mode enabled: cannot perform %s operation", method)
	}

	// Buffer the body so it can be replayed on retries
	hasBody := body != nil
	var bodyBytes []byte
	if hasBody {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	const maxRetries = 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			c.Logger.Debug("Retry %d/%d after %v for %s %s", attempt, maxRetries, backoff, method, fullURL.String())
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Create a fresh body reader for this attempt
		var bodyReader io.Reader
		if hasBody {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), bodyReader)
		if err != nil {
			return nil, err
		}

		// Set headers
		req.Header.Set("Accept", "application/json")
		if hasBody {
			req.Header.Set("Content-Type", bodyContentType)
		}
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}

		// Set authentication header
		if err := c.setAuthHeader(req); err != nil {
			return nil, err
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
			if isRetryableError(err) && attempt < maxRetries {
				c.Logger.Debug("Retryable error on attempt %d: %v", attempt+1, err)
				continue
			}
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

		// Check for retryable status codes (5xx server errors)
		if resp.StatusCode >= 500 && attempt < maxRetries {
			c.Logger.Debug("Retryable status %d on attempt %d for %s %s", resp.StatusCode, attempt+1, method, fullURL.String())
			resp.Body.Close()
			continue
		}

		return resp, nil
	}

	// Should not reach here, but guard against it
	return nil, fmt.Errorf("request failed after %d retries for %s %s", maxRetries, method, path)
}

// isRetryableError checks if a network error is transient and worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Context cancellation is not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Network timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// Connection reset / refused
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	// Fallback: check error message for common transient patterns
	errStr := err.Error()
	return strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe")
}

// setAuthHeader sets the appropriate authentication header based on the auth type
func (c *HTTPClient) setAuthHeader(req *http.Request) error {
	// Handle browser-based authentication (manual cookies)
	if strings.ToLower(c.AuthType) == "browser" {
		// For browser-based auth, cookies are automatically handled by the HTTP client's cookie jar
		// The cookies were pre-loaded in setCookiesFromConfig()
		return nil
	}

	// Handle traditional authentication methods
	switch strings.ToLower(c.AuthType) {
	case "bearer":
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
	default:
		return fmt.Errorf("unsupported auth type: %s", c.AuthType)
	}

	return nil
}

// TriggerAuthAndCaptureCookies makes a request that triggers the authentication flow
// and ensures all cookies (including those set during redirects) are captured
func (c *HTTPClient) TriggerAuthAndCaptureCookies(ctx context.Context, path string) error {
	// Create a new request
	fullURL := c.BaseURL.ResolveReference(&url.URL{Path: path})
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL.String(), nil)
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Accept", "application/json")

	// Set authentication header
	if err := c.setAuthHeader(req); err != nil {
		return err
	}

	// Perform the request - this will follow redirects and store cookies in the jar
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the response body to ensure all redirects are processed
	_, _ = io.ReadAll(resp.Body)

	return nil
}

// EnhancedTriggerAuthAndCaptureCookies makes a request that triggers the authentication flow
// with enhanced cookie capture during redirects
func (c *HTTPClient) EnhancedTriggerAuthAndCaptureCookies(ctx context.Context, path string) error {
	// Create a custom transport that captures cookies during redirects
	transport := c.HTTPClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	// Wrap the transport to capture cookies during redirects
	trackingTransport := &CookieTrackingRoundTripper{
		Transport: transport,
		BaseURL:   c.BaseURL,
		Logger:    c.Logger,
		Client:    c,
	}

	// Create a temporary client with our custom transport
	tempClient := &http.Client{
		Transport: trackingTransport,
		Timeout:   c.HTTPClient.Timeout,
		CheckRedirect: c.HTTPClient.CheckRedirect, // Use the same redirect policy
	}

	// Create a new request
	fullURL := c.BaseURL.ResolveReference(&url.URL{Path: path})
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL.String(), nil)
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Accept", "application/json")

	// Set authentication header
	if err := c.setAuthHeader(req); err != nil {
		return err
	}

	// Perform the request - this will follow redirects and store cookies in the jar
	resp, err := tempClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read the response body to ensure all redirects are processed
	_, _ = io.ReadAll(resp.Body)

	return nil
}

// CookieTrackingRoundTripper wraps an HTTP transport to capture cookies during redirects
type CookieTrackingRoundTripper struct {
	Transport http.RoundTripper
	BaseURL   *url.URL
	Logger    *logging.Logger
	Client    *HTTPClient
}

func (ctr *CookieTrackingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctr.Logger.Debug("Making request to: %s", req.URL.String())

	// Add any existing cookies to the request
	existingCookies := ctr.Client.HTTPClient.Jar.Cookies(req.URL)
	for _, cookie := range existingCookies {
		req.AddCookie(cookie)
	}

	// Perform the request
	resp, err := ctr.Transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Capture cookies from the response
	if resp != nil {
		cookies := resp.Cookies()
		ctr.Logger.Debug("Captured %d cookies from response to: %s", len(cookies), req.URL.String())

		for _, cookie := range cookies {
			ctr.Logger.Debug("Response cookie: %s=%s (domain: %s)", cookie.Name, cookie.Value, cookie.Domain)

			// Store cookies in the main jar
			ctr.Client.HTTPClient.Jar.SetCookies(req.URL, []*http.Cookie{cookie})

			// Also check if this is an IDP-related cookie that we should store separately
			if strings.Contains(strings.ToLower(cookie.Name), "idp_last_account") ||
				strings.Contains(strings.ToLower(cookie.Name), "saml") ||
				strings.Contains(strings.ToLower(cookie.Name), "auth_") ||
				strings.Contains(strings.ToLower(cookie.Name), "_auth") ||
				strings.Contains(strings.ToLower(cookie.Name), "idp") ||
				strings.Contains(strings.ToLower(cookie.Name), "sso") {
				
				ctr.Client.SAMLAuthCookie = fmt.Sprintf("%s=%s", cookie.Name, cookie.Value)
				ctr.Logger.Info("Captured SAML/IDP auth cookie: %s", cookie.Name)
			}
		}
	}

	return resp, err
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
	return c.MakeRequest(ctx, "GET", path, nil, nil)
}

// GetPageByTitleRaw retrieves a page by its space key and title (raw API call without business logic)
func (c *HTTPClient) GetPageByTitleRaw(ctx context.Context, spaceKey, title string) (*http.Response, error) {
	params := url.Values{}
	params.Add("space", spaceKey)
	params.Add("title", title)

	path := c.APIPrefix + "/content"
	return c.MakeRequest(ctx, "GET", path, params, nil)
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

	path := fmt.Sprintf("%s/content/%s", c.APIPrefix, idStr)
	return c.MakeRequest(ctx, "GET", path, params, nil)
}

// GetPageChildrenRaw retrieves the children of a page (raw API call without business logic)
func (c *HTTPClient) GetPageChildrenRaw(ctx context.Context, id int) (*http.Response, error) {
	path := fmt.Sprintf("%s/content/%d/child/page", c.APIPrefix, id)
	return c.MakeRequest(ctx, "GET", path, nil, nil)
}

// GetSpaceRaw retrieves a space by its key (raw API call without business logic)
func (c *HTTPClient) GetSpaceRaw(ctx context.Context, key string) (*http.Response, error) {
	path := fmt.Sprintf("%s/space/%s", c.APIPrefix, key)
	return c.MakeRequest(ctx, "GET", path, nil, nil)
}

// GetAllPagesInSpaceRaw retrieves all pages in a space (raw API call without business logic)
func (c *HTTPClient) GetAllPagesInSpaceRaw(ctx context.Context, spaceKey string) (*http.Response, error) {
	params := url.Values{}
	params.Add("space", spaceKey)
	params.Add("start", "0")
	params.Add("limit", "100")

	path := c.APIPrefix + "/content"
	return c.MakeRequest(ctx, "GET", path, params, nil)
}

// SearchRaw searches for pages using CQL (raw API call without business logic)
func (c *HTTPClient) SearchRaw(ctx context.Context, cql string, limit int) (*http.Response, error) {
	params := url.Values{}
	params.Add("cql", cql)
	if limit > 0 {
		params.Add("limit", fmt.Sprintf("%d", limit))
	}

	path := c.APIPrefix + "/search"
	return c.MakeRequest(ctx, "GET", path, params, nil)
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

	return c.MakeRequest(ctx, "POST", c.APIPrefix+"/content", nil, bytes.NewBuffer(jsonData))
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
	return c.MakeRequest(ctx, "PUT", path, nil, bytes.NewBuffer(jsonData))
}

// DeletePageRaw deletes a page (raw API call without business logic)
func (c *HTTPClient) DeletePageRaw(ctx context.Context, id int) (*http.Response, error) {
	if c.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot delete page")
	}

	path := fmt.Sprintf("%s/content/%d", c.APIPrefix, id)
	return c.MakeRequest(ctx, "DELETE", path, nil, nil)
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

	return c.MakeRequest(ctx, "POST", c.APIPrefix+"/content", nil, bytes.NewBuffer(jsonData))
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
	return c.MakeRequest(ctx, "POST", path, nil, bytes.NewBuffer(jsonData))
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

	path := fmt.Sprintf("%s/content/%s", c.APIPrefix, idStr)
	return c.MakeRequest(ctx, "GET", path, params, nil)
}

// GetCommentsRaw retrieves comments for a page (raw API call without business logic)
func (c *HTTPClient) GetCommentsRaw(ctx context.Context, pageID int) (*http.Response, error) {
	path := fmt.Sprintf("%s/content/%d/comment", c.APIPrefix, pageID)
	return c.MakeRequest(ctx, "GET", path, nil, nil)
}

// GetLabelsRaw retrieves labels for a page (raw API call without business logic)
func (c *HTTPClient) GetLabelsRaw(ctx context.Context, pageID int) (*http.Response, error) {
	path := fmt.Sprintf("%s/content/%d/label", c.APIPrefix, pageID)
	return c.MakeRequest(ctx, "GET", path, nil, nil)
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

// GetBaseURL returns the base URL of the HTTP client
func (c *HTTPClient) GetBaseURL() string {
	return c.BaseURL.String()
}

// OpenBrowser opens the specified URL in the default browser
func OpenBrowser(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	return cmd.Start()
}

// GetAPIPrefix returns the API prefix of the HTTP client
func (c *HTTPClient) GetAPIPrefix() string {
	return c.APIPrefix
}