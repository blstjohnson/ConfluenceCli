package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"confcli/pkg/confluence"
	"confcli/pkg/models"
	"confcli/pkg/formatter"
	"confcli/pkg/converter"
	"confcli/pkg/utils"
)

// NewPageCmd creates the page command
func NewPageCmd() *cobra.Command {
	pageCmd := &cobra.Command{
		Use:   "page",
		Short: "Commands for managing Confluence pages",
		Long:  `Commands for retrieving, creating, updating, and deleting Confluence pages`,
	}

	pageCmd.AddCommand(newPageGetCmd())
	pageCmd.AddCommand(newPageCommentsCmd())
	pageCmd.AddCommand(newPageLabelsCmd())
	pageCmd.AddCommand(newPageCreateCmd())
	pageCmd.AddCommand(newPageUpdateCmd())
	pageCmd.AddCommand(newPageDeleteCmd())
	pageCmd.AddCommand(newPageCommentAddCmd())
	pageCmd.AddCommand(newPageLabelAddCmd())

	return pageCmd
}

// newPageGetCmd implements the page get command
func newPageGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [id|title]",
		Short: "Get a page by ID, space/title, or path",
		Long:  "Retrieve a Confluence page by its ID, space and title, or path",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get command-line flags
			id, _ := cmd.Flags().GetInt("id")
			space, _ := cmd.Flags().GetString("space")
			title, _ := cmd.Flags().GetString("title")
			path, _ := cmd.Flags().GetString("path")
			format, _ := cmd.Flags().GetString("format")
			version, _ := cmd.Flags().GetInt("version")
			date, _ := cmd.Flags().GetString("date")
			output, _ := cmd.Flags().GetString("output")
			outputDir, _ := cmd.Flags().GetString("output-dir")
			full, _ := cmd.Flags().GetBool("full")
			withComments, _ := cmd.Flags().GetBool("with-comments")
			withLabels, _ := cmd.Flags().GetBool("with-labels")
			withMetadata, _ := cmd.Flags().GetBool("with-metadata")

			// Validate inputs
			if id == 0 && title == "" && path == "" {
				return fmt.Errorf("must specify either --id, --space and --title, or --path")
			}

			if (space != "" && title == "") || (space == "" && title != "") {
				if path == "" {
					return fmt.Errorf("must specify both --space and --title together, or use --path")
				}
			}

			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			var page *models.Page

			// Determine how to retrieve the page
			if id != 0 {
				page, err = apiClient.GetPage(ctx, id)
			} else if path != "" {
				// Parse path to get space and title
				parts := strings.Split(path, "/")
				if len(parts) < 2 {
					return fmt.Errorf("invalid path format, expected Space/Page or Space/Parent/Child/.../Page")
				}
				space = parts[0]
				title = parts[len(parts)-1] // Last part is the page title
				page, err = apiClient.GetPageByTitle(ctx, space, title)
			} else {
				page, err = apiClient.GetPageByTitle(ctx, space, title)
			}

			if err != nil {
				return fmt.Errorf("failed to get page: %w", err)
			}

			// If version or date is specified, we might need to get historical content
			if version > 0 || date != "" {
				// Note: Getting historical content would require additional API calls
				// This is a simplified implementation
				if version > 0 && version != page.Version.Number {
					return fmt.Errorf("getting specific version not implemented in this example")
				}
			}

			// Get only the storage format as the main content initially
			content, err := apiClient.GetPageContent(ctx, page.ID.IntOrString(), "storage")
			if err != nil {
				return fmt.Errorf("failed to get page storage content: %w", err)
			}

			// Apply transformations to the storage content if needed
			transformedContent := content
			if format != "storage" {
				// Apply transformation based on requested format
				switch format {
				case "markdown":
					transformedContent, err = converter.StorageToMarkdown(content)
					if err != nil {
						return fmt.Errorf("failed to convert storage to markdown: %w", err)
					}
				case "html":
					// For now, we'll just use the storage content as HTML since it's already HTML-like
					// In a real implementation, we might need to clean up the storage format
					transformedContent = content
				case "plain":
					// For plain text, strip HTML tags from storage content
					transformedContent = utils.StripHTMLTags(content)
				}
			}

			// Get additional data only if requested
			if withComments || full {
				pageID, ok := page.ID.Int()
				if !ok {
					fmt.Fprintf(os.Stderr, "Warning: page ID is not an integer, cannot retrieve comments\n")
				} else {
					comments, err := apiClient.GetComments(ctx, pageID)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Warning: could not retrieve comments: %v\n", err)
					} else {
						page.Comments = comments
					}
				}
			}

			if withLabels || full {
				pageID, ok := page.ID.Int()
				if !ok {
					fmt.Fprintf(os.Stderr, "Warning: page ID is not an integer, cannot retrieve labels\n")
				} else {
					labels, err := apiClient.GetLabels(ctx, pageID)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Warning: could not retrieve labels: %v\n", err)
					} else {
						page.Labels = labels
					}
				}
			}

			if withMetadata || full {
				// Get additional metadata if requested
				// This could include history, properties, etc.
				expandedPage, err := apiClient.GetPageWithExpansions(ctx, page.ID.IntOrString(), []string{
					"metadata.labels", "metadata.properties", "metadata.history",
					"history", "history.lastUpdated", "history.contributors",
					"version", "ancestors", "operations",
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not retrieve additional metadata: %v\n", err)
				} else {
					// Update the page with expanded data
					page.Labels = expandedPage.Labels
					page.Ancestors = expandedPage.Ancestors
					page.Version = expandedPage.Version
				}
			}

			// Determine output format
			outputFormat := viper.GetString("output_format")

			// Handle output to file or directory
			if output != "" {
				if err := os.WriteFile(output, []byte(transformedContent), 0644); err != nil {
					return fmt.Errorf("failed to write content to file: %w", err)
				}
				fmt.Printf("Content saved to %s\n", output)
			} else if outputDir != "" {
				// Create output directory if it doesn't exist
				if err := os.MkdirAll(outputDir, 0755); err != nil {
					return fmt.Errorf("failed to create output directory: %w", err)
				}

				// Write content to file
				idStr := page.ID.String()
				filename := fmt.Sprintf("%s.%s", idStr, utils.GetExtensionForFormat(format))
				filePath := filepath.Join(outputDir, filename)
				if err := os.WriteFile(filePath, []byte(transformedContent), 0644); err != nil {
					return fmt.Errorf("failed to write content to file: %w", err)
				}

				// If full export is requested, also save metadata
				if full {
					metadata := map[string]interface{}{
						"id":          page.ID,
						"title":       page.Title,
						"spaceId":     page.SpaceID,
						"status":      page.Status,
						"createdAt":   page.CreatedAt,
						"updatedAt":   page.UpdatedAt,
						"version":     page.Version,
						"authorId":    page.AuthorID,
						"labels":      page.Labels,
						"comments":    page.Comments,
						"attachments": page.Attachments,
					}

					metadataBytes, err := formatter.FormatOutputToString(metadata, "json")
					if err != nil {
						return fmt.Errorf("failed to format metadata: %w", err)
					}

					metadataFilePath := filepath.Join(outputDir, fmt.Sprintf("%s.metadata.json", idStr))
					if err := os.WriteFile(metadataFilePath, []byte(metadataBytes), 0644); err != nil {
						return fmt.Errorf("failed to write metadata to file: %w", err)
					}
				}

				fmt.Printf("Page exported to %s\n", outputDir)
			} else {
				// Output to stdout
				if outputFormat == "json" {
					// For JSON output, include all requested data
					data := map[string]interface{}{
						"id":       page.ID,
						"title":    page.Title,
						"version":  page.Version.Number,
						"space":    page.SpaceID,
						"content": map[string]string{
							"storage": content, // Always include the raw storage content
						},
						"transformedContent": map[string]string{
							format: transformedContent, // Include the transformed content in the requested format
						},
						"metadata": map[string]interface{}{
							"created":  page.CreatedAt,
							"author":   page.AuthorID,
							"modified": page.UpdatedAt,
						},
					}

					if withLabels || full {
						data["labels"] = page.Labels
					}
					if withComments || full {
						data["comments"] = page.Comments
					}
					if withMetadata || full {
						// Add more metadata fields
						data["ancestors"] = page.Ancestors
						data["versions"] = []interface{}{page.Version} // Simplified
					}

					return formatter.FormatOutput(data, "json")
				} else {
					// For text output, just show the page info
					return formatter.FormatOutput(page, outputFormat)
				}
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().Int("id", 0, "Page ID")
	cmd.Flags().String("space", "", "Space key")
	cmd.Flags().String("title", "", "Page title")
	cmd.Flags().String("path", "", "Page path (e.g., Space/Parent/Child)")
	cmd.Flags().StringP("format", "f", "markdown", "Content format: markdown, storage, html, plain")
	cmd.Flags().Int("version", 0, "Version number (0 = latest)")
	cmd.Flags().String("date", "", "Date in YYYY-MM-DD format")
	cmd.Flags().StringP("output", "o", "", "Save content to file")
	cmd.Flags().String("output-dir", "", "Save full page to directory")
	cmd.Flags().Bool("full", false, "Full export (content in multiple formats, metadata, comments, history)")
	cmd.Flags().Bool("with-comments", false, "Include comments in output")
	cmd.Flags().Bool("with-labels", false, "Include labels in output")
	cmd.Flags().Bool("with-metadata", false, "Include additional metadata in output")

	return cmd
}


func newPageCommentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "comments <id>",
		Short: "Get comments for a page",
		Long:  "Retrieve comments for a specific page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid page ID: %w", err)
			}

			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Get the page to verify it exists
			page, err := apiClient.GetPage(ctx, pageID)
			if err != nil {
				return fmt.Errorf("failed to get page: %w", err)
			}

			// Get comments for the page
			// Note: This is a simplified implementation - in reality, comments would come from a separate API endpoint
			// For now, we'll use the comments field from the page object
			comments := page.Comments

			// Determine output format
			outputFormat := viper.GetString("output_format")

			// Format and output comments
			return formatter.FormatOutput(comments, outputFormat)
		},
	}
}

func newPageLabelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "labels <id>",
		Short: "Get labels for a page",
		Long:  "Retrieve labels for a specific page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid page ID: %w", err)
			}

			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Get the page to verify it exists
			page, err := apiClient.GetPage(ctx, pageID)
			if err != nil {
				return fmt.Errorf("failed to get page: %w", err)
			}

			// Get labels for the page
			labels := page.Labels

			// Determine output format
			outputFormat := viper.GetString("output_format")

			// Format and output labels
			return formatter.FormatOutput(labels, outputFormat)
		},
	}
}

func newPageCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new page",
		Long:  "Create a new Confluence page",
		RunE: func(cmd *cobra.Command, args []string) error {
			space, _ := cmd.Flags().GetString("space")
			title, _ := cmd.Flags().GetString("title")
			parent, _ := cmd.Flags().GetInt("parent")
			contentFile, _ := cmd.Flags().GetString("content-file")
			contentStdin, _ := cmd.Flags().GetString("content-stdin")
			format, _ := cmd.Flags().GetString("format")
			confirm, _ := cmd.Flags().GetBool("confirm")

			// Check if confirmation is required
			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Get content
			var content string
			if contentFile != "" {
				contentBytes, err := os.ReadFile(contentFile)
				if err != nil {
					return fmt.Errorf("failed to read content file: %w", err)
				}
				content = string(contentBytes)
			} else if contentStdin != "" {
				content = contentStdin
			} else {
				return fmt.Errorf("either --content-file or --content-stdin must be provided")
			}

			// Prepare parent ID
			var parentID *int
			if parent != 0 {
				parentID = &parent
			}

			// Create the page
			newPage, err := apiClient.CreatePage(ctx, space, parentID, title, content, format)
			if err != nil {
				return fmt.Errorf("failed to create page: %w", err)
			}

			// Determine output format
			outputFormat := viper.GetString("output_format")

			// Format and output the created page
			return formatter.FormatOutput(newPage, outputFormat)
		},
	}

	cmd.Flags().String("space", "", "Space key (required)")
	cmd.Flags().String("title", "", "Page title (required)")
	cmd.Flags().Int("parent", 0, "Parent page ID")
	cmd.Flags().String("content-file", "", "Path to content file")
	cmd.Flags().String("content-stdin", "", "Content from stdin")
	cmd.Flags().String("format", "storage", "Content format: storage, markdown")
	cmd.Flags().Bool("confirm", false, "Confirm creation (required for write operations)")
	cmd.MarkFlagRequired("space")
	cmd.MarkFlagRequired("title")

	return cmd
}

func newPageUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an existing page",
		Long:  "Update an existing Confluence page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid page ID: %w", err)
			}

			contentFile, _ := cmd.Flags().GetString("content-file")
			versionComment, _ := cmd.Flags().GetString("version-comment")
			confirm, _ := cmd.Flags().GetBool("confirm")

			// Check if confirmation is required
			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Get content
			var content string
			if contentFile != "" {
				contentBytes, err := os.ReadFile(contentFile)
				if err != nil {
					return fmt.Errorf("failed to read content file: %w", err)
				}
				content = string(contentBytes)
			} else {
				return fmt.Errorf("content file must be provided with --content-file")
			}

			// Update the page
			updatedPage, err := apiClient.UpdatePage(ctx, pageID, content, versionComment)
			if err != nil {
				return fmt.Errorf("failed to update page: %w", err)
			}

			// Determine output format
			outputFormat := viper.GetString("output_format")

			// Format and output the updated page
			return formatter.FormatOutput(updatedPage, outputFormat)
		},
	}

	cmd.Flags().String("content-file", "", "Path to content file")
	cmd.Flags().String("version-comment", "", "Version comment")
	cmd.Flags().Bool("confirm", false, "Confirm update (required for write operations)")

	return cmd
}

func newPageDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a page",
		Long:  "Delete a Confluence page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid page ID: %w", err)
			}

			confirm, _ := cmd.Flags().GetBool("confirm")

			// Check if confirmation is required
			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Delete the page
			err = apiClient.DeletePage(ctx, pageID)
			if err != nil {
				return fmt.Errorf("failed to delete page: %w", err)
			}

			fmt.Printf("Page %d deleted successfully\n", pageID)
			return nil
		},
	}

	cmd.Flags().Bool("confirm", false, "Confirm deletion (required)")

	return cmd
}

func newPageCommentAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment add <id>",
		Short: "Add a comment to a page",
		Long:  "Add a comment to a Confluence page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid page ID: %w", err)
			}

			text, _ := cmd.Flags().GetString("text")
			parentComment, _ := cmd.Flags().GetInt("parent-comment")
			confirm, _ := cmd.Flags().GetBool("confirm")

			// Check if confirmation is required
			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Prepare parent comment ID
			var parentCommentID *int
			if parentComment != 0 {
				parentCommentID = &parentComment
			}

			// Add the comment
			newComment, err := apiClient.AddComment(ctx, pageID, text, parentCommentID)
			if err != nil {
				return fmt.Errorf("failed to add comment: %w", err)
			}

			// Determine output format
			outputFormat := viper.GetString("output_format")

			// Format and output the created comment
			return formatter.FormatOutput(newComment, outputFormat)
		},
	}

	cmd.Flags().String("text", "", "Comment text (required)")
	cmd.Flags().Int("parent-comment", 0, "Parent comment ID for nested comments")
	cmd.Flags().Bool("confirm", false, "Confirm addition (required for write operations)")
	cmd.MarkFlagRequired("text")

	return cmd
}

func newPageLabelAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label add <id>",
		Short: "Add a label to a page",
		Long:  "Add a label to a Confluence page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid page ID: %w", err)
			}

			label, _ := cmd.Flags().GetString("label")
			confirm, _ := cmd.Flags().GetBool("confirm")

			// Check if confirmation is required
			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Add the label
			err = apiClient.AddLabel(ctx, pageID, label)
			if err != nil {
				return fmt.Errorf("failed to add label: %w", err)
			}

			fmt.Printf("Label '%s' added to page %d successfully\n", label, pageID)
			return nil
		},
	}

	cmd.Flags().String("label", "", "Label name (required)")
	cmd.Flags().Bool("confirm", false, "Confirm addition (required for write operations)")
	cmd.MarkFlagRequired("label")

	return cmd
}