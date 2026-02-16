package usecases

import (
	"context"

	"confcli/pkg/api"
	"confcli/pkg/models"
)

// SearchUseCase defines the interface for search-related use cases
type SearchUseCase interface {
	// SearchPages searches for pages using CQL
	SearchPages(ctx context.Context, req *SearchPagesRequest) (*SearchPagesResponse, error)
	
	// SearchByTitle searches for pages by title
	SearchByTitle(ctx context.Context, req *SearchByTitleRequest) (*SearchByTitleResponse, error)
	
	// SearchByLabel searches for pages by label
	SearchByLabel(ctx context.Context, req *SearchByLabelRequest) (*SearchByLabelResponse, error)
}

// SearchPagesRequest represents a request to search for pages
type SearchPagesRequest struct {
	CQL   string
	Limit int
}

// SearchPagesResponse represents the response from searching for pages
type SearchPagesResponse struct {
	Results []models.SearchResult
	Total   int
	HasMore bool
}

// SearchByTitleRequest represents a request to search by title
type SearchByTitleRequest struct {
	SpaceKey string
	Title    string
}

// SearchByTitleResponse represents the response from searching by title
type SearchByTitleResponse struct {
	Page *models.Page
}

// SearchByLabelRequest represents a request to search by label
type SearchByLabelRequest struct {
	Label  string
	SpaceKey string
	Limit  int
}

// SearchByLabelResponse represents the response from searching by label
type SearchByLabelResponse struct {
	Pages []models.Page
}

// searchUseCase implements the SearchUseCase interface
type searchUseCase struct {
	apiClient api.Client
}

// NewSearchUseCase creates a new search use case
func NewSearchUseCase(apiClient api.Client) SearchUseCase {
	return &searchUseCase{
		apiClient: apiClient,
	}
}

// SearchPages searches for pages using CQL
func (uc *searchUseCase) SearchPages(ctx context.Context, req *SearchPagesRequest) (*SearchPagesResponse, error) {
	results, err := uc.apiClient.Search(ctx, req.CQL, req.Limit)
	if err != nil {
		return nil, err
	}
	
	return &SearchPagesResponse{
		Results: results,
		Total:   len(results),
		HasMore: len(results) == req.Limit,
	}, nil
}

// SearchByTitle searches for pages by title
func (uc *searchUseCase) SearchByTitle(ctx context.Context, req *SearchByTitleRequest) (*SearchByTitleResponse, error) {
	page, err := uc.apiClient.GetPageByTitle(ctx, req.SpaceKey, req.Title)
	if err != nil {
		return nil, err
	}
	
	return &SearchByTitleResponse{
		Page: page,
	}, nil
}

// SearchByLabel searches for pages by label
func (uc *searchUseCase) SearchByLabel(ctx context.Context, req *SearchByLabelRequest) (*SearchByLabelResponse, error) {
	// Build CQL query for label search
	cql := "label = \"" + req.Label + "\""
	if req.SpaceKey != "" {
		cql += " AND space = \"" + req.SpaceKey + "\""
	}
	
	results, err := uc.apiClient.Search(ctx, cql, req.Limit)
	if err != nil {
		return nil, err
	}
	
	// Convert search results to pages
	pages := make([]models.Page, 0, len(results))
	for _, result := range results {
		// Extract page info from search result
		if result.Content.ID.Value != nil {
			pages = append(pages, result.Content)
		}
	}
	
	return &SearchByLabelResponse{
		Pages: pages,
	}, nil
}
