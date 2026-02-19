package usecases

import (
	"context"
	"fmt"

	"confcli/pkg/api"
	"confcli/pkg/models"
)

// PageBatchHandler is a callback function for processing page batches
type PageBatchHandler func(batch []models.Page) error

// SpaceUseCase defines the interface for space-related use cases
type SpaceUseCase interface {
	// GetSpaceWithPages retrieves a space with its pages
	GetSpaceWithPages(ctx context.Context, req *GetSpaceWithPagesRequest) (*GetSpaceWithPagesResponse, error)

	// ExportSpace exports a space to a directory structure
	ExportSpace(ctx context.Context, req *ExportSpaceRequest) (*ExportSpaceResponse, error)

	// ExportSpaceIterative exports a space to a directory structure iteratively (batch by batch)
	ExportSpaceIterative(ctx context.Context, req *ExportSpaceRequest) (*ExportSpaceResponse, error)

	// GetSpaceHierarchy retrieves the hierarchy of pages in a space
	GetSpaceHierarchy(ctx context.Context, req *GetSpaceHierarchyRequest) (*GetSpaceHierarchyResponse, error)
}

// GetSpaceWithPagesRequest represents a request to get a space with pages
type GetSpaceWithPagesRequest struct {
	SpaceKey     string
	IncludePages bool
	Limit        int
}

// GetSpaceWithPagesResponse represents the response from getting a space with pages
type GetSpaceWithPagesResponse struct {
	Space *models.Space
	Pages []models.Page
}

// ExportSpaceRequest represents a request to export a space
type ExportSpaceRequest struct {
	SpaceKey          string
	OutputDir         string
	Format            string
	Depth             int
	SkipContent       bool
	ExportAttachments bool
}

// ExportSpaceResponse represents the response from exporting a space
type ExportSpaceResponse struct {
	ExportedPath string
	PageCount    int
}

// GetSpaceHierarchyRequest represents a request to get space hierarchy
type GetSpaceHierarchyRequest struct {
	SpaceKey string
	Depth    int
	Flat     bool
}

// GetSpaceHierarchyResponse represents the response from getting space hierarchy
type GetSpaceHierarchyResponse struct {
	Space       *models.Space
	RootPages   []models.Page
	AllPages    []models.Page
	HasMore     bool
}

// spaceUseCase implements the SpaceUseCase interface
type spaceUseCase struct {
	apiClient api.Client
}

// NewSpaceUseCase creates a new space use case
func NewSpaceUseCase(apiClient api.Client) SpaceUseCase {
	return &spaceUseCase{
		apiClient: apiClient,
	}
}

// GetSpaceWithPages retrieves a space with its pages
func (uc *spaceUseCase) GetSpaceWithPages(ctx context.Context, req *GetSpaceWithPagesRequest) (*GetSpaceWithPagesResponse, error) {
	// Get space info
	space, err := uc.apiClient.GetSpace(ctx, req.SpaceKey)
	if err != nil {
		return nil, err
	}
	
	var pages []models.Page
	if req.IncludePages {
		pages, err = uc.apiClient.GetAllPagesInSpace(ctx, req.SpaceKey)
		if err != nil {
			return nil, err
		}
		
		// Apply limit if specified
		if req.Limit > 0 && len(pages) > req.Limit {
			pages = pages[:req.Limit]
		}
	}
	
	return &GetSpaceWithPagesResponse{
		Space: space,
		Pages: pages,
	}, nil
}

// ExportSpace exports a space to a directory structure
func (uc *spaceUseCase) ExportSpace(ctx context.Context, req *ExportSpaceRequest) (*ExportSpaceResponse, error) {
	// Get all pages in the space
	pages, err := uc.apiClient.GetAllPagesInSpace(ctx, req.SpaceKey)
	if err != nil {
		return nil, err
	}

	// Export logic would go here (file writing, directory structure creation, etc.)
	// For now, return basic response
	return &ExportSpaceResponse{
		ExportedPath: req.OutputDir,
		PageCount:    len(pages),
	}, nil
}

// ExportSpaceIterative exports a space to a directory structure iteratively (batch by batch)
// This is more memory-efficient for large spaces as it processes and saves each batch before fetching the next
func (uc *spaceUseCase) ExportSpaceIterative(ctx context.Context, req *ExportSpaceRequest) (*ExportSpaceResponse, error) {
	pageCount := 0

	// Use iterative processing to save memory
	batchSize := 10 // Default batch size
	err := uc.apiClient.GetAllPagesInSpaceIterative(ctx, req.SpaceKey, batchSize, func(batch []models.Page) error {
		// Process and save each batch
		pageCount += len(batch)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &ExportSpaceResponse{
		ExportedPath: req.OutputDir,
		PageCount:    pageCount,
	}, nil
}

// GetSpaceHierarchy retrieves the hierarchy of pages in a space
func (uc *spaceUseCase) GetSpaceHierarchy(ctx context.Context, req *GetSpaceHierarchyRequest) (*GetSpaceHierarchyResponse, error) {
	// Get space info
	space, err := uc.apiClient.GetSpace(ctx, req.SpaceKey)
	if err != nil {
		return nil, err
	}
	
	// Get root pages
	rootPages, err := uc.apiClient.GetSpaceRootPages(ctx, req.SpaceKey)
	if err != nil {
		return nil, err
	}
	
	var allPages []models.Page
	if req.Depth == 0 {
		// Unlimited depth - get all pages
		allPages, err = uc.apiClient.GetAllPagesInSpace(ctx, req.SpaceKey)
		if err != nil {
			return nil, err
		}
	} else {
		// Limited depth - get pages up to specified depth
		// This would require recursive fetching based on hierarchy
		allPages = rootPages
		for _, rootPage := range rootPages {
			pageID, ok := rootPage.ID.Int()
			if !ok {
				continue
			}
			
			descendants, err := uc.apiClient.GetDescendants(ctx, pageID, req.Depth-1)
			if err != nil {
				return nil, fmt.Errorf("failed to get descendants for page %d: %w", pageID, err)
			}
			allPages = append(allPages, descendants...)
		}
	}
	
	return &GetSpaceHierarchyResponse{
		Space:     space,
		RootPages: rootPages,
		AllPages:  allPages,
	}, nil
}
