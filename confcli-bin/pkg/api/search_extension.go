package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"confcli/pkg/models"
)

// SearchExtension provides higher-level search operations built on top of HTTPClient
type SearchExtension struct {
	client *HTTPClient
}

// NewSearchExtension creates a new search extension
func NewSearchExtension(client *HTTPClient) *SearchExtension {
	return &SearchExtension{client: client}
}

// SearchRequest represents a request to search using CQL
type SearchRequest struct {
	CQL   string
	Limit int
	Start int
}

// SearchResponse represents the response from a search operation
type SearchResponse struct {
	Results []models.SearchResult
	Start   int
	Limit   int
	Size    int
	Total   int
	HasMore bool
}

// Search searches for pages using CQL
func (e *SearchExtension) Search(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	params := url.Values{}
	params.Add("cql", req.CQL)
	params.Add("start", fmt.Sprintf("%d", req.Start))

	limit := req.Limit
	if limit <= 0 {
		limit = 25 // Default limit
	}
	params.Add("limit", fmt.Sprintf("%d", limit))

	path := e.client.APIPrefix + "/search"
	resp, err := e.client.MakeRequest(ctx, "GET", path, params, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []models.SearchResult `json:"results"`
		Start   int                   `json:"start"`
		Limit   int                   `json:"limit"`
		Size    int                   `json:"size"`
		Total   int                   `json:"totalSize"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	hasMore := result.Size == limit && (result.Total-result.Start) > limit

	return &SearchResponse{
		Results: result.Results,
		Start:   result.Start,
		Limit:   result.Limit,
		Size:    result.Size,
		Total:   result.Total,
		HasMore: hasMore,
	}, nil
}
