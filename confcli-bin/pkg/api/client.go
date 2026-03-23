package api

import (
	"context"
	"confcli/pkg/models"
)

// Client defines the interface for a Confluence API client
// This is the high-level interface used by the CLI commands
type Client interface {
	// Page operations
	GetPage(ctx context.Context, id int) (*models.Page, error)
	GetPageByTitle(ctx context.Context, spaceKey, title string) (*models.Page, error)
	GetPageContent(ctx context.Context, id interface{}, format string, version int) (string, error)
	GetPageChildren(ctx context.Context, id int) ([]models.Page, error)
	GetDescendants(ctx context.Context, id int, depth int) ([]models.Page, error)
	GetPageWithExpansions(ctx context.Context, id interface{}, expansions []string) (*models.Page, error)
	CreatePage(ctx context.Context, spaceKey string, parentID *int, title string, content string, format string) (*models.Page, error)
	UpdatePage(ctx context.Context, id int, content string, versionComment string, format string) (*models.Page, error)
	DeletePage(ctx context.Context, id int) error

	// Space operations
	GetSpace(ctx context.Context, key string) (*models.Space, error)
	GetSpaceRootPages(ctx context.Context, spaceKey string) ([]models.Page, error)
	GetAllPagesInSpace(ctx context.Context, spaceKey string) ([]models.Page, error)
	GetAllPagesInSpaceIterative(ctx context.Context, spaceKey string, batchSize int, handler func(batch []models.Page) error) error

	// Search operations
	Search(ctx context.Context, cql string, limit int) ([]models.SearchResult, error)

	// Content operations (comments, labels)
	GetComments(ctx context.Context, pageID int) ([]models.Comment, error)
	GetLabels(ctx context.Context, pageID int) ([]models.Label, error)
	AddComment(ctx context.Context, pageID int, text string, parentCommentID *int) (*models.Comment, error)
	AddLabel(ctx context.Context, pageID int, labelName string) error

	// Authentication operations
	AuthenticateViaBrowser(ctx context.Context) error
	GetCurrentUserDetails(ctx context.Context) (*models.User, error)

	// Version history operations
	GetPageVersions(ctx context.Context, pageID int) ([]models.Version, error)

	// Low-level access (for advanced use cases)
	GetHTTPClient() interface{}
}

// ClientOptions holds configuration options for the API client
type ClientOptions struct {
	BaseURL        string
	AuthType       string
	Token          string
	Username       string
	Password       string
	ReadOnly       bool
	SessionCookie  string
	SAMLAuthCookie string
}
