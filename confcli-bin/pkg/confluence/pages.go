package confluence

import (
	"context"
	"fmt"
	"confcli/pkg/models"
)

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
	return ps.client.GetPage(ctx, id)
}

// GetByTitle retrieves a page by its space key and title
func (ps *PageService) GetByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error) {
	return ps.client.GetPageByTitle(ctx, spaceKey, title)
}

// GetContent retrieves the content of a page in the specified format
func (ps *PageService) GetContent(ctx context.Context, id interface{}, format string) (string, error) {
	return ps.client.GetPageContent(ctx, id, format)
}

// GetChildren retrieves the children of a page
func (ps *PageService) GetChildren(ctx context.Context, id int) ([]models.Page, error) {
	return ps.client.GetPageChildren(ctx, id)
}

// GetDescendants retrieves all descendants of a page up to a certain depth
func (ps *PageService) GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error) {
	return ps.client.GetDescendants(ctx, id, depth)
}

// GetWithExpansions retrieves a page with specified expansions
func (ps *PageService) GetWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error) {
	return ps.client.GetPageWithExpansions(ctx, id, expansions)
}

// Create creates a new page
func (ps *PageService) Create(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error) {
	return ps.client.CreatePage(ctx, spaceKey, parentID, title, content, format)
}

// Update updates an existing page
func (ps *PageService) Update(ctx context.Context, id int, content string, versionComment string) (*models.Page, error) {
	return ps.client.UpdatePage(ctx, id, content, versionComment)
}

// Delete deletes a page
func (ps *PageService) Delete(ctx context.Context, id int) error {
	return ps.client.DeletePage(ctx, id)
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