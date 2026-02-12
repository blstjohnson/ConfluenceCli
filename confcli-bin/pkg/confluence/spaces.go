package confluence

import (
	"context"
	"confcli/pkg/models"
)

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
	return ss.client.GetSpace(ctx, key)
}

// GetRootPages retrieves the root pages of a space
func (ss *SpaceService) GetRootPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	return ss.client.GetSpaceRootPages(ctx, spaceKey)
}

// GetAllPages retrieves all pages in a space
func (ss *SpaceService) GetAllPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	return ss.client.GetAllPagesInSpace(ctx, spaceKey)
}