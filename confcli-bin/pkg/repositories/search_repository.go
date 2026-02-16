package repositories

import (
	"context"

	"confcli/pkg/api"
	"confcli/pkg/models"
)

// HTTPSearchRepository implements the SearchRepository interface using API extensions
type HTTPSearchRepository struct {
	searchExtension *api.SearchExtension
}

// NewHTTPSearchRepository creates a new HTTP search repository
func NewHTTPSearchRepository(client *api.HTTPClient) *HTTPSearchRepository {
	return &HTTPSearchRepository{
		searchExtension: api.NewSearchExtension(client),
	}
}

// Search searches for pages using CQL
func (r *HTTPSearchRepository) Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error) {
	if limit <= 0 {
		limit = 25 // Default limit
	}

	resp, err := r.searchExtension.Search(ctx, &api.SearchRequest{
		CQL:   cql,
		Limit: limit,
		Start: 0,
	})
	if err != nil {
		return nil, err
	}

	return resp.Results, nil
}
