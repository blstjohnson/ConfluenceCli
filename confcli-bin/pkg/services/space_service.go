package services

import (
	"context"
	"fmt"

	"confcli/pkg/models"
	"confcli/pkg/repositories"
)

// SpaceServiceInterface defines the interface for space-related business operations
type SpaceServiceInterface interface {
	Get(ctx context.Context, key string) (*models.Space, error)
	GetRootPages(ctx context.Context, spaceKey string) ([]models.Page, error)
	GetAllPages(ctx context.Context, spaceKey string) ([]models.Page, error)
}

// SpaceService implements the SpaceServiceInterface with business logic
type SpaceService struct {
	repository repositories.SpaceRepository
}

// NewSpaceService creates a new space service
func NewSpaceService(repository repositories.SpaceRepository) *SpaceService {
	return &SpaceService{
		repository: repository,
	}
}

// Get retrieves a space by its key with validation
func (ss *SpaceService) Get(ctx context.Context, key string) (*models.Space, error) {
	if key == "" {
		return nil, fmt.Errorf("space key cannot be empty")
	}
	return ss.repository.Get(ctx, key)
}

// GetRootPages retrieves root-level pages in a space with validation
func (ss *SpaceService) GetRootPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	if spaceKey == "" {
		return nil, fmt.Errorf("space key cannot be empty")
	}
	return ss.repository.GetRootPages(ctx, spaceKey)
}

// GetAllPages retrieves all pages in a space with validation
func (ss *SpaceService) GetAllPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	if spaceKey == "" {
		return nil, fmt.Errorf("space key cannot be empty")
	}
	return ss.repository.GetAllPages(ctx, spaceKey)
}
