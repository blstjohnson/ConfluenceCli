package repositories

import (
	"context"

	"confcli/pkg/models"
)

// PageRepository defines the interface for page data access operations
type PageRepository interface {
	GetPage(ctx context.Context, id int) (*models.Page, error)
	GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error)
	GetPageContent(ctx context.Context, id interface{}, format string, version int) (string, error)
	GetPageChildren(ctx context.Context, id int) ([]models.Page, error)
	GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error)
	GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error)
	CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error)
	UpdatePage(ctx context.Context, id int, content string, versionComment string, format string, parentID *int) (*models.Page, error)
	DeletePage(ctx context.Context, id int) error
	UploadAttachment(ctx context.Context, pageID int, filename string, data []byte, mimeType string) error
	GetComments(ctx context.Context, pageID int) ([]models.Comment, error)
	GetLabels(ctx context.Context, pageID int) ([]models.Label, error)
	AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error)
	AddLabel(ctx context.Context, pageID int, labelName string) error
	RemoveLabel(ctx context.Context, pageID int, labelName string) error
	GetPageVersions(ctx context.Context, pageID int) ([]models.Version, error)
}

// SpaceRepository defines the interface for space data access operations
type SpaceRepository interface {
	Get(ctx context.Context, key string) (*models.Space, error)
	GetRootPages(ctx context.Context, spaceKey string) ([]models.Page, error)
	GetAllPages(ctx context.Context, spaceKey string) ([]models.Page, error)
	GetAllPagesIterative(ctx context.Context, spaceKey string, batchSize int, handler PageBatchHandler) error
}

// PageBatchHandler is a callback function for processing page batches
type PageBatchHandler func(batch []models.Page) error

// SearchRepository defines the interface for search data access operations
type SearchRepository interface {
	Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error)
}