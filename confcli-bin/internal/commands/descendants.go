package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xlab/treeprint"

	"confcli/internal/client"
	"confcli/internal/formatter"
	"confcli/internal/utils"
)

// NewDescendantsCmd creates the descendants command
func NewDescendantsCmd() *cobra.Command {
	descendantsCmd := &cobra.Command{
		Use:   "descendants",
		Short: "Commands for working with page descendants",
		Long:  `Commands for retrieving and exporting page descendants`,
	}

	descendantsCmd.AddCommand(newDescendantsGetCmd())

	return descendantsCmd
}

// newDescendantsGetCmd implements the descendants get command
func newDescendantsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get descendants of a page",
		Long:  "Retrieve and export descendants of a specific Confluence page",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get command-line flags
			id, _ := cmd.Flags().GetInt("id")
			path, _ := cmd.Flags().GetString("path")
			depth, _ := cmd.Flags().GetInt("depth")
			outputDir, _ := cmd.Flags().GetString("output-dir")
			format, _ := cmd.Flags().GetString("format")
			flat, _ := cmd.Flags().GetBool("flat")
			treeView, _ := cmd.Flags().GetBool("tree")
			includeSelf, _ := cmd.Flags().GetBool("include-self")
			skipContent, _ := cmd.Flags().GetBool("skip-content")
			
			// Validate inputs
			if id == 0 && path == "" {
				return fmt.Errorf("must specify either --id or --path")
			}
			
			// Create API client
			apiClient, err := client.NewClient()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}
			
			ctx := context.Background()
			
			var targetPage *client.Page
			var pageID int
			
			// Determine the target page
			if id != 0 {
				targetPage, err = apiClient.GetPage(ctx, id)
				if err != nil {
					return fmt.Errorf("failed to get page: %w", err)
				}
				pageID = id
			} else {
				// Parse path to get space and title
				parts := strings.Split(path, "/")
				if len(parts) < 2 {
					return fmt.Errorf("invalid path format, expected Space/Page or Space/Parent/Child/.../Page")
				}
				space := parts[0]
				title := parts[len(parts)-1] // Last part is the page title
				targetPage, err = apiClient.GetPageByTitle(ctx, space, title)
				if err != nil {
					return fmt.Errorf("failed to get page by title: %w", err)
				}
				pageID = targetPage.ID
			}
			
			// Get descendants
			descendants, err := apiClient.GetDescendants(ctx, pageID, depth)
			if err != nil {
				return fmt.Errorf("failed to get descendants: %w", err)
			}
			
			// Determine output format
			outputFormat := viper.GetString("output_format")
			
			// Handle different output modes
			if flat {
				// Output as flat list
				if outputFormat == "json" {
					// Convert pages to flat structure with parent IDs
					flatPages := make([]map[string]interface{}, len(descendants))
					for i, page := range descendants {
						flatPages[i] = map[string]interface{}{
							"id":       page.ID,
							"title":    page.Title,
							"parentId": pageID, // Simplified - in reality, we'd need to determine actual parent
							"depth":    1,      // Simplified - in reality, we'd calculate actual depth
						}
					}
					return formatter.FormatOutput(flatPages, "json")
				} else {
					return formatter.FormatOutput(descendants, "text")
				}
			} else if treeView {
				// Display as ASCII tree
				tree := treeprint.New()
				
				// Add the source page if include-self is true
				if includeSelf {
					tree = treeprint.New()
					sourceBranch := tree.AddBranch(fmt.Sprintf("%d: %s", targetPage.ID, targetPage.Title))
					for _, descendant := range descendants {
						sourceBranch.AddBranch(fmt.Sprintf("%d: %s", descendant.ID, descendant.Title))
					}
				} else {
					// Just show descendants
					for _, descendant := range descendants {
						tree.AddBranch(fmt.Sprintf("%d: %s", descendant.ID, descendant.Title))
					}
				}
				
				fmt.Println(tree.String())
			} else if outputDir != "" {
				// Export to directory structure
				if err := exportDescendantsToDirectory(apiClient, targetPage, descendants, outputDir, format, depth, skipContent, includeSelf); err != nil {
					return fmt.Errorf("failed to export descendants to directory: %w", err)
				}
				fmt.Printf("Descendants of page %d exported to %s\n", pageID, outputDir)
			} else {
				// Default: show tree in console
				tree := treeprint.New()
				
				// Add the source page if include-self is true
				currentTree := tree
				if includeSelf {
					currentTree = tree.AddBranch(fmt.Sprintf("%d: %s", targetPage.ID, targetPage.Title))
				}
				
				// Add descendants to the tree
				for _, descendant := range descendants {
					currentTree.AddBranch(fmt.Sprintf("%d: %s", descendant.ID, descendant.Title))
				}
				
				fmt.Println(tree.String())
			}
			
			return nil
		},
	}
	
	// Add flags
	cmd.Flags().Int("id", 0, "Page ID")
	cmd.Flags().String("path", "", "Page path (e.g., Space/Parent/Child)")
	cmd.Flags().Int("depth", 0, "Recursion depth (default: unlimited)")
	cmd.Flags().String("output-dir", "", "Export subtree to directory")
	cmd.Flags().String("format", "markdown", "Format for saved pages: markdown, storage, both")
	cmd.Flags().Bool("flat", false, "Output flat list instead of tree")
	cmd.Flags().Bool("tree", false, "Display ASCII tree in console")
	cmd.Flags().Bool("include-self", false, "Include the source page in output/export")
	cmd.Flags().Bool("skip-content", false, "Export only structure, no content")
	
	return cmd
}

// exportDescendantsToDirectory exports the descendants to a directory structure
func exportDescendantsToDirectory(apiClient *client.Client, sourcePage *client.Page, descendants []client.Page, outputDir, format string, depth int, skipContent, includeSelf bool) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// If include-self is true, create a directory for the source page
	baseDir := outputDir
	if includeSelf {
		sourceDir := filepath.Join(outputDir, fmt.Sprintf("%d_%s", sourcePage.ID, utils.SanitizeFilename(sourcePage.Title)))
		if err := os.MkdirAll(sourceDir, 0755); err != nil {
			return fmt.Errorf("failed to create source page directory: %w", err)
		}
		baseDir = sourceDir
	}
	
	// Export each descendant page
	for _, page := range descendants {
		if err := utils.ExportPageToFile(apiClient, page, baseDir, format, skipContent); err != nil {
			// Log error but continue with other pages
			fmt.Fprintf(os.Stderr, "Warning: failed to export descendant page %d: %v\n", page.ID, err)
		}
	}
	
	return nil
}

