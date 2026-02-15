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
	sbc.httpClient.Logger.Info("After successful login, return to this terminal.")

	// Open the login URL in the default browser
	err := openBrowser(loginURL)
	if err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	fmt.Println("\nPlease complete the login process in your browser.")
	fmt.Println("Once you've logged in, press Enter here to continue...")

	// Wait for user to press Enter
	var input string
	fmt.Scanln(&input)

	// First, try the standard approach
	err = sbc.standardCookieCapture(ctx)
	if err != nil {
		sbc.httpClient.Logger.Info("Standard cookie capture failed: %v. Trying enhanced approach...", err)
		
		// If standard approach fails, return an error suggesting to configure token
		return fmt.Errorf("authentication failed - please configure your token or use browser-based authentication")
	}

	// Now validate the authentication by making a test request with browser auth type
	originalAuthType := sbc.httpClient.AuthType
	sbc.httpClient.AuthType = "browser"

	defer func() {
		sbc.httpClient.AuthType = originalAuthType
	}()

	// Try to get current user info to validate the session
	userInfo, err := sbc.getCurrentUserInfo(ctx)
	if err != nil {
		sbc.httpClient.Logger.Info("Standard validation failed: %v. Trying enhanced approach...", err)
		return fmt.Errorf("authentication failed - please configure your token or use browser-based authentication")
	}

	// For SSO setups, sometimes we get anonymous initially, so let's try a different endpoint
	if userInfo.DisplayName == "Anonymous" || userInfo.DisplayName == "" {
		// Try getting user info from a different endpoint that might work better with SSO
		userInfo, err = sbc.getCurrentUserDetails(ctx)
		if err != nil || userInfo.DisplayName == "Anonymous" || userInfo.DisplayName == "" {
			sbc.httpClient.Logger.Info("Standard validation failed with anonymous user. Trying enhanced approach...")
			return fmt.Errorf("authentication failed - please configure your token or use browser-based authentication")
		}
	}

	sbc.httpClient.Logger.Info("Authentication successful! Logged in as: %s", userInfo.DisplayName)
	return nil
}

// standardCookieCapture performs the original cookie capture approach
func (sbc *SimpleBusinessClient) standardCookieCapture(ctx context.Context) error {
	// Force a request to the Confluence instance to trigger cookie setting in the HTTP client
	// This is key for capturing cookies that were already set in the browser
	testPaths := []string{
		fmt.Sprintf("%s/", sbc.httpClient.APIPrefix), // Root path
		fmt.Sprintf("%s/plugins/servlet/no.kantega.saml", sbc.httpClient.APIPrefix), // SAML plugin path
		fmt.Sprintf("%s/plugins/servlet/saml", sbc.httpClient.APIPrefix), // Alternative SAML path
		"/", // Root of the domain
	}

	// Try multiple paths to ensure we capture all relevant cookies
	// Use the new method that properly handles redirects and captures cookies
	for _, path := range testPaths {
		err := sbc.httpClient.TriggerAuthAndCaptureCookies(ctx, path)
		if err != nil {
			// Continue to next path if this one fails
			continue
		}
		// Small delay to allow cookies to be processed
		time.Sleep(100 * time.Millisecond)
	}

	// Check if we captured any cookies from the request
	cookies := sbc.httpClient.HTTPClient.Jar.Cookies(sbc.httpClient.BaseURL)
	if len(cookies) > 0 {
		// Look for session and SAML cookies in the jar
		for _, cookie := range cookies {
			if strings.HasPrefix(cookie.Name, "JSESSIONID") ||
				strings.HasPrefix(cookie.Name, "seraph.rememberme.cookie") ||
				strings.Contains(strings.ToLower(cookie.Name), "session") {

				sbc.httpClient.SessionCookie = fmt.Sprintf("%s=%s", cookie.Name, cookie.Value)
				sbc.httpClient.Logger.Info("Captured session cookie: %s", cookie.Name)
			}

			if strings.Contains(strings.ToLower(cookie.Name), "saml") ||
				strings.Contains(strings.ToLower(cookie.Name), "auth_") ||
				strings.Contains(strings.ToLower(cookie.Name), "_auth") ||
				strings.Contains(strings.ToLower(cookie.Name), "idp_last_account") {

				sbc.httpClient.SAMLAuthCookie = fmt.Sprintf("%s=%s", cookie.Name, cookie.Value)
				sbc.httpClient.Logger.Info("Captured SAML auth cookie: %s", cookie.Name)
			}
		}
	}
	
	return nil
}

// enhancedCookieCapture performs enhanced cookie capture for SSO/IDP scenarios
func (sbc *SimpleBusinessClient) enhancedCookieCapture(ctx context.Context) error {
	sbc.httpClient.Logger.Info("Enhanced cookie capture for SSO/IDP scenario not available - please configure your token or use browser-based authentication")
	
	return fmt.Errorf("enhanced authentication not available - please configure your token or use browser-based authentication")
}

// hasValidSession checks if we already have valid session cookies
func (sbc *SimpleBusinessClient) hasValidSession() bool {
	// If we already have session cookies, try to validate them
	if sbc.httpClient.SessionCookie != "" {
		// Create a temporary request to validate the session
		testPath := fmt.Sprintf("%s/user/current", sbc.httpClient.APIPrefix)

		// Temporarily set auth type to browser to use cookies
		originalAuthType := sbc.httpClient.AuthType
		sbc.httpClient.AuthType = "browser"

		req, err := http.NewRequest("GET", sbc.httpClient.BaseURL.String()+testPath, nil)
		if err != nil {
			sbc.httpClient.AuthType = originalAuthType
			return false
		}

		// Apply authentication headers/cookies
		if err := sbc.httpClient.setAuthHeader(req); err != nil {
			sbc.httpClient.AuthType = originalAuthType
			return false
		}

		// Perform the request manually to check session validity
		resp, err := sbc.httpClient.HTTPClient.Do(req)
		sbc.httpClient.AuthType = originalAuthType // Restore original auth type

		if err != nil {
			return false
		}
		defer resp.Body.Close()

		// If we get a successful response, the session is valid
		return resp.StatusCode == http.StatusOK
	}

	return false
}

// getCurrentUserInfo gets information about the current authenticated user
func (sbc *SimpleBusinessClient) getCurrentUserInfo(ctx context.Context) (*models.User, error) {
	testPath := fmt.Sprintf("%s/user/current", sbc.httpClient.APIPrefix)
	resp, err := sbc.httpClient.makeRequest(ctx, "GET", testPath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info, status: %d", resp.StatusCode)
	}

	var userInfo models.User
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

// GetComments retrieves comments for a page
func (sbc *SimpleBusinessClient) GetComments(ctx context.Context, pageID int) ([]models.Comment, error) {
	resp, err := sbc.httpClient.GetCommentsRaw(ctx, pageID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []models.Comment `json:"results"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// GetLabels retrieves labels for a page
func (sbc *SimpleBusinessClient) GetLabels(ctx context.Context, pageID int) ([]models.Label, error) {
	resp, err := sbc.httpClient.GetLabelsRaw(ctx, pageID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []models.Label `json:"results"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Results, nil
}


// getCurrentUserDetails gets more detailed information about the current authenticated user
// This uses a different endpoint that might work better with SSO setups
func (sbc *SimpleBusinessClient) getCurrentUserDetails(ctx context.Context) (*models.User, error) {
	// Try using the current user endpoint with expanded fields
	testPath := fmt.Sprintf("%s/user/current?expand=details.personal", sbc.httpClient.APIPrefix)
	resp, err := sbc.httpClient.makeRequest(ctx, "GET", testPath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Try a different endpoint that might work with SSO
		testPath = fmt.Sprintf("%s/myself", sbc.httpClient.APIPrefix)
		resp, err = sbc.httpClient.makeRequest(ctx, "GET", testPath, nil)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to get user info from alternative endpoint, status: %d", resp.StatusCode)
		}
	}

	var userInfo models.User
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

