package api

import (
	"context"
	"confcli/pkg/models"
)

// API constants
const (
	// APIVersion refers to the Confluence REST API version
	APIVersion = "v2" // Using v2 for Cloud, fallback to v1 for Server
)

// ClientOptions holds configuration options for the API client
type ClientOptions struct {
	BaseURL        string
	AuthType       string
	Token          string
	Username       string
	Password       string
	ImpersonateAs  string
	UseDomainAuth  bool
	ReadOnly       bool
	SessionCookie  string
	SAMLAuthCookie string
}

// PageOperations defines operations related to pages
type PageOperations interface {
	GetPage(ctx context.Context, id int) (*models.Page, error)
	GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error)
	GetPageContent(ctx context.Context, id interface{}, format string) (string, error)
	GetPageChildren(ctx context.Context, id int) ([]models.Page, error)
	GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error)
	GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error)
	CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error)
	UpdatePage(ctx context.Context, id int, content string, versionComment string) (*models.Page, error)
	DeletePage(ctx context.Context, id int) error
}

// SpaceOperations defines operations related to spaces
type SpaceOperations interface {
	GetSpace(ctx context.Context, key string) (*models.Space, error)
	GetSpaceRootPages(ctx context.Context, spaceKey string) ([]models.Page, error)
	GetAllPagesInSpace(ctx context.Context, spaceKey string) ([]models.Page, error)
}

// ContentOperations defines operations related to content (comments, labels, etc.)
type ContentOperations interface {
	AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error)
	AddLabel(ctx context.Context, pageID int, labelName string) error
}

// SearchOperations defines operations related to searching
type SearchOperations interface {
	Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error)
}