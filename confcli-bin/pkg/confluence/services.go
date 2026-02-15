package confluence

import (
	"context"
	"fmt"

	"confcli/pkg/models"
)

// Services provides access to different Confluence API services
type Services struct {
	Page  *PageService
	Space *SpaceService
	Search *SearchService
}

// PageService handles operations related to Confluence pages
type PageService struct {
	client *client
}

// NewPageService creates a new page service
func NewPageService(client *client) *PageService {
	return &PageService{
		client: client,
	}
}

// Get retrieves a page by its ID
func (ps *PageService) Get(ctx context.Context, id int) (*models.Page, error) {
	return ps.client.businessClient.GetPage(ctx, id)
}

// GetByTitle retrieves a page by its space key and title
func (ps *PageService) GetByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error) {
	return ps.client.businessClient.GetPageByTitle(ctx, spaceKey, title)
}

// GetContent retrieves the content of a page in the specified format
func (ps *PageService) GetContent(ctx context.Context, id interface{}, format string) (string, error) {
	return ps.client.businessClient.GetPageContent(ctx, id, format)
}

// GetChildren retrieves the children of a page
func (ps *PageService) GetChildren(ctx context.Context, id int) ([]models.Page, error) {
	return ps.client.businessClient.GetPageChildren(ctx, id)
}

// GetDescendants retrieves all descendants of a page up to a certain depth
func (ps *PageService) GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error) {
	return ps.client.businessClient.GetDescendants(ctx, id, depth)
}

// GetWithExpansions retrieves a page with specified expansions
func (ps *PageService) GetWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error) {
	return ps.client.businessClient.GetPageWithExpansions(ctx, id, expansions)
}

// Create creates a new page
func (ps *PageService) Create(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error) {
	return ps.client.businessClient.CreatePage(ctx, spaceKey, parentID, title, content, format)
}

// Update updates an existing page
func (ps *PageService) Update(ctx context.Context, id int, content string, versionComment string) (*models.Page, error) {
	return ps.client.businessClient.UpdatePage(ctx, id, content, versionComment)
}

// Delete deletes a page
func (ps *PageService) Delete(ctx context.Context, id int) error {
	return ps.client.businessClient.DeletePage(ctx, id)
}

// GetComments retrieves comments for a page
func (ps *PageService) GetComments(ctx context.Context, pageID int) ([]models.Comment, error) {
	return ps.client.businessClient.GetComments(ctx, pageID)
}

// GetLabels retrieves labels for a page
func (ps *PageService) GetLabels(ctx context.Context, pageID int) ([]models.Label, error) {
	return ps.client.businessClient.GetLabels(ctx, pageID)
}

// AddComment adds a comment to a page
func (ps *PageService) AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error) {
	return ps.client.businessClient.AddComment(ctx, pageID, text, parentCommentID)
}

// AddLabel adds a label to a page
func (ps *PageService) AddLabel(ctx context.Context, pageID int, labelName string) error {
	return ps.client.businessClient.AddLabel(ctx, pageID, labelName)
}

// ValidatePage validates page parameters
func (ps *PageService) ValidatePage(id int, space, title, path string) error {
	// Validate inputs
	if id == 0 && title == "" && path == "" {
		return fmt.Errorf("must specify either id, space and title, or path")
	}

	if (space != "" && title == "") || (space == "" && title != "") {
		if path == "" {
			return fmt.Errorf("must specify both space and title together, or use path")
		}
	}

	return nil
}

// SpaceService handles operations related to Confluence spaces
type SpaceService struct {
	client *client
}

// NewSpaceService creates a new space service
func NewSpaceService(client *client) *SpaceService {
	return &SpaceService{
		client: client,
	}
}

// Get retrieves a space by its key
func (ss *SpaceService) Get(ctx context.Context, key string) (*models.Space, error) {
	return ss.client.businessClient.GetSpace(ctx, key)
}

// GetRootPages retrieves the root pages of a space
func (ss *SpaceService) GetRootPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	return ss.client.businessClient.GetSpaceRootPages(ctx, spaceKey)
}

// GetAllPages retrieves all pages in a space
func (ss *SpaceService) GetAllPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	return ss.client.businessClient.GetAllPagesInSpace(ctx, spaceKey)
}