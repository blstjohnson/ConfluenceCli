package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xlab/treeprint"

	"confcli/pkg/clients"
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
				if err := exportSpaceToDirectory(apiClient, space, outputDir, format, depth, skipContent); err != nil {
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
	cmd.Flags().String("format", "markdown", "Format for saved pages: markdown, storage, both")
	cmd.Flags().Int("depth", 0, "Recursion depth (default: unlimited)")
	cmd.Flags().Bool("flat", false, "Output flat list instead of tree")
	cmd.Flags().Bool("tree", false, "Display ASCII tree in console")
	cmd.Flags().Bool("skip-content", false, "Export only structure and metadata, no content")
	cmd.MarkFlagRequired("space")

	return cmd
}

// exportSpaceToDirectory exports the space hierarchy to a directory structure
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

	for _, page := range pages {
		pageID, _ := page.ID.Int()
		if !skipContent {
			// Get content format - Confluence only supports "storage" and "editor" formats
			apiFormat := utils.GetContentFormatForAPI(format)
			storageContent, err := apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), apiFormat, 0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to get content for page %d: %v\n", pageID, err)
			} else {
				// Convert from storage to requested format if needed
				content, err := utils.ConvertContentFromStorage(storageContent, format, "")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to convert content for page %d: %v\n", pageID, err)
				} else {
					filename := fmt.Sprintf("%d_%s.%s", pageID, utils.SanitizeFilename(page.Title), utils.GetExtensionForFormat(format))
					filePath := filepath.Join(spaceDir, filename)
					if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: failed to write page %d to file: %v\n", pageID, err)
					}
				}
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
