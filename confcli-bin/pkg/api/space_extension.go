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

// SpaceExtension provides higher-level space operations built on top of HTTPClient
type SpaceExtension struct {
	client *HTTPClient
}

// NewSpaceExtension creates a new space extension
func NewSpaceExtension(client *HTTPClient) *SpaceExtension {
	return &SpaceExtension{client: client}
}

// FetchSpaceRequest represents a request to fetch a space
type FetchSpaceRequest struct {
	SpaceKey string
}

// FetchSpaceResponse represents the response from fetching a space
type FetchSpaceResponse struct {
	Space *models.Space
}

// FetchSpace fetches a space by its key
func (e *SpaceExtension) FetchSpace(ctx context.Context, req *FetchSpaceRequest) (*FetchSpaceResponse, error) {
	path := fmt.Sprintf("%s/space/%s", e.client.APIPrefix, req.SpaceKey)
	resp, err := e.client.MakeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var space models.Space
	if err := json.NewDecoder(resp.Body).Decode(&space); err != nil {
		return nil, err
	}

	return &FetchSpaceResponse{Space: &space}, nil
}

// FetchAllPagesInSpaceRequest represents a request to fetch all pages in a space
type FetchAllPagesInSpaceRequest struct {
	SpaceKey string
	Start    int
	Limit    int
}

// FetchAllPagesInSpaceResponse represents the response from fetching all pages in a space
type FetchAllPagesInSpaceResponse struct {
	Pages  []models.Page
	Start  int
	Limit  int
	Size   int
	HasMore bool
}

// FetchAllPagesInSpace fetches all pages in a space with pagination
func (e *SpaceExtension) FetchAllPagesInSpace(ctx context.Context, req *FetchAllPagesInSpaceRequest) (*FetchAllPagesInSpaceResponse, error) {
	params := url.Values{}
	params.Add("space", req.SpaceKey)
	params.Add("start", fmt.Sprintf("%d", req.Start))
	params.Add("limit", fmt.Sprintf("%d", req.Limit))

	path := e.client.APIPrefix + "/content?" + params.Encode()
	resp, err := e.client.MakeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []models.Page `json:"results"`
		Start   int           `json:"start"`
		Limit   int           `json:"limit"`
		Size    int           `json:"size"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	hasMore := result.Size == req.Limit

	return &FetchAllPagesInSpaceResponse{
		Pages:   result.Results,
		Start:   result.Start,
		Limit:   result.Limit,
		Size:    result.Size,
		HasMore: hasMore,
	}, nil
}

// FetchRootPagesInSpaceRequest represents a request to fetch root pages in a space
type FetchRootPagesInSpaceRequest struct {
	SpaceKey string
}

// FetchRootPagesInSpaceResponse represents the response from fetching root pages
type FetchRootPagesInSpaceResponse struct {
	Pages []models.Page
}

// FetchRootPagesInSpace fetches root-level pages in a space
func (e *SpaceExtension) FetchRootPagesInSpace(ctx context.Context, req *FetchRootPagesInSpaceRequest) (*FetchRootPagesInSpaceResponse, error) {
	// For now, return all pages in the space
	// A more sophisticated implementation would filter to only root-level pages
	allPagesResp, err := e.FetchAllPagesInSpace(ctx, &FetchAllPagesInSpaceRequest{
		SpaceKey: req.SpaceKey,
		Start:    0,
		Limit:    100,
	})
	if err != nil {
		return nil, err
	}

	// Filter to only include pages that are direct children of the space homepage
	// or have no ancestors (truly root-level)
	rootPages := make([]models.Page, 0)
	for _, page := range allPagesResp.Pages {
		if len(page.Ancestors) == 0 {
			rootPages = append(rootPages, page)
		}
	}

	return &FetchRootPagesInSpaceResponse{Pages: rootPages}, nil
}
