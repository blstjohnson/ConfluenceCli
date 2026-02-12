package confluence

import (
	"context"
	"confcli/pkg/models"
)

// SearchService handles operations related to searching in Confluence
type SearchService struct {
	client *client
}

// NewSearchService creates a new search service
func NewSearchService(client *client) *SearchService {
	return &SearchService{
		client: client,
	}
}

// Search performs a search using CQL (Confluence Query Language)
func (ss *SearchService) Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error) {
	return ss.client.Search(ctx, cql, limit)
}