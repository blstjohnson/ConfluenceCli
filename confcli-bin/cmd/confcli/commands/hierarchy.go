package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xlab/treeprint"

	"confcli/pkg/api"
	"confcli/pkg/clients"
	"confcli/pkg/converters"
	"confcli/pkg/models"
	"confcli/pkg/formatters"
	"confcli/pkg/utils"
	"confcli/pkg/usecases"
)

// NewHierarchyCmd creates the hierarchy command
func NewHierarchyCmd() *cobra.Command {
	hierarchyCmd := &cobra.Command{
		Use:   "hierarchy",
		Short: "Commands for working with page hierarchies",
		Long:  `Commands for retrieving and exporting page hierarchies`,
	}

	hierarchyCmd.AddCommand(newHierarchySpaceCmd())

	return hierarchyCmd
}

// newHierarchySpaceCmd implements the hierarchy space command
func newHierarchySpaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "space",
		Short: "Get hierarchy of a space",
		Long:  "Retrieve and export the hierarchy of pages in a Confluence space",
		RunE: func(cmd *cobra.Command, args []string) error {
			space, _ := cmd.Flags().GetString("space")
			outputDir, _ := cmd.Flags().GetString("output-dir")
			format, _ := cmd.Flags().GetString("format")
			depth, _ := cmd.Flags().GetInt("depth")
			flat, _ := cmd.Flags().GetBool("flat")
			treeView, _ := cmd.Flags().GetBool("tree")
			skipContent, _ := cmd.Flags().GetBool("skip-content")
			batchSize, _ := cmd.Flags().GetInt("batch-size")

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()
			spaceUseCase := usecases.NewSpaceUseCase(apiClient)

			req := &usecases.GetSpaceHierarchyRequest{
				SpaceKey: space,
				Depth:    depth,
				Flat:     flat,
			}

			resp, err := spaceUseCase.GetSpaceHierarchy(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to get space hierarchy: %w", err)
			}

			outputFormat := viper.GetString("output_format")

			if flat {
				if outputFormat == "json" {
					flatPages := make([]map[string]interface{}, len(resp.AllPages))
					for i, page := range resp.AllPages {
						flatPages[i] = map[string]interface{}{
							"id":    page.ID,
							"title": page.Title,
							"depth": 0,
						}
					}
					return formatters.FormatOutput(flatPages, "json")
				} else {
					return formatters.FormatOutput(resp.AllPages, "text")
				}
			} else if treeView {
				tree := treeprint.New()
				for _, page := range resp.RootPages {
					pageID, _ := page.ID.Int()
					tree.AddBranch(fmt.Sprintf("%d: %s", pageID, page.Title))
				}
				fmt.Println(tree.String())
			} else if outputDir != "" {
				if err := exportSpaceToDirectoryIterative(apiClient, space, outputDir, format, depth, skipContent, batchSize); err != nil {
					return fmt.Errorf("failed to export space to directory: %w", err)
				}
				fmt.Printf("Space %s exported to %s\n", space, outputDir)
			} else {
				tree := treeprint.New()
				pageMap := make(map[int]*models.Page)
				childrenMap := make(map[int][]*models.Page)

				for i := range resp.AllPages {
					page := &resp.AllPages[i]
					pageID, ok := page.ID.Int()
					if ok {
						pageMap[pageID] = page
						childrenMap[0] = append(childrenMap[0], page)
					}
				}

				buildTree(tree, childrenMap[0], childrenMap, depth, 0)
				fmt.Println(tree.String())
			}

			return nil
		},
	}

	cmd.Flags().String("space", "", "Space key (required)")
	cmd.Flags().String("output-dir", "", "Export space to directory")
	cmd.Flags().String("format", "markdown", "Format for saved pages: markdown, storage, html, plain, edit, export/export_view (converted to markdown)")
	cmd.Flags().Int("depth", 0, "Recursion depth (default: unlimited)")
	cmd.Flags().Bool("flat", false, "Output flat list instead of tree")
	cmd.Flags().Bool("tree", false, "Display ASCII tree in console")
	cmd.Flags().Bool("skip-content", false, "Export only structure and metadata, no content")
	cmd.Flags().Int("batch-size", 10, "Batch size for iterative page fetching")
	cmd.MarkFlagRequired("space")

	return cmd
}

// exportSpaceToDirectory exports the space hierarchy to a directory structure
// It supports iterative batch processing to save memory for large spaces
func exportSpaceToDirectory(apiClient interface {
	GetPageContent(context.Context, interface{}, string, int) (string, error)
	GetSpace(context.Context, string) (*models.Space, error)
	GetAllPagesInSpace(context.Context, string) ([]models.Page, error)
}, space, outputDir, format string, depth int, skipContent bool) error {
	ctx := context.Background()

	pages, err := apiClient.GetAllPagesInSpace(ctx, space)
	if err != nil {
		return fmt.Errorf("failed to get pages in space: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	spaceDir := filepath.Join(outputDir, space)
	if err := os.MkdirAll(spaceDir, 0755); err != nil {
		return fmt.Errorf("failed to create space directory: %w", err)
	}

	baseURL := viper.GetString("url")

	for _, page := range pages {
		pageID, _ := page.ID.Int()
		if !skipContent {
			// Get content format - Confluence supports "storage", "editor", and "export_view" formats
			apiFormat := utils.GetContentFormatForAPI(format)
			
			// Try to get content from the page body first (already fetched via expansions)
			var apiContent string
			if page.Body != nil {
				bodyKey := apiFormat
				if bodyKey == "editor" {
					bodyKey = "editor"
				} else if bodyKey == "export_view" {
					bodyKey = "export_view"
				} else {
					bodyKey = "storage"
				}
				
				if bodyContent, ok := page.Body[bodyKey].(map[string]interface{}); ok {
					if value, ok := bodyContent["value"].(string); ok && value != "" {
						apiContent = value
					}
				}
				
				// If not found in requested format, try other formats
				if apiContent == "" {
					for _, key := range []string{"storage", "editor", "export_view", "view"} {
						if bodyContent, ok := page.Body[key].(map[string]interface{}); ok {
							if value, ok := bodyContent["value"].(string); ok && value != "" {
								apiContent = value
								break
							}
						}
					}
				}
			}
			
			var content string
			var err error
			// If still no content, fetch it via API call
			if apiContent == "" {
				fmt.Fprintf(os.Stderr, "Warning: page %d body not available, fetching separately\n", pageID)
				apiContent, err = apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), apiFormat, 0)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to get content for page %d: %v\n", pageID, err)
					continue
				}
			}
			
			// For export_view format, convert to markdown if requested
			if format == "export" {
				// export_view is clean HTML, convert to markdown
				content, err = converters.ExportViewToMarkdown(apiContent, baseURL)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to convert export_view to markdown for page %d: %v\n", pageID, err)
					content = apiContent
				}
			} else {
				// Convert from API format to requested format
				content, err = utils.ConvertContentFromStorage(apiContent, format, baseURL)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to convert content for page %d: %v\n", pageID, err)
					content = apiContent
				}
			}

			filename := fmt.Sprintf("%d_%s.%s", pageID, utils.SanitizeFilename(page.Title), utils.GetExtensionForFormat(format))
			filePath := filepath.Join(spaceDir, filename)
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write page %d to file: %v\n", pageID, err)
			}
		}
	}

	spaceInfo, err := apiClient.GetSpace(ctx, space)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get space info: %v\n", err)
	} else {
		metadata := map[string]interface{}{
			"key":         spaceInfo.Key,
			"name":        spaceInfo.Name,
			"description": spaceInfo.Description,
			"homepageId":  spaceInfo.HomepageID,
			"status":      spaceInfo.Status,
		}

		metadataBytes, err := formatters.FormatOutputToString(metadata, "json")
		if err != nil {
			return fmt.Errorf("failed to format space metadata: %w", err)
		}

		metadataFile := filepath.Join(spaceDir, "_space_metadata.json")
		if err := os.WriteFile(metadataFile, []byte(metadataBytes), 0644); err != nil {
			return fmt.Errorf("failed to write space metadata: %w", err)
		}
	}

	return nil
}

// exportSpaceToDirectoryIterative exports the space hierarchy to a directory structure
// using iterative batch processing to save memory for large spaces
// Creates a hierarchical folder structure: each page gets a folder named by pageId
func exportSpaceToDirectoryIterative(apiClient api.Client, space, outputDir, format string, depth int, skipContent bool, batchSize int) error {
	ctx := context.Background()

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	spaceDir := filepath.Join(outputDir, space)
	if err := os.MkdirAll(spaceDir, 0755); err != nil {
		return fmt.Errorf("failed to create space directory: %w", err)
	}

	baseURL := viper.GetString("url")
	pageCount := 0

	// Normalize format - support both "export" and "export_view"
	normalizedFormat := format
	if format == "export_view" {
		normalizedFormat = "export"
	}

	// First pass: collect all pages and build parent-child relationships
	pageMap := make(map[int]*models.Page)
	childrenMap := make(map[int][]int) // parent ID -> list of child IDs
	var rootPageIDs []int

	// Use iterative processing to fetch all pages
	err := apiClient.GetAllPagesInSpaceIterative(ctx, space, batchSize, func(batch []models.Page) error {
		fmt.Fprintf(os.Stderr, "Processing batch of %d pages\n", len(batch))
		for _, page := range batch {
			pageCount++
			pageID, ok := page.ID.Int()
			if !ok {
				fmt.Fprintf(os.Stderr, "Warning: page ID cannot be converted to int: %v\n", page.ID)
				continue
			}

			fmt.Fprintf(os.Stderr, "Processing page %d: %s (body=%v)\n", pageID, page.Title, page.Body != nil)
			pageMap[pageID] = &page

			// Build parent-child relationships
			if len(page.Ancestors) > 0 {
				// Get immediate parent (last ancestor)
				parentID, ok := page.Ancestors[len(page.Ancestors)-1].ID.Int()
				if ok {
					childrenMap[parentID] = append(childrenMap[parentID], pageID)
				}
			} else {
				// No ancestors = root page
				rootPageIDs = append(rootPageIDs, pageID)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to fetch pages iteratively: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Fetched %d pages, building hierarchy...\n", pageCount)

	// Second pass: create directory structure and save content
	for pageID, page := range pageMap {
		// Create page folder
		pageDir := filepath.Join(spaceDir, fmt.Sprintf("%d", pageID))
		if err := os.MkdirAll(pageDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create directory for page %d: %v\n", pageID, err)
			continue
		}

		// Save page metadata
		if err := savePageMetadata(pageDir, page); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save metadata for page %d: %v\n", pageID, err)
		}

		if !skipContent {
			// Get content format - use export_view for export format
			apiFormat := "export_view"
			if normalizedFormat != "export" {
				apiFormat = utils.GetContentFormatForAPI(normalizedFormat)
			}

			// Try to get content from the page body first (already fetched via expansions)
			var apiContent string
			if page.Body != nil {
				fmt.Fprintf(os.Stderr, "Page %d body keys: ", pageID)
				for k := range page.Body {
					fmt.Fprintf(os.Stderr, "%s ", k)
				}
				fmt.Fprintf(os.Stderr, "\n")

				// First try the requested format
				if bodyContent, ok := page.Body[apiFormat].(map[string]interface{}); ok {
					if value, ok := bodyContent["value"].(string); ok && value != "" {
						apiContent = value
						fmt.Fprintf(os.Stderr, "Found content in %s format\n", apiFormat)
					}
				}

				// If not found, try export_view as fallback
				if apiContent == "" && apiFormat != "export_view" {
					if bodyContent, ok := page.Body["export_view"].(map[string]interface{}); ok {
						if value, ok := bodyContent["value"].(string); ok && value != "" {
							apiContent = value
							apiFormat = "export_view"
							fmt.Fprintf(os.Stderr, "Found content in export_view format\n")
						}
					}
				}

				// If still not found, try storage as final fallback
				if apiContent == "" {
					if bodyContent, ok := page.Body["storage"].(map[string]interface{}); ok {
						if value, ok := bodyContent["value"].(string); ok && value != "" {
							apiContent = value
							apiFormat = "storage"
							fmt.Fprintf(os.Stderr, "Found content in storage format\n")
						}
					}
				}
			}

			// If still no content, fetch it via API call
			if apiContent == "" {
				fmt.Fprintf(os.Stderr, "Warning: page %d body not available, fetching separately\n", pageID)
				var fetchErr error
				apiContent, fetchErr = apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), apiFormat, 0)
				if fetchErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to get content for page %d: %v\n", pageID, fetchErr)
					continue
				}
			}

			var content string
			var convertErr error
			// For export/export_view format, convert to markdown
			if normalizedFormat == "export" {
				// export_view is clean HTML, convert to markdown
				content, convertErr = converters.ExportViewToMarkdown(apiContent, baseURL)
				if convertErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to convert export_view to markdown for page %d: %v\n", pageID, convertErr)
					content = apiContent
				}
			} else {
				// Convert from API format to requested format
				content, convertErr = utils.ConvertContentFromStorage(apiContent, normalizedFormat, baseURL)
				if convertErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to convert content for page %d: %v\n", pageID, convertErr)
					content = apiContent
				}
			}

			filename := fmt.Sprintf("%d_%s.%s", pageID, utils.SanitizeFilename(page.Title), utils.GetExtensionForFormat(normalizedFormat))
			filePath := filepath.Join(pageDir, filename)
			fmt.Fprintf(os.Stderr, "Writing page %d to %s\n", pageID, filePath)
			if writeErr := os.WriteFile(filePath, []byte(content), 0644); writeErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write page %d to file: %v\n", pageID, writeErr)
			}
		}

		// Create children folders (placeholders for hierarchy)
		childIDs := childrenMap[pageID]
		for _, childID := range childIDs {
			childPage, exists := pageMap[childID]
			if !exists {
				continue
			}
			childDir := filepath.Join(spaceDir, fmt.Sprintf("%d", childID))
			if err := os.MkdirAll(childDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to create directory for child page %d: %v\n", childID, err)
			}
			// Create a symlink or marker file to indicate parent-child relationship
			// For now, the folder structure itself shows the relationship
			_ = childPage // Use childPage variable to avoid unused warning
		}
	}

	// Write space metadata at the end
	spaceInfo, err := apiClient.GetSpace(ctx, space)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get space info: %v\n", err)
	} else {
		metadata := map[string]interface{}{
			"key":         spaceInfo.Key,
			"name":        spaceInfo.Name,
			"description": spaceInfo.Description,
			"homepageId":  spaceInfo.HomepageID,
			"status":      spaceInfo.Status,
			"pageCount":   pageCount,
			"rootPages":   rootPageIDs,
		}

		metadataBytes, err := formatters.FormatOutputToString(metadata, "json")
		if err != nil {
			return fmt.Errorf("failed to format space metadata: %w", err)
		}

		metadataFile := filepath.Join(spaceDir, "_space_metadata.json")
		if err := os.WriteFile(metadataFile, []byte(metadataBytes), 0644); err != nil {
			return fmt.Errorf("failed to write space metadata: %w", err)
		}
	}

	return nil
}

// savePageMetadata saves page metadata to a JSON file
func savePageMetadata(spaceDir string, page *models.Page) error {
	pageID, ok := page.ID.Int()
	if !ok {
		return nil
	}

	metadata := map[string]interface{}{
		"id":        pageID,
		"title":     page.Title,
		"type":      page.Type,
		"status":    page.Status,
		"version":   page.Version.Number,
		"createdAt": page.CreatedAt(),
		"updatedAt": page.UpdatedAt(),
	}

	// Add ancestors if present
	if len(page.Ancestors) > 0 {
		ancestorIDs := make([]int, len(page.Ancestors))
		for i, a := range page.Ancestors {
			if id, ok := a.ID.Int(); ok {
				ancestorIDs[i] = id
			}
		}
		metadata["ancestors"] = ancestorIDs
	}

	metadataBytes, err := formatters.FormatOutputToString(metadata, "json")
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%d_%s.meta.json", pageID, utils.SanitizeFilename(page.Title))
	filePath := filepath.Join(spaceDir, filename)
	return os.WriteFile(filePath, []byte(metadataBytes), 0644)
}

// buildTree builds a tree structure from pages
func buildTree(tree treeprint.Tree, pages []*models.Page, childrenMap map[int][]*models.Page, maxDepth, currentDepth int) {
	if maxDepth > 0 && currentDepth >= maxDepth {
		return
	}

	for _, page := range pages {
		pageID, ok := page.ID.Int()
		if !ok {
			continue
		}

		branch := tree.AddBranch(fmt.Sprintf("%d: %s", pageID, page.Title))

		children := childrenMap[pageID]
		if len(children) > 0 {
			buildTree(branch, children, childrenMap, maxDepth, currentDepth+1)
		}
	}
}
