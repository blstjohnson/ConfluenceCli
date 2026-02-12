package api

import (
	"context"
	"confcli/pkg/models"
)

// Client defines the interface for a Confluence API client
type Client interface {
	GetPage(ctx context.Context, id int) (*models.Page, error)
	GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error)
	GetPageContent(ctx context.Context, id interface{}, format string) (string, error)
	GetPageChildren(ctx context.Context, id int) ([]models.Page, error)
	GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error)
	GetSpaceRootPages(ctx context.Context, spaceKey string) ([]models.Page, error)
	GetSpace(ctx context.Context, key string) (*models.Space, error)
	GetAllPagesInSpace(ctx context.Context, spaceKey string) ([]models.Page, error)
	Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error)
	CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error)
	UpdatePage(ctx context.Context, id int, content string, versionComment string) (*models.Page, error)
	DeletePage(ctx context.Context, id int) error
	AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error)
	AddLabel(ctx context.Context, pageID int, labelName string) error
	GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error)
	AuthenticateViaBrowser(ctx context.Context) error
}