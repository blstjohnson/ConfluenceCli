package repositories

import (
	"context"

	"confcli/pkg/api"
	"confcli/pkg/models"
)

// HTTPSpaceRepository implements the SpaceRepository interface using API extensions
type HTTPSpaceRepository struct {
	spaceExtension *api.SpaceExtension
}

// NewHTTPSpaceRepository creates a new HTTP space repository
func NewHTTPSpaceRepository(client *api.HTTPClient) *HTTPSpaceRepository {
	return &HTTPSpaceRepository{
		spaceExtension: api.NewSpaceExtension(client),
	}
}

// Get retrieves a space by its key
func (r *HTTPSpaceRepository) Get(ctx context.Context, key string) (*models.Space, error) {
	resp, err := r.spaceExtension.FetchSpace(ctx, &api.FetchSpaceRequest{
		SpaceKey: key,
	})
	if err != nil {
		return nil, err
	}
	return resp.Space, nil
}

// GetRootPages retrieves root-level pages in a space
func (r *HTTPSpaceRepository) GetRootPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	resp, err := r.spaceExtension.FetchRootPagesInSpace(ctx, &api.FetchRootPagesInSpaceRequest{
		SpaceKey: spaceKey,
	})
	if err != nil {
		return nil, err
	}
	return resp.Pages, nil
}

// GetAllPages retrieves all pages in a space with pagination support
// This method loads all pages into memory - use GetAllPagesIterative for large spaces
func (r *HTTPSpaceRepository) GetAllPages(ctx context.Context, spaceKey string) ([]models.Page, error) {
	allPages := make([]models.Page, 0)
	start := 0
	limit := 100

	for {
		resp, err := r.spaceExtension.FetchAllPagesInSpace(ctx, &api.FetchAllPagesInSpaceRequest{
			SpaceKey: spaceKey,
			Start:    start,
			Limit:    limit,
		})
		if err != nil {
			return nil, err
		}

		allPages = append(allPages, resp.Pages...)

		if !resp.HasMore {
			break
		}

		start += limit
	}

	return allPages, nil
}

// GetAllPagesIterative retrieves all pages in a space with pagination support,
// processing each batch through the provided handler before fetching the next batch.
// This approach saves memory by not loading all pages at once.
func (r *HTTPSpaceRepository) GetAllPagesIterative(ctx context.Context, spaceKey string, handler PageBatchHandler) error {
	start := 0
	limit := 100

	for {
		resp, err := r.spaceExtension.FetchAllPagesInSpace(ctx, &api.FetchAllPagesInSpaceRequest{
			SpaceKey: spaceKey,
			Start:    start,
			Limit:    limit,
		})
		if err != nil {
			return err
		}

		// Process this batch before fetching the next
		if len(resp.Pages) > 0 {
			if err := handler(resp.Pages); err != nil {
				return err
			}
		}

		if !resp.HasMore {
			break
		}

		start += limit
	}

	return nil
}
