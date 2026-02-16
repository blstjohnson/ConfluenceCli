package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"confcli/pkg/models"
)

// ContentExtension provides higher-level content operations (comments, labels) built on top of HTTPClient
type ContentExtension struct {
	client *HTTPClient
}

// NewContentExtension creates a new content extension
func NewContentExtension(client *HTTPClient) *ContentExtension {
	return &ContentExtension{client: client}
}

// FetchCommentsRequest represents a request to fetch comments
type FetchCommentsRequest struct {
	PageID int
}

// FetchCommentsResponse represents the response from fetching comments
type FetchCommentsResponse struct {
	Comments []models.Comment
}

// FetchComments fetches comments for a page
func (e *ContentExtension) FetchComments(ctx context.Context, req *FetchCommentsRequest) (*FetchCommentsResponse, error) {
	path := fmt.Sprintf("%s/content/%d/comment", e.client.APIPrefix, req.PageID)
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
		Results []models.Comment `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &FetchCommentsResponse{Comments: result.Results}, nil
}

// FetchLabelsRequest represents a request to fetch labels
type FetchLabelsRequest struct {
	PageID int
}

// FetchLabelsResponse represents the response from fetching labels
type FetchLabelsResponse struct {
	Labels []models.Label
}

// FetchLabels fetches labels for a page
func (e *ContentExtension) FetchLabels(ctx context.Context, req *FetchLabelsRequest) (*FetchLabelsResponse, error) {
	path := fmt.Sprintf("%s/content/%d/label", e.client.APIPrefix, req.PageID)
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
		Results []models.Label `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &FetchLabelsResponse{Labels: result.Results}, nil
}

// AddCommentRequest represents a request to add a comment
type AddCommentRequest struct {
	PageID          int
	Text            string
	ParentCommentID *int
}

// AddCommentResponse represents the response from adding a comment
type AddCommentResponse struct {
	Comment *models.Comment
}

// AddComment adds a comment to a page
func (e *ContentExtension) AddComment(ctx context.Context, req *AddCommentRequest) (*AddCommentResponse, error) {
	if e.client.ReadOnly {
		return nil, fmt.Errorf("read-only mode enabled: cannot add comment")
	}

	commentData := map[string]interface{}{
		"type": "comment",
		"body": map[string]interface{}{
			"storage": map[string]interface{}{
				"value":          req.Text,
				"representation": "storage",
			},
		},
	}

	if req.ParentCommentID != nil {
		commentData["container"] = map[string]interface{}{
			"id":   fmt.Sprintf("%d", *req.ParentCommentID),
			"type": "comment",
		}
	} else {
		commentData["container"] = map[string]interface{}{
			"id":   fmt.Sprintf("%d", req.PageID),
			"type": "page",
		}
	}

	jsonData, err := json.Marshal(commentData)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.MakeRequest(ctx, "POST", e.client.APIPrefix+"/content", strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var newComment models.Comment
	if err := json.NewDecoder(resp.Body).Decode(&newComment); err != nil {
		return nil, err
	}

	return &AddCommentResponse{Comment: &newComment}, nil
}

// AddLabelRequest represents a request to add a label
type AddLabelRequest struct {
	PageID   int
	LabelName string
}

// AddLabel adds a label to a page
func (e *ContentExtension) AddLabel(ctx context.Context, req *AddLabelRequest) error {
	if e.client.ReadOnly {
		return fmt.Errorf("read-only mode enabled: cannot add label")
	}

	labelData := map[string]interface{}{
		"prefix": "global",
		"name":   req.LabelName,
	}

	jsonData, err := json.Marshal(labelData)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("%s/content/%d/label", e.client.APIPrefix, req.PageID)
	resp, err := e.client.MakeRequest(ctx, "POST", path, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// FetchPageWithExpansionsRequest represents a request to fetch a page with expansions
type FetchPageWithExpansionsRequest struct {
	PageID     interface{}
	Expansions []string
}

// FetchPageWithExpansionsResponse represents the response from fetching a page with expansions
type FetchPageWithExpansionsResponse struct {
	Page *models.Page
}

// FetchPageWithExpansions fetches a page with specified expansions
func (e *ContentExtension) FetchPageWithExpansions(ctx context.Context, req *FetchPageWithExpansionsRequest) (*FetchPageWithExpansionsResponse, error) {
	idStr := pageIDToString(req.PageID)
	expandParam := strings.Join(req.Expansions, ",")
	params := url.Values{}
	params.Add("expand", expandParam)

	path := fmt.Sprintf("%s/content/%s?%s", e.client.APIPrefix, idStr, params.Encode())
	resp, err := e.client.MakeRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var page models.Page
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}

	return &FetchPageWithExpansionsResponse{Page: &page}, nil
}
