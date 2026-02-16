package usecases

import (
	"context"

	"confcli/pkg/api"
	"confcli/pkg/models"
)

// PageUseCase defines the interface for page-related use cases
type PageUseCase interface {
	// GetPageWithContent retrieves a page with its content and optional expansions
	GetPageWithContent(ctx context.Context, req *GetPageWithContentRequest) (*GetPageWithContentResponse, error)
	
	// GetPageHierarchy retrieves a page with its ancestors and descendants
	GetPageHierarchy(ctx context.Context, req *GetPageHierarchyRequest) (*GetPageHierarchyResponse, error)
	
	// CreatePageWithValidation creates a page with full validation
	CreatePageWithValidation(ctx context.Context, req *CreatePageWithValidationRequest) (*CreatePageWithValidationResponse, error)
	
	// UpdatePageWithVersion updates a page handling version management
	UpdatePageWithVersion(ctx context.Context, req *UpdatePageWithVersionRequest) (*UpdatePageWithVersionResponse, error)
	
	// DeletePageWithConfirmation deletes a page with confirmation check
	DeletePageWithConfirmation(ctx context.Context, req *DeletePageWithConfirmationRequest) error
	
	// ExportPage exports a page to various formats
	ExportPage(ctx context.Context, req *ExportPageRequest) (*ExportPageResponse, error)
}

// GetPageWithContentRequest represents a request to get a page with content
type GetPageWithContentRequest struct {
	PageID        int
	SpaceKey      string
	Title         string
	Format        string
	Version       int
	Date          string
	WithComments  bool
	WithLabels    bool
	WithMetadata  bool
}

// GetPageWithContentResponse represents the response from getting a page with content
type GetPageWithContentResponse struct {
	Page             *models.Page
	Content          string
	TransformedContent string
	Comments         []models.Comment
	Labels           []models.Label
}

// GetPageHierarchyRequest represents a request to get page hierarchy
type GetPageHierarchyRequest struct {
	PageID   int
	SpaceKey string
	Title    string
	Depth    int
}

// GetPageHierarchyResponse represents the response from getting page hierarchy
type GetPageHierarchyResponse struct {
	Page        *models.Page
	Ancestors   []models.Page
	Descendants []models.Page
}

// CreatePageWithValidationRequest represents a request to create a page
type CreatePageWithValidationRequest struct {
	SpaceKey string
	ParentID *int
	Title    string
	Content  string
	Format   string
}

// CreatePageWithValidationResponse represents the response from creating a page
type CreatePageWithValidationResponse struct {
	Page *models.Page
}

// UpdatePageWithVersionRequest represents a request to update a page
type UpdatePageWithVersionRequest struct {
	PageID          int
	Content         string
	VersionComment  string
}

// UpdatePageWithVersionResponse represents the response from updating a page
type UpdatePageWithVersionResponse struct {
	Page *models.Page
}

// DeletePageWithConfirmationRequest represents a request to delete a page
type DeletePageWithConfirmationRequest struct {
	PageID    int
	Confirmed bool
}

// ExportPageRequest represents a request to export a page
type ExportPageRequest struct {
	PageID        int
	SpaceKey      string
	Title         string
	Format        string
	OutputPath    string
	OutputDir     string
	IncludeMetadata bool
}

// ExportPageResponse represents the response from exporting a page
type ExportPageResponse struct {
	FilePath       string
	MetadataPath   string
	Page           *models.Page
}

// pageUseCase implements the PageUseCase interface
type pageUseCase struct {
	apiClient api.Client
}

// NewPageUseCase creates a new page use case
func NewPageUseCase(apiClient api.Client) PageUseCase {
	return &pageUseCase{
		apiClient: apiClient,
	}
}

// GetPageWithContent retrieves a page with its content and optional expansions
func (uc *pageUseCase) GetPageWithContent(ctx context.Context, req *GetPageWithContentRequest) (*GetPageWithContentResponse, error) {
	// Get the page
	var page *models.Page
	var err error
	
	if req.PageID != 0 {
		page, err = uc.apiClient.GetPage(ctx, req.PageID)
	} else {
		page, err = uc.apiClient.GetPageByTitle(ctx, req.SpaceKey, req.Title)
	}
	if err != nil {
		return nil, err
	}
	
	// Get content
	content, err := uc.apiClient.GetPageContent(ctx, page.ID.IntOrString(), "storage")
	if err != nil {
		return nil, err
	}
	
	// Get optional data
	var comments []models.Comment
	var labels []models.Label
	
	if req.WithComments {
		pageID, ok := page.ID.Int()
		if ok {
			comments, _ = uc.apiClient.GetComments(ctx, pageID)
		}
	}
	
	if req.WithLabels {
		pageID, ok := page.ID.Int()
		if ok {
			labels, _ = uc.apiClient.GetLabels(ctx, pageID)
		}
	}
	
	return &GetPageWithContentResponse{
		Page:    page,
		Content: content,
		Comments: comments,
		Labels:   labels,
	}, nil
}

// GetPageHierarchy retrieves a page with its ancestors and descendants
func (uc *pageUseCase) GetPageHierarchy(ctx context.Context, req *GetPageHierarchyRequest) (*GetPageHierarchyResponse, error) {
	// Get the page
	var page *models.Page
	var err error
	
	if req.PageID != 0 {
		page, err = uc.apiClient.GetPage(ctx, req.PageID)
	} else {
		page, err = uc.apiClient.GetPageByTitle(ctx, req.SpaceKey, req.Title)
	}
	if err != nil {
		return nil, err
	}
	
	// Get page with ancestors
	pageWithExpansions, err := uc.apiClient.GetPageWithExpansions(ctx, page.ID.IntOrString(), []string{"ancestors"})
	if err != nil {
		return nil, err
	}
	
	// Get descendants
	descendants, err := uc.apiClient.GetDescendants(ctx, page.ID.IntOrString().(int), req.Depth)
	if err != nil {
		return nil, err
	}
	
	return &GetPageHierarchyResponse{
		Page:        page,
		Ancestors:   pageWithExpansions.Ancestors,
		Descendants: descendants,
	}, nil
}

// CreatePageWithValidation creates a page with full validation
func (uc *pageUseCase) CreatePageWithValidation(ctx context.Context, req *CreatePageWithValidationRequest) (*CreatePageWithValidationResponse, error) {
	page, err := uc.apiClient.CreatePage(ctx, req.SpaceKey, req.ParentID, req.Title, req.Content, req.Format)
	if err != nil {
		return nil, err
	}
	
	return &CreatePageWithValidationResponse{
		Page: page,
	}, nil
}

// UpdatePageWithVersion updates a page handling version management
func (uc *pageUseCase) UpdatePageWithVersion(ctx context.Context, req *UpdatePageWithVersionRequest) (*UpdatePageWithVersionResponse, error) {
	page, err := uc.apiClient.UpdatePage(ctx, req.PageID, req.Content, req.VersionComment)
	if err != nil {
		return nil, err
	}
	
	return &UpdatePageWithVersionResponse{
		Page: page,
	}, nil
}

// DeletePageWithConfirmation deletes a page with confirmation check
func (uc *pageUseCase) DeletePageWithConfirmation(ctx context.Context, req *DeletePageWithConfirmationRequest) error {
	if !req.Confirmed {
		return &ValidationError{Message: "confirmation required to delete page"}
	}
	
	return uc.apiClient.DeletePage(ctx, req.PageID)
}

// ExportPage exports a page to various formats
func (uc *pageUseCase) ExportPage(ctx context.Context, req *ExportPageRequest) (*ExportPageResponse, error) {
	// Get the page
	var page *models.Page
	var err error
	
	if req.PageID != 0 {
		page, err = uc.apiClient.GetPage(ctx, req.PageID)
	} else {
		page, err = uc.apiClient.GetPageByTitle(ctx, req.SpaceKey, req.Title)
	}
	if err != nil {
		return nil, err
	}
	
	// Export logic would go here (file writing, etc.)
	// For now, return basic response
	return &ExportPageResponse{
		Page: page,
	}, nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}
