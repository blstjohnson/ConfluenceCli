package services

import (
	"context"
	"fmt"

	"confcli/pkg/models"
	"confcli/pkg/repositories"
)

// PageServiceInterface defines the interface for page-related business operations
type PageServiceInterface interface {
	GetPage(ctx context.Context, id int) (*models.Page, error)
	GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error)
	GetPageContent(ctx context.Context, id interface{}, format string, version int) (string, error)
	GetPageChildren(ctx context.Context, id int) ([]models.Page, error)
	GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error)
	GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error)
	CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error)
	UpdatePage(ctx context.Context, id int, content string, versionComment string, format string) (*models.Page, error)
	DeletePage(ctx context.Context, id int) error
	GetComments(ctx context.Context, pageID int) ([]models.Comment, error)
	GetLabels(ctx context.Context, pageID int) ([]models.Label, error)
	AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error)
	AddLabel(ctx context.Context, pageID int, labelName string) error
	RemoveLabel(ctx context.Context, pageID int, labelName string) error
	GetPageVersions(ctx context.Context, pageID int) ([]models.Version, error)
}

// PageService implements the PageServiceInterface with business logic
type PageService struct {
	repository repositories.PageRepository
}

// NewPageService creates a new page service
func NewPageService(repository repositories.PageRepository) *PageService {
	return &PageService{
		repository: repository,
	}
}

// GetPage retrieves a page by its ID with validation
func (ps *PageService) GetPage(ctx context.Context, id int) (*models.Page, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid page ID: %d", id)
	}
	return ps.repository.GetPage(ctx, id)
}

// GetPageByTitle retrieves a page by its space key and title with validation
func (ps *PageService) GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error) {
	if spaceKey == "" {
		return nil, fmt.Errorf("space key cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}
	return ps.repository.GetPageByTitle(ctx, spaceKey, title)
}

// GetPageContent retrieves the content of a page in the specified format with validation
func (ps *PageService) GetPageContent(ctx context.Context, id interface{}, format string, version int) (string, error) {
	if format == "" {
		return "", fmt.Errorf("format cannot be empty")
	}
	return ps.repository.GetPageContent(ctx, id, format, version)
}

// GetPageChildren retrieves the children of a page with validation
func (ps *PageService) GetPageChildren(ctx context.Context, id int) ([]models.Page, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid page ID: %d", id)
	}
	return ps.repository.GetPageChildren(ctx, id)
}

// GetPageWithExpansions retrieves a page with specified expansions
func (ps *PageService) GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error) {
	return ps.repository.GetPageWithExpansions(ctx, id, expansions)
}

// GetDescendants retrieves all descendants of a page up to a certain depth with validation
func (ps *PageService) GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid page ID: %d", id)
	}
	if depth < 0 {
		return nil, fmt.Errorf("invalid depth: %d (must be >= 0, where 0 = unlimited)", depth)
	}
	return ps.repository.GetDescendants(ctx, id, depth)
}

// CreatePage creates a new page with validation
func (ps *PageService) CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error) {
	if spaceKey == "" {
		return nil, fmt.Errorf("space key cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}
	if content == "" {
		return nil, fmt.Errorf("content cannot be empty")
	}
	if format == "" {
		format = "storage" // Default format
	}
	if parentID != nil && *parentID <= 0 {
		return nil, fmt.Errorf("invalid parent ID: %d", *parentID)
	}
	return ps.repository.CreatePage(ctx, spaceKey, parentID, title, content, format)
}

// UpdatePage updates an existing page with validation
func (ps *PageService) UpdatePage(ctx context.Context, id int, content string, versionComment string, format string) (*models.Page, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid page ID: %d", id)
	}
	if content == "" {
		return nil, fmt.Errorf("content cannot be empty")
	}
	if format == "" {
		format = "storage" // Default format
	}
	return ps.repository.UpdatePage(ctx, id, content, versionComment, format)
}

// DeletePage deletes a page with validation
func (ps *PageService) DeletePage(ctx context.Context, id int) error {
	if id <= 0 {
		return fmt.Errorf("invalid page ID: %d", id)
	}
	return ps.repository.DeletePage(ctx, id)
}

// GetComments retrieves comments for a page with validation
func (ps *PageService) GetComments(ctx context.Context, pageID int) ([]models.Comment, error) {
	if pageID <= 0 {
		return nil, fmt.Errorf("invalid page ID: %d", pageID)
	}
	return ps.repository.GetComments(ctx, pageID)
}

// GetLabels retrieves labels for a page with validation
func (ps *PageService) GetLabels(ctx context.Context, pageID int) ([]models.Label, error) {
	if pageID <= 0 {
		return nil, fmt.Errorf("invalid page ID: %d", pageID)
	}
	return ps.repository.GetLabels(ctx, pageID)
}

// AddComment adds a comment to a page with validation
func (ps *PageService) AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error) {
	if pageID <= 0 {
		return nil, fmt.Errorf("invalid page ID: %d", pageID)
	}
	if text == "" {
		return nil, fmt.Errorf("comment text cannot be empty")
	}
	if parentCommentID != nil && *parentCommentID <= 0 {
		return nil, fmt.Errorf("invalid parent comment ID: %d", *parentCommentID)
	}
	return ps.repository.AddComment(ctx, pageID, text, parentCommentID)
}

// AddLabel adds a label to a page with validation
func (ps *PageService) AddLabel(ctx context.Context, pageID int, labelName string) error {
	if pageID <= 0 {
		return fmt.Errorf("invalid page ID: %d", pageID)
	}
	if labelName == "" {
		return fmt.Errorf("label name cannot be empty")
	}
	return ps.repository.AddLabel(ctx, pageID, labelName)
}

// RemoveLabel removes a label from a page with validation
func (ps *PageService) RemoveLabel(ctx context.Context, pageID int, labelName string) error {
	if pageID <= 0 {
		return fmt.Errorf("invalid page ID: %d", pageID)
	}
	if labelName == "" {
		return fmt.Errorf("label name cannot be empty")
	}
	return ps.repository.RemoveLabel(ctx, pageID, labelName)
}

// GetPageVersions retrieves the version history for a page with validation
func (ps *PageService) GetPageVersions(ctx context.Context, pageID int) ([]models.Version, error) {
	if pageID <= 0 {
		return nil, fmt.Errorf("invalid page ID: %d", pageID)
	}
	return ps.repository.GetPageVersions(ctx, pageID)
}
