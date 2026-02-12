package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"confcli/pkg/models"
)

// SimpleBusinessClient wraps the HTTP client without caching
type SimpleBusinessClient struct {
	httpClient *HTTPClient
}

// NewSimpleBusinessClient creates a new client without caching
func NewSimpleBusinessClient(httpClient *HTTPClient) *SimpleBusinessClient {
	return &SimpleBusinessClient{
		httpClient: httpClient,
	}
}

// GetPage retrieves a page by its ID
func (sbc *SimpleBusinessClient) GetPage(ctx context.Context, id int) (*models.Page, error) {
	resp, err := sbc.httpClient.GetPageRaw(ctx, id)
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

	return &page, nil
}

// GetPageByTitle retrieves a page by its space key and title
func (sbc *SimpleBusinessClient) GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error) {
	resp, err := sbc.httpClient.GetPageByTitleRaw(ctx, spaceKey, title)
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
		return nil, fmt.Errorf("page with title '%s' in space '%s' not found", title, spaceKey)
	}

	return &result.Results[0], nil
}

// GetPageContent retrieves the content of a page in the specified format
func (sbc *SimpleBusinessClient) GetPageContent(ctx context.Context, id interface{}, format string) (string, error) {
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

	// Use only the specific format requested with single expansion
	expansion := fmt.Sprintf("body.%s", format)
	params := strings.Join([]string{"expand=" + expansion}, "&")
	
	path := fmt.Sprintf("%s/content/%s?%s", sbc.httpClient.APIPrefix, idStr, params)
	
	resp, err := sbc.httpClient.makeRequest(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var page models.Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return "", err
	}

	// Return content in the requested format
	bodyContent, ok := page.Body[format]
	if !ok {
		// If the requested format is not available, return the first available format
		for _, f := range []string{"storage", "view", "export_view", "styled_view"} {
			if content, exists := page.Body[f]; exists {
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

// GetPageChildren retrieves the children of a page
func (sbc *SimpleBusinessClient) GetPageChildren(ctx context.Context, id int) ([]models.Page, error) {
	resp, err := sbc.httpClient.GetPageChildrenRaw(ctx, id)
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

	return result.Results, nil
}

// GetPageWithExpansions retrieves a page with specified expansions
func (sbc *SimpleBusinessClient) GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error) {
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
	params := strings.Join([]string{"expand=" + expandParam}, "&")
	
	path := fmt.Sprintf("%s/content/%s?%s", sbc.httpClient.APIPrefix, idStr, params)
	
	resp, err := sbc.httpClient.makeRequest(ctx, "GET", path, nil)
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

	return &page, nil
}

// GetSpace retrieves a space by its key
func (sbc *SimpleBusinessClient) GetSpace(ctx context.Context, key string) (*models.Space, error) {
	resp, err := sbc.httpClient.GetSpaceRaw(ctx, key)
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

	return &space, nil
}

// GetDescendants retrieves all descendants of a page up to a certain depth
func (sbc *SimpleBusinessClient) GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error) {
	// This is a simplified implementation - a full implementation would require recursion
	// to get all descendants up to the specified depth
	pages := make([]models.Page, 0)

	// Get the initial page (we'll use it to validate the ID exists)
	_, err := sbc.GetPage(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get children of the initial page
	children, err := sbc.GetPageChildren(ctx, id)
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
			childDescendants, err := sbc.getDescendantsRecursive(ctx, childID, depth, 2)
			if err != nil {
				// Log the error but continue with other pages
				continue
			}
			pages = append(pages, childDescendants...)
		}
	}

	return pages, nil
}

// getDescendantsRecursive is a helper function to recursively get descendants
func (sbc *SimpleBusinessClient) getDescendantsRecursive(ctx context.Context, id int, maxDepth, currentDepth int) ([]models.Page, error) {
	if maxDepth > 0 && currentDepth > maxDepth {
		return []models.Page{}, nil
	}

	children, err := sbc.GetPageChildren(ctx, id)
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
		childDescendants, err := sbc.getDescendantsRecursive(ctx, childID, maxDepth, currentDepth+1)
		if err != nil {
			// Log the error but continue with other pages
			continue
		}
		result = append(result, childDescendants...)
	}

	return result, nil
}

// GetSpaceRootPages retrieves the root pages of a space
func (sbc *SimpleBusinessClient) GetSpaceRootPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	// First get the space to find its homepage
	space, err := sbc.GetSpace(ctx, spaceKey)
	if err != nil {
		return nil, err
	}

	// Get the homepage and its children
	homepage, err := sbc.GetPage(ctx, space.HomepageID)
	if err != nil {
		return nil, err
	}

	// Get children of the homepage (these are typically root pages)
	homepageID, ok := homepage.ID.Int()
	if !ok {
		return nil, fmt.Errorf("homepage ID is not an integer: %v", homepage.ID)
	}
	children, err := sbc.GetPageChildren(ctx, homepageID)
	if err != nil {
		return nil, err
	}

	// Add the homepage itself as a root page
	rootPages := append([]models.Page{*homepage}, children...)

	return rootPages, nil
}

// GetAllPagesInSpace retrieves all pages in a space
func (sbc *SimpleBusinessClient) GetAllPagesInSpace(ctx context.Context, spaceKey string) ([]models.Page, error) {
	allPages := make([]models.Page, 0)

	// Use pagination to get all pages
	start := 0
	limit := 100 // Max limit for Confluence API

	for {
		// Make the API call
		resp, err := sbc.httpClient.GetAllPagesInSpaceRaw(ctx, spaceKey)
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
			Results  []models.Page `json:"results"`
			Start    int           `json:"start,omitempty"`
			Limit    int           `json:"limit,omitempty"`
			Size     int           `json:"size,omitempty"`
			Next     string        `json:"_links.next,omitempty"`
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

	return allPages, nil
}

// Search searches for pages using CQL
func (sbc *SimpleBusinessClient) Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error) {
	resp, err := sbc.httpClient.SearchRaw(ctx, cql, limit)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []models.SearchResult `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// CreatePage creates a new page
func (sbc *SimpleBusinessClient) CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error) {
	resp, err := sbc.httpClient.CreatePageRaw(ctx, spaceKey, parentID, title, content, format)
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

	return &newPage, nil
}

// UpdatePage updates an existing page
func (sbc *SimpleBusinessClient) UpdatePage(ctx context.Context, id int, content string, versionComment string) (*models.Page, error) {
	resp, err := sbc.httpClient.UpdatePageRaw(ctx, id, content, versionComment)
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

	return &updatedPage, nil
}

// DeletePage deletes a page
func (sbc *SimpleBusinessClient) DeletePage(ctx context.Context, id int) error {
	resp, err := sbc.httpClient.DeletePageRaw(ctx, id)
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
func (sbc *SimpleBusinessClient) AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error) {
	resp, err := sbc.httpClient.AddCommentRaw(ctx, pageID, text, parentCommentID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var newComment models.Comment
	if err := json.NewDecoder(resp.Body).Decode(&newComment); err != nil {
		return nil, err
	}

	return &newComment, nil
}

// AddLabel adds a label to a page
func (sbc *SimpleBusinessClient) AddLabel(ctx context.Context, pageID int, labelName string) error {
	resp, err := sbc.httpClient.AddLabelRaw(ctx, pageID, labelName)
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
func (sbc *SimpleBusinessClient) AuthenticateViaBrowser(ctx context.Context) error {
	loginURL := sbc.httpClient.BaseURL.String()

	sbc.httpClient.Logger.Info("Opening browser for authentication...")
	sbc.httpClient.Logger.Info("Please log in to Confluence in the browser.")
	sbc.httpClient.Logger.Info("After successful login, close the browser window.")

	// Open the login URL in the default browser
	err := openBrowser(loginURL)
	if err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	sbc.httpClient.Logger.Info("Browser opened. Please complete the login process.")
	sbc.httpClient.Logger.Info("Waiting for authentication to complete...")

	// Poll for authentication with a timeout
	const maxWaitTime = 60 * time.Second // Increased timeout to allow for SAML second factor
	const pollInterval = 5 * time.Second
	startTime := time.Now()

	var sessionCookie, samlAuthCookie *http.Cookie

	for time.Since(startTime) < maxWaitTime {
		// Test the authentication by making a simple request to get current user info
		// Confluence Server typically uses /rest/api/user/current
		// Confluence Cloud might use a different endpoint
		testPath := fmt.Sprintf("%s/user/current", sbc.httpClient.APIPrefix)
		resp, err := sbc.httpClient.makeRequest(ctx, "GET", testPath, nil)
		if err != nil {
			// If /user/current fails, try a simple content request as fallback
			testPath = fmt.Sprintf("%s/content?limit=1", sbc.httpClient.APIPrefix)
			resp, err = sbc.httpClient.makeRequest(ctx, "GET", testPath, nil)
			if err != nil {
				// Authentication may not be complete yet, continue polling
				sbc.httpClient.Logger.Debug("Authentication test request failed, continuing to poll: %v", err)
				time.Sleep(pollInterval)
				continue
			}
		}

		// Capture cookies from the response
		cookies := resp.Cookies()

		// Look for session cookies in the response
		for _, cookie := range cookies {
			// Common Confluence session cookie names
			if strings.HasPrefix(cookie.Name, "JSESSIONID") ||
			   strings.HasPrefix(cookie.Name, "seraph.rememberme.cookie") ||
			   strings.Contains(strings.ToLower(cookie.Name), "session") {
				if sessionCookie == nil || sessionCookie.Value != cookie.Value {
					sessionCookie = cookie
					sbc.httpClient.Logger.Info("Session cookie captured: %s", sessionCookie.Name)
				}
			}

			// Look for SAML auth cookies
			if strings.Contains(strings.ToLower(cookie.Name), "saml") ||
			   strings.Contains(strings.ToLower(cookie.Name), "auth_") ||
			   strings.Contains(strings.ToLower(cookie.Name), "_auth") {
				if samlAuthCookie == nil || samlAuthCookie.Value != cookie.Value {
					samlAuthCookie = cookie
					sbc.httpClient.Logger.Info("SAML auth cookie captured: %s", samlAuthCookie.Name)
				}
			}
		}

		resp.Body.Close()

		// Check if we have both session and SAML cookies (or at least one if SAML is not required)
		if sessionCookie != nil {
			// If we have a session cookie, we might still need to wait for SAML cookies
			// But if we've waited long enough and still don't have SAML cookies, continue anyway
			if samlAuthCookie != nil || time.Since(startTime) > 30*time.Second {
				break
			}
		}

		time.Sleep(pollInterval)
	}

	// Check if we've timed out
	if sessionCookie == nil && time.Since(startTime) >= maxWaitTime {
		return fmt.Errorf("authentication timed out after %v", maxWaitTime)
	}

	// Store the cookies in the client instance
	if sessionCookie != nil {
		sbc.httpClient.SessionCookie = fmt.Sprintf("%s=%s", sessionCookie.Name, sessionCookie.Value)
	}

	if samlAuthCookie != nil {
		sbc.httpClient.SAMLAuthCookie = fmt.Sprintf("%s=%s", samlAuthCookie.Name, samlAuthCookie.Value)
	}

	// Verify that we have at least a session cookie
	if sbc.httpClient.SessionCookie == "" {
		return fmt.Errorf("failed to capture session cookie after authentication")
	}

	sbc.httpClient.Logger.Info("Authentication successful!")
	return nil
}