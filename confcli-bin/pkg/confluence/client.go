package confluence

import (
	"context"
	"fmt"

	"github.com/spf13/viper"

	"confcli/pkg/api"
	"confcli/pkg/models"
	"confcli/internal/config"
)

// client implements the api.Client interface
type client struct {
	businessClient *SimpleBusinessClient
	httpClient     *HTTPClient
	Services       *Services
}

// NewClient creates a new Confluence API client
func NewClient(options *api.ClientOptions) (api.Client, error) {
	// Create HTTP client
	httpClient, err := NewHTTPClient(options)
	if err != nil {
		return nil, err
	}

	// Create simple business client (without caching)
	businessClient := NewSimpleBusinessClient(httpClient)

	// Create the client instance
	c := &client{
		businessClient: businessClient,
		httpClient:     httpClient,
	}

	// Initialize services with the client instance
	c.Services = &Services{
		Page:   NewPageService(c),
		Space:  NewSpaceService(c),
		Search: NewSearchService(c),
	}

	return c, nil
}

// NewClientFromViper creates a new Confluence API client using viper configuration
func NewClientFromViper() (api.Client, error) {
	baseURLStr := viper.GetString("url")
	if baseURLStr == "" {
		return nil, fmt.Errorf("Confluence URL is not configured. Please set it using 'confcli config set url <your_confluence_url>'")
	}

	// Get current profile name
	currentProfile := viper.GetString("current_profile")
	if currentProfile == "" {
		currentProfile = config.DefaultProfileName
	}

	options := &api.ClientOptions{
		BaseURL:        baseURLStr,
		AuthType:       viper.GetString("auth_type"),
		Token:          viper.GetString("token"),
		Username:       viper.GetString("username"),
		ImpersonateAs:  viper.GetString("impersonate_as"),
		Password:       "",
		ReadOnly:       viper.GetBool("read_only"),
		SessionCookie:  viper.GetString(fmt.Sprintf("profiles.%s.session_cookie", currentProfile)),
		SAMLAuthCookie: viper.GetString(fmt.Sprintf("profiles.%s.saml_auth_cookie", currentProfile)),
	}


	return NewClient(options)
}

// GetPage retrieves a page by its ID
func (c *client) GetPage(ctx context.Context, id int) (*models.Page, error) {
	return c.Services.Page.Get(ctx, id)
}

// GetPageByTitle retrieves a page by its space key and title
func (c *client) GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error) {
	return c.Services.Page.GetByTitle(ctx, spaceKey, title)
}

// GetPageContent retrieves the content of a page in the specified format
func (c *client) GetPageContent(ctx context.Context, id interface{}, format string) (string, error) {
	return c.Services.Page.GetContent(ctx, id, format)
}

// GetPageChildren retrieves the children of a page
func (c *client) GetPageChildren(ctx context.Context, id int) ([]models.Page, error) {
	return c.Services.Page.GetChildren(ctx, id)
}

// GetDescendants retrieves all descendants of a page up to a certain depth
func (c *client) GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error) {
	return c.Services.Page.GetDescendants(ctx, id, depth)
}

// GetSpaceRootPages retrieves the root pages of a space
func (c *client) GetSpaceRootPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	return c.Services.Space.GetRootPages(ctx, spaceKey)
}

// GetSpace retrieves a space by its key
func (c *client) GetSpace(ctx context.Context, key string) (*models.Space, error) {
	return c.Services.Space.Get(ctx, key)
}

// GetAllPagesInSpace retrieves all pages in a space
func (c *client) GetAllPagesInSpace(ctx context.Context, spaceKey string) ([]models.Page, error) {
	return c.Services.Space.GetAllPages(ctx, spaceKey)
}

// Search searches for pages using CQL
func (c *client) Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error) {
	return c.Services.Search.Search(ctx, cql, limit)
}

// CreatePage creates a new page
func (c *client) CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error) {
	return c.Services.Page.Create(ctx, spaceKey, parentID, title, content, format)
}

// UpdatePage updates an existing page
func (c *client) UpdatePage(ctx context.Context, id int, content string, versionComment string) (*models.Page, error) {
	return c.Services.Page.Update(ctx, id, content, versionComment)
}

// DeletePage deletes a page
func (c *client) DeletePage(ctx context.Context, id int) error {
	return c.Services.Page.Delete(ctx, id)
}

// AddComment adds a comment to a page
func (c *client) AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error) {
	return c.Services.Page.AddComment(ctx, pageID, text, parentCommentID)
}

// AddLabel adds a label to a page
func (c *client) AddLabel(ctx context.Context, pageID int, labelName string) error {
	return c.Services.Page.AddLabel(ctx, pageID, labelName)
}

// AuthenticateViaBrowser opens the browser to authenticate the user and captures session cookies
func (c *client) AuthenticateViaBrowser(ctx context.Context) error {
	return c.businessClient.AuthenticateViaBrowser(ctx)
}

// GetCurrentUserDetails gets detailed information about the current authenticated user
func (c *client) GetCurrentUserDetails(ctx context.Context) (*models.User, error) {
	return c.businessClient.getCurrentUserDetails(ctx)
}


// GetPageWithExpansions retrieves a page with specified expansions
func (c *client) GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error) {
	return c.Services.Page.GetWithExpansions(ctx, id, expansions)
}

// GetComments retrieves comments for a page
func (c *client) GetComments(ctx context.Context, pageID int) ([]models.Comment, error) {
	return c.Services.Page.GetComments(ctx, pageID)
}

// GetLabels retrieves labels for a page
func (c *client) GetLabels(ctx context.Context, pageID int) ([]models.Label, error) {
	return c.Services.Page.GetLabels(ctx, pageID)
}

// GetHTTPClient returns the underlying HTTP client
func (c *client) GetHTTPClient() interface{} {
	return c.httpClient
}