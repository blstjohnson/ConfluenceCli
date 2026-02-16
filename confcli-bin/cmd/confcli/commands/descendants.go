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

	"confcli/pkg/clients"
	"confcli/pkg/models"
	"confcli/pkg/formatters"
	"confcli/pkg/utils"
	"confcli/pkg/usecases"
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
			id, _ := cmd.Flags().GetInt("id")
			path, _ := cmd.Flags().GetString("path")
			depth, _ := cmd.Flags().GetInt("depth")
			outputDir, _ := cmd.Flags().GetString("output-dir")
			format, _ := cmd.Flags().GetString("format")
			flat, _ := cmd.Flags().GetBool("flat")
			treeView, _ := cmd.Flags().GetBool("tree")
			includeSelf, _ := cmd.Flags().GetBool("include-self")
			skipContent, _ := cmd.Flags().GetBool("skip-content")

			if id == 0 && path == "" {
				return fmt.Errorf("must specify either --id or --path")
			}

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()
			pageUseCase := usecases.NewPageUseCase(apiClient)

			var targetPage *models.Page
			var pageID int

			if id != 0 {
				targetPage, err = apiClient.GetPage(ctx, id)
				if err != nil {
					return fmt.Errorf("failed to get page: %w", err)
				}
				pageID = id
			} else {
				parts := strings.Split(path, "/")
				if len(parts) < 2 {
					return fmt.Errorf("invalid path format, expected Space/Page or Space/Parent/Child/.../Page")
				}
				space := parts[0]
				title := parts[len(parts)-1]
				targetPage, err = apiClient.GetPageByTitle(ctx, space, title)
				if err != nil {
					return fmt.Errorf("failed to get page by title: %w", err)
				}
				var ok bool
				pageID, ok = targetPage.ID.Int()
				if !ok {
					return fmt.Errorf("target page ID is not an integer: %v", targetPage.ID)
				}
			}

			hierarchyReq := &usecases.GetPageHierarchyRequest{
				PageID: pageID,
				Depth:  depth,
			}

			hierarchyResp, err := pageUseCase.GetPageHierarchy(ctx, hierarchyReq)
			if err != nil {
				return fmt.Errorf("failed to get page hierarchy: %w", err)
			}

			descendants := hierarchyResp.Descendants
			outputFormat := viper.GetString("output_format")

			if flat {
				if outputFormat == "json" {
					flatPages := make([]map[string]interface{}, len(descendants))
					for i, page := range descendants {
						flatPages[i] = map[string]interface{}{
							"id":       page.ID,
							"title":    page.Title,
							"parentId": pageID,
							"depth":    1,
						}
					}
					return formatters.FormatOutput(flatPages, "json")
				} else {
					return formatters.FormatOutput(descendants, "text")
				}
			} else if treeView {
				tree := treeprint.New()
				if includeSelf {
					sourceID, _ := targetPage.ID.Int()
					sourceBranch := tree.AddBranch(fmt.Sprintf("%d: %s", sourceID, targetPage.Title))
					for _, descendant := range descendants {
						descendantID, _ := descendant.ID.Int()
						sourceBranch.AddBranch(fmt.Sprintf("%d: %s", descendantID, descendant.Title))
					}
				} else {
					for _, descendant := range descendants {
						descendantID, _ := descendant.ID.Int()
						tree.AddBranch(fmt.Sprintf("%d: %s", descendantID, descendant.Title))
					}
				}
				fmt.Println(tree.String())
			} else if outputDir != "" {
				if err := exportDescendantsToDirectory(apiClient, targetPage, descendants, outputDir, format, skipContent, includeSelf); err != nil {
					return fmt.Errorf("failed to export descendants to directory: %w", err)
				}
				fmt.Printf("Descendants of page %d exported to %s\n", pageID, outputDir)
			} else {
				tree := treeprint.New()
				currentTree := tree
				if includeSelf {
					sourceID, _ := targetPage.ID.Int()
					currentTree = tree.AddBranch(fmt.Sprintf("%d: %s", sourceID, targetPage.Title))
				}
				for _, descendant := range descendants {
					descendantID, _ := descendant.ID.Int()
					currentTree.AddBranch(fmt.Sprintf("%d: %s", descendantID, descendant.Title))
				}
				fmt.Println(tree.String())
			}

			return nil
		},
	}

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
func exportDescendantsToDirectory(apiClient interface{ GetPageContent(context.Context, interface{}, string) (string, error) }, sourcePage *models.Page, descendants []models.Page, outputDir, format string, skipContent, includeSelf bool) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	baseDir := outputDir
	if includeSelf {
		sourceID, _ := sourcePage.ID.Int()
		sourceDir := filepath.Join(outputDir, fmt.Sprintf("%d_%s", sourceID, utils.SanitizeFilename(sourcePage.Title)))
		if err := os.MkdirAll(sourceDir, 0755); err != nil {
			return fmt.Errorf("failed to create source page directory: %w", err)
		}
		baseDir = sourceDir
	}

	for _, page := range descendants {
		pageID, _ := page.ID.Int()
		if !skipContent {
			content, err := apiClient.GetPageContent(context.Background(), page.ID.IntOrString(), format)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to get content for page %d: %v\n", pageID, err)
			} else {
				filename := fmt.Sprintf("%d_%s.%s", pageID, utils.SanitizeFilename(page.Title), utils.GetExtensionForFormat(format))
				filePath := filepath.Join(baseDir, filename)
				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write page %d to file: %v\n", pageID, err)
				}
			}
		}
	}

	return nil
}
