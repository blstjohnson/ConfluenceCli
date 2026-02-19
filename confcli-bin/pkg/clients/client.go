package clients

import (
	"context"
	"fmt"

	"github.com/spf13/viper"

	"confcli/pkg/api"
	"confcli/pkg/models"
	"confcli/pkg/config"
	"confcli/pkg/repositories"
	"confcli/pkg/services"
)

// client implements the api.Client interface
type client struct {
	pageService   services.PageServiceInterface
	spaceService  services.SpaceServiceInterface
	searchService services.SearchServiceInterface
}

// NewClient creates a new Confluence API client
func NewClient(options *api.ClientOptions) (api.Client, error) {
	// Create HTTP client
	httpClient, err := api.NewHTTPClient(options)
	if err != nil {
		return nil, err
	}

	// Create repositories
	pageRepo := repositories.NewHTTPPageRepository(httpClient)
	spaceRepo := repositories.NewHTTPSpaceRepository(httpClient)
	searchRepo := repositories.NewHTTPSearchRepository(httpClient)

	// Create services
	pageService := services.NewPageService(pageRepo)
	spaceService := services.NewSpaceService(spaceRepo)
	searchService := services.NewSearchService(searchRepo)

	// Create the client instance
	c := &client{
		pageService:   pageService,
		spaceService:  spaceService,
		searchService: searchService,
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
		Password:       "",
		ReadOnly:       viper.GetBool("read_only"),
		SessionCookie:  viper.GetString(fmt.Sprintf("profiles.%s.session_cookie", currentProfile)),
		SAMLAuthCookie: viper.GetString(fmt.Sprintf("profiles.%s.saml_auth_cookie", currentProfile)),
	}


	return NewClient(options)
}

// GetPage retrieves a page by its ID
func (c *client) GetPage(ctx context.Context, id int) (*models.Page, error) {
	return c.pageService.GetPage(ctx, id)
}

// GetPageByTitle retrieves a page by its space key and title
func (c *client) GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error) {
	return c.pageService.GetPageByTitle(ctx, spaceKey, title)
}

// GetPageContent retrieves the content of a page in the specified format
func (c *client) GetPageContent(ctx context.Context, id interface{}, format string, version int) (string, error) {
	return c.pageService.GetPageContent(ctx, id, format, version)
}

// GetPageChildren retrieves the children of a page
func (c *client) GetPageChildren(ctx context.Context, id int) ([]models.Page, error) {
	return c.pageService.GetPageChildren(ctx, id)
}

// GetDescendants retrieves all descendants of a page up to a certain depth
func (c *client) GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error) {
	return c.pageService.GetDescendants(ctx, id, depth)
}

// GetSpaceRootPages retrieves the root pages of a space
func (c *client) GetSpaceRootPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	return c.spaceService.GetRootPages(ctx, spaceKey)
}

// GetSpace retrieves a space by its key
func (c *client) GetSpace(ctx context.Context, key string) (*models.Space, error) {
	return c.spaceService.Get(ctx, key)
}

// GetAllPagesInSpace retrieves all pages in a space
func (c *client) GetAllPagesInSpace(ctx context.Context, spaceKey string) ([]models.Page, error) {
	return c.spaceService.GetAllPages(ctx, spaceKey)
}

// GetAllPagesInSpaceIterative retrieves all pages in a space iteratively, processing each batch
func (c *client) GetAllPagesInSpaceIterative(ctx context.Context, spaceKey string, batchSize int, handler func(batch []models.Page) error) error {
	return c.spaceService.GetAllPagesIterative(ctx, spaceKey, batchSize, handler)
}

// Search searches for pages using CQL
func (c *client) Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error) {
	return c.searchService.Search(ctx, cql, limit)
}

// CreatePage creates a new page
func (c *client) CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error) {
	return c.pageService.CreatePage(ctx, spaceKey, parentID, title, content, format)
}

// UpdatePage updates an existing page
func (c *client) UpdatePage(ctx context.Context, id int, content string, versionComment string, format string) (*models.Page, error) {
	return c.pageService.UpdatePage(ctx, id, content, versionComment, format)
}

// DeletePage deletes a page
func (c *client) DeletePage(ctx context.Context, id int) error {
	return c.pageService.DeletePage(ctx, id)
}

// AddComment adds a comment to a page
func (c *client) AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error) {
	return c.pageService.AddComment(ctx, pageID, text, parentCommentID)
}

// AddLabel adds a label to a page
func (c *client) AddLabel(ctx context.Context, pageID int, labelName string) error {
	return c.pageService.AddLabel(ctx, pageID, labelName)
}

// AuthenticateViaBrowser opens the browser to authenticate the user and captures session cookies
func (c *client) AuthenticateViaBrowser(ctx context.Context) error {
	// This is a simplified implementation - in a real scenario, we'd need to implement
	// authentication in the HTTP client
	return fmt.Errorf("browser authentication not implemented in refactored client")
}

// GetCurrentUserDetails gets detailed information about the current authenticated user
func (c *client) GetCurrentUserDetails(ctx context.Context) (*models.User, error) {
	// This would need to be implemented in the HTTP client
	return nil, fmt.Errorf("get current user details not implemented in refactored client")
}


// GetPageWithExpansions retrieves a page with specified expansions
func (c *client) GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error) {
	return c.pageService.GetPageWithExpansions(ctx, id, expansions)
}

// GetComments retrieves comments for a page
func (c *client) GetComments(ctx context.Context, pageID int) ([]models.Comment, error) {
	return c.pageService.GetComments(ctx, pageID)
}

// GetLabels retrieves labels for a page
func (c *client) GetLabels(ctx context.Context, pageID int) ([]models.Label, error) {
	return c.pageService.GetLabels(ctx, pageID)
}

// GetHTTPClient returns the underlying HTTP client
func (c *client) GetHTTPClient() interface{} {
	// This would need to be implemented based on the actual HTTP client
	// For now, returning nil
	return nil
}