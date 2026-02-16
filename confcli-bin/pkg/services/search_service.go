package services

import (
	"context"
	"fmt"
	"strings"

	"confcli/pkg/models"
	"confcli/pkg/repositories"
)

// SearchServiceInterface defines the interface for search-related business operations
type SearchServiceInterface interface {
	Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error)
}

// SearchService implements the SearchServiceInterface with business logic
type SearchService struct {
	repository repositories.SearchRepository
}

// NewSearchService creates a new search service
func NewSearchService(repository repositories.SearchRepository) *SearchService {
	return &SearchService{
		repository: repository,
	}
}

// Search searches for pages using CQL with validation
func (ss *SearchService) Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error) {
	if cql == "" {
		return nil, fmt.Errorf("CQL query cannot be empty")
	}
	
	// Basic CQL validation - check for common patterns
	cql = strings.TrimSpace(cql)
	if len(cql) < 3 {
		return nil, fmt.Errorf("CQL query too short (minimum 3 characters)")
	}
	
	if limit <= 0 {
		limit = 25 // Default limit
	}
	
	if limit > 100 {
		limit = 100 // Maximum limit to prevent excessive API calls
	}
	
	return ss.repository.Search(ctx, cql, limit)
}
