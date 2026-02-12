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
	"confcli/pkg/confluence"
	"confcli/pkg/models"
	"confcli/internal/formatter"
	"confcli/pkg/utils"
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
			// Get command-line flags
			space, _ := cmd.Flags().GetString("space")
			outputDir, _ := cmd.Flags().GetString("output-dir")
			format, _ := cmd.Flags().GetString("format")
			depth, _ := cmd.Flags().GetInt("depth")
			flat, _ := cmd.Flags().GetBool("flat")
			treeView, _ := cmd.Flags().GetBool("tree")
			skipContent, _ := cmd.Flags().GetBool("skip-content")
			exportAttachments, _ := cmd.Flags().GetBool("export-attachments")
			
			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}
			
			ctx := context.Background()
			
			// Get all pages in the space
			pages, err := apiClient.GetAllPagesInSpace(ctx, space)
			if err != nil {
				return fmt.Errorf("failed to get pages in space %s: %w", space, err)
			}
			
			// Determine output format
			outputFormat := viper.GetString("output_format")
			
			// Handle different output modes
			if flat {
				// Output as flat list
				if outputFormat == "json" {
					// Convert pages to flat structure with parent IDs
					flatPages := make([]map[string]interface{}, len(pages))
					for i, page := range pages {
						flatPages[i] = map[string]interface{}{
							"id":       page.ID,
							"title":    page.Title,
							"parentId": nil, // Need to determine parent relationships
							"depth":    0,   // Need to calculate depth
						}
					}
					return formatter.FormatOutput(flatPages, "json")
				} else {
					return formatter.FormatOutput(pages, "text")
				}
			} else if treeView {
				// Display as ASCII tree
				tree := treeprint.New()
				
				// For simplicity, we'll just show top-level pages
				// A full implementation would build the tree structure
				for _, page := range pages {
					pageID, _ := page.ID.Int() // Use 0 if not an integer
					tree.AddBranch(fmt.Sprintf("%d: %s", pageID, page.Title))
				}
				
				fmt.Println(tree.String())
			} else if outputDir != "" {
				// Export to directory structure
				if err := exportSpaceToDirectory(apiClient, space, outputDir, format, depth, skipContent, exportAttachments); err != nil {
					return fmt.Errorf("failed to export space to directory: %w", err)
				}
				fmt.Printf("Space %s exported to %s\n", space, outputDir)
			} else {
				// Default: show tree in console
				tree := treeprint.New()
				
				// Group pages by parent for tree structure
				pageMap := make(map[int]*models.Page)
				childrenMap := make(map[int][]*models.Page)

				for i := range pages {
					page := &pages[i]
					pageID, ok := page.ID.Int()
					if ok {
						pageMap[pageID] = page

						// This is a simplified approach - in reality, we'd need to determine parent-child relationships
						// For now, we'll just treat all pages as top-level
						childrenMap[0] = append(childrenMap[0], page)
					} else {
						// Skip pages with non-integer IDs
						continue
					}
				}
				
				// Build the tree
				buildTree(tree, childrenMap[0], childrenMap, depth, 0)
				
				fmt.Println(tree.String())
			}
			
			return nil
		},
	}
	
	// Add flags
	cmd.Flags().String("space", "", "Space key (required)")
	cmd.Flags().String("output-dir", "", "Export space to directory")
	cmd.Flags().String("format", "markdown", "Format for saved pages: markdown, storage, both")
	cmd.Flags().Int("depth", 0, "Recursion depth (default: unlimited)")
	cmd.Flags().Bool("flat", false, "Output flat list instead of tree")
	cmd.Flags().Bool("tree", false, "Display ASCII tree in console")
	cmd.Flags().Bool("skip-content", false, "Export only structure and metadata, no content")
	cmd.Flags().Bool("export-attachments", false, "Export attachments as well")
	cmd.MarkFlagRequired("space")
	
	return cmd
}

// exportSpaceToDirectory exports the space hierarchy to a directory structure
func exportSpaceToDirectory(apiClient api.Client, space, outputDir, format string, depth int, skipContent, exportAttachments bool) error {
	ctx := context.Background()
	
	// Get all pages in the space
	pages, err := apiClient.GetAllPagesInSpace(ctx, space)
	if err != nil {
		return fmt.Errorf("failed to get pages in space: %w", err)
	}
	
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// Create space directory
	spaceDir := filepath.Join(outputDir, space)
	if err := os.MkdirAll(spaceDir, 0755); err != nil {
		return fmt.Errorf("failed to create space directory: %w", err)
	}

	// Group pages by parent for directory structure
	pageMap := make(map[int]*models.Page)
	childrenMap := make(map[int][]*models.Page)

	for i := range pages {
		page := &pages[i]
		pageID, ok := page.ID.Int()
		if ok {
			pageMap[pageID] = page

			// This is a simplified approach - in reality, we'd need to determine parent-child relationships
			// For now, we'll just put all pages in the root
			childrenMap[0] = append(childrenMap[0], page)
		} else {
			// Skip pages with non-integer IDs
			continue
		}
	}

	// Export pages to files
	for _, page := range pages {
		pageID, _ := page.ID.Int() // Use 0 if not an integer
		// Export page content to file
		if !skipContent {
			content, err := apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), format)
			if err != nil {
				// Log error but continue with other pages
				fmt.Fprintf(os.Stderr, "Warning: failed to get content for page %d: %v\n", pageID, err)
			} else {
				// Create filename based on page ID and title
				filename := fmt.Sprintf("%d_%s.%s", pageID, utils.SanitizeFileName(page.Title), utils.GetExtensionForFormat(format))
				filePath := filepath.Join(spaceDir, filename)
				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write page %d to file: %v\n", pageID, err)
				}
			}
		}
	}
	
	// Export space metadata
	spaceInfo, err := apiClient.GetSpace(ctx, space)
	if err != nil {
		// Log error but don't fail the entire operation
		fmt.Fprintf(os.Stderr, "Warning: failed to get space info: %v\n", err)
	} else {
		metadata := map[string]interface{}{
			"key":         spaceInfo.Key,
			"name":        spaceInfo.Name,
			"description": spaceInfo.Description,
			"homepageId":  spaceInfo.HomepageID,
			"status":      spaceInfo.Status,
		}
		
		metadataBytes, err := formatter.FormatOutputToString(metadata, "json")
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
			// Skip pages with non-integer IDs
			continue
		}

		branch := tree.AddBranch(fmt.Sprintf("%d: %s", pageID, page.Title))

		// Add children to this branch
		children := childrenMap[pageID]
		if len(children) > 0 {
			buildTree(branch, children, childrenMap, maxDepth, currentDepth+1)
		}
	}
}

