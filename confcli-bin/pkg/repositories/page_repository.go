package repositories

import (
	"context"

	"confcli/pkg/api"
	"confcli/pkg/models"
)

// HTTPPageRepository implements the PageRepository interface using API extensions
type HTTPPageRepository struct {
	pageExtension *api.PageExtension
}

// NewHTTPPageRepository creates a new HTTP page repository
func NewHTTPPageRepository(client *api.HTTPClient) *HTTPPageRepository {
	return &HTTPPageRepository{
		pageExtension: api.NewPageExtension(client),
	}
}

// GetPage retrieves a page by its ID
func (r *HTTPPageRepository) GetPage(ctx context.Context, id int) (*models.Page, error) {
	resp, err := r.pageExtension.FetchPage(ctx, &api.FetchPageRequest{
		PageID: id,
	})
	if err != nil {
		return nil, err
	}
	return resp.Page, nil
}

// GetPageByTitle retrieves a page by its space key and title
func (r *HTTPPageRepository) GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error) {
	resp, err := r.pageExtension.FindPageByTitle(ctx, &api.FindPageByTitleRequest{
		SpaceKey: spaceKey,
		Title:    title,
	})
	if err != nil {
		return nil, err
	}
	return resp.Page, nil
}

// GetPageContent retrieves the content of a page in the specified format
func (r *HTTPPageRepository) GetPageContent(ctx context.Context, id interface{}, format string) (string, error) {
	resp, err := r.pageExtension.FetchPageContent(ctx, &api.FetchPageContentRequest{
		PageID: id,
		Format: format,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// GetPageChildren retrieves the children of a page
func (r *HTTPPageRepository) GetPageChildren(ctx context.Context, id int) ([]models.Page, error) {
	resp, err := r.pageExtension.FetchPageChildren(ctx, &api.FetchPageChildrenRequest{
		PageID: id,
	})
	if err != nil {
		return nil, err
	}
	return resp.Children, nil
}

// GetPageWithExpansions retrieves a page with specified expansions
func (r *HTTPPageRepository) GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error) {
	// Use content extension for this operation as it handles expansions
	contentExt := api.NewContentExtension(r.pageExtension.GetHTTPClient())
	resp, err := contentExt.FetchPageWithExpansions(ctx, &api.FetchPageWithExpansionsRequest{
		PageID:     id,
		Expansions: expansions,
	})
	if err != nil {
		return nil, err
	}
	return resp.Page, nil
}

// GetDescendants retrieves all descendants of a page up to a certain depth
func (r *HTTPPageRepository) GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error) {
	pages := make([]models.Page, 0)

	// Validate the page exists
	_, err := r.GetPage(ctx, id)
	if err != nil {
		return nil, err
	}

	// Get children of the initial page
	children, err := r.GetPageChildren(ctx, id)
	if err != nil {
		return nil, err
	}

	pages = append(pages, children...)

	// If depth > 1 or unlimited (0), recursively get children of children
	if depth > 1 || depth == 0 {
		for _, child := range children {
			childID, ok := child.ID.Int()
			if !ok {
				continue
			}
			childDescendants, err := r.getDescendantsRecursive(ctx, childID, depth, 2)
			if err != nil {
				// Log the error but continue with other pages
				continue
			}
			pages = append(pages, childDescendants...)
		}
	}

	return pages, nil
}

// getDescendantsRecursive is a helper function to recursively get descendants
func (r *HTTPPageRepository) getDescendantsRecursive(ctx context.Context, id int, maxDepth, currentDepth int) ([]models.Page, error) {
	if maxDepth > 0 && currentDepth > maxDepth {
		return []models.Page{}, nil
	}

	children, err := r.GetPageChildren(ctx, id)
	if err != nil {
		return nil, err
	}

	result := children

	// Recursively get children of children
	for _, child := range children {
		childID, ok := child.ID.Int()
		if !ok {
			continue
		}
		childDescendants, err := r.getDescendantsRecursive(ctx, childID, maxDepth, currentDepth+1)
		if err != nil {
			// Log the error but continue with other pages
			continue
		}
		result = append(result, childDescendants...)
	}

	return result, nil
}

// CreatePage creates a new page
func (r *HTTPPageRepository) CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error) {
	resp, err := r.pageExtension.CreatePage(ctx, &api.CreatePageRequest{
		SpaceKey: spaceKey,
		ParentID: parentID,
		Title:    title,
		Content:  content,
		Format:   format,
	})
	if err != nil {
		return nil, err
	}
	return resp.Page, nil
}

// UpdatePage updates an existing page
func (r *HTTPPageRepository) UpdatePage(ctx context.Context, id int, content string, versionComment string, format string) (*models.Page, error) {
	resp, err := r.pageExtension.UpdatePage(ctx, &api.UpdatePageRequest{
		PageID:         id,
		Content:        content,
		VersionComment: versionComment,
		Format:         format,
	})
	if err != nil {
		return nil, err
	}
	return resp.Page, nil
}

// DeletePage deletes a page
func (r *HTTPPageRepository) DeletePage(ctx context.Context, id int) error {
	return r.pageExtension.DeletePage(ctx, &api.DeletePageRequest{
		PageID: id,
	})
}

// GetComments retrieves comments for a page
func (r *HTTPPageRepository) GetComments(ctx context.Context, pageID int) ([]models.Comment, error) {
	contentExt := api.NewContentExtension(r.pageExtension.GetHTTPClient())
	resp, err := contentExt.FetchComments(ctx, &api.FetchCommentsRequest{
		PageID: pageID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Comments, nil
}

// GetLabels retrieves labels for a page
func (r *HTTPPageRepository) GetLabels(ctx context.Context, pageID int) ([]models.Label, error) {
	contentExt := api.NewContentExtension(r.pageExtension.GetHTTPClient())
	resp, err := contentExt.FetchLabels(ctx, &api.FetchLabelsRequest{
		PageID: pageID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Labels, nil
}

// AddComment adds a comment to a page
func (r *HTTPPageRepository) AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error) {
	contentExt := api.NewContentExtension(r.pageExtension.GetHTTPClient())
	resp, err := contentExt.AddComment(ctx, &api.AddCommentRequest{
		PageID:          pageID,
		Text:            text,
		ParentCommentID: parentCommentID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Comment, nil
}

// AddLabel adds a label to a page
func (r *HTTPPageRepository) AddLabel(ctx context.Context, pageID int, labelName string) error {
	contentExt := api.NewContentExtension(r.pageExtension.GetHTTPClient())
	return contentExt.AddLabel(ctx, &api.AddLabelRequest{
		PageID:    pageID,
		LabelName: labelName,
	})
}
