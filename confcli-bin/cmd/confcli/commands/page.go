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

	"confcli/pkg/clients"
	"confcli/pkg/formatters"
	"confcli/pkg/converters"
	"confcli/pkg/utils"
	"confcli/pkg/usecases"
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
			output, _ := cmd.Flags().GetString("output")
			outputDir, _ := cmd.Flags().GetString("output-dir")
			withComments, _ := cmd.Flags().GetBool("with-comments")
			withLabels, _ := cmd.Flags().GetBool("with-labels")

			// Validate inputs
			if id == 0 && title == "" && path == "" {
				return fmt.Errorf("must specify either --id, --space and --title, or --path")
			}

			if (space != "" && title == "") || (space == "" && title != "") {
				if path == "" {
					return fmt.Errorf("must specify both --space and --title together, or use --path")
				}
			}

			// Create API client and usecase
			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}
			
			pageUseCase := usecases.NewPageUseCase(apiClient)
			ctx := context.Background()

			// Parse path if provided
			if path != "" {
				parts := strings.Split(path, "/")
				if len(parts) < 2 {
					return fmt.Errorf("invalid path format, expected Space/Page or Space/Parent/Child/.../Page")
				}
				space = parts[0]
				title = parts[len(parts)-1]
			}

			// Use usecase to get page with content
			req := &usecases.GetPageWithContentRequest{
				PageID:       id,
				SpaceKey:     space,
				Title:        title,
				Format:       format,
				WithComments: withComments,
				WithLabels:   withLabels,
			}

			resp, err := pageUseCase.GetPageWithContent(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to get page: %w", err)
			}

			// Apply content transformation if needed
			transformedContent := resp.Content
			if format != "storage" {
				switch format {
				case "markdown":
					transformedContent, err = converters.StorageToMarkdown(resp.Content)
					if err != nil {
						return fmt.Errorf("failed to convert storage to markdown: %w", err)
					}
				case "plain":
					transformedContent = utils.StripHTMLTags(resp.Content)
				}
			}

			// Handle output
			outputFormat := viper.GetString("output_format")
			
			if output != "" {
				if err := os.WriteFile(output, []byte(transformedContent), 0644); err != nil {
					return fmt.Errorf("failed to write content to file: %w", err)
				}
				fmt.Printf("Content saved to %s\n", output)
			} else if outputDir != "" {
				if err := os.MkdirAll(outputDir, 0755); err != nil {
					return fmt.Errorf("failed to create output directory: %w", err)
				}

				idStr := resp.Page.ID.String()
				filename := fmt.Sprintf("%s.%s", idStr, utils.GetExtensionForFormat(format))
				filePath := filepath.Join(outputDir, filename)
				if err := os.WriteFile(filePath, []byte(transformedContent), 0644); err != nil {
					return fmt.Errorf("failed to write content to file: %w", err)
				}
				fmt.Printf("Page exported to %s\n", outputDir)
			} else {
				if outputFormat == "json" {
					data := map[string]interface{}{
						"id":       resp.Page.ID,
						"title":    resp.Page.Title,
						"version":  resp.Page.Version.Number,
						"space":    resp.Page.SpaceID,
						"content": map[string]string{
							"storage": resp.Content,
						},
						"transformedContent": map[string]string{
							format: transformedContent,
						},
					}
					if withLabels {
						data["labels"] = resp.Labels
					}
					if withComments {
						data["comments"] = resp.Comments
					}
					return formatters.FormatOutput(data, "json")
				} else {
					return formatters.FormatOutput(resp.Page, outputFormat)
				}
			}

			return nil
		},
	}

	cmd.Flags().Int("id", 0, "Page ID")
	cmd.Flags().String("space", "", "Space key")
	cmd.Flags().String("title", "", "Page title")
	cmd.Flags().String("path", "", "Page path (e.g., Space/Parent/Child)")
	cmd.Flags().StringP("format", "f", "markdown", "Content format: markdown, storage, html, plain")
	cmd.Flags().StringP("output", "o", "", "Save content to file")
	cmd.Flags().String("output-dir", "", "Save full page to directory")
	cmd.Flags().Bool("with-comments", false, "Include comments in output")
	cmd.Flags().Bool("with-labels", false, "Include labels in output")

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

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()
			comments, err := apiClient.GetComments(ctx, pageID)
			if err != nil {
				return fmt.Errorf("failed to get comments: %w", err)
			}

			outputFormat := viper.GetString("output_format")
			return formatters.FormatOutput(comments, outputFormat)
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

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()
			labels, err := apiClient.GetLabels(ctx, pageID)
			if err != nil {
				return fmt.Errorf("failed to get labels: %w", err)
			}

			outputFormat := viper.GetString("output_format")
			return formatters.FormatOutput(labels, outputFormat)
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

			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

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

			var parentID *int
			if parent != 0 {
				parentID = &parent
			}

			pageUseCase := usecases.NewPageUseCase(apiClient)
			req := &usecases.CreatePageWithValidationRequest{
				SpaceKey: space,
				ParentID: parentID,
				Title:    title,
				Content:  content,
				Format:   format,
			}

			resp, err := pageUseCase.CreatePageWithValidation(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to create page: %w", err)
			}

			outputFormat := viper.GetString("output_format")
			return formatters.FormatOutput(resp.Page, outputFormat)
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

			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

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

			pageUseCase := usecases.NewPageUseCase(apiClient)
			req := &usecases.UpdatePageWithVersionRequest{
				PageID:         pageID,
				Content:        content,
				VersionComment: versionComment,
			}

			resp, err := pageUseCase.UpdatePageWithVersion(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to update page: %w", err)
			}

			outputFormat := viper.GetString("output_format")
			return formatters.FormatOutput(resp.Page, outputFormat)
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

			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()
			
			pageUseCase := usecases.NewPageUseCase(apiClient)
			req := &usecases.DeletePageWithConfirmationRequest{
				PageID:    pageID,
				Confirmed: confirm,
			}

			err = pageUseCase.DeletePageWithConfirmation(ctx, req)
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

			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			var parentCommentID *int
			if parentComment != 0 {
				parentCommentID = &parentComment
			}

			newComment, err := apiClient.AddComment(ctx, pageID, text, parentCommentID)
			if err != nil {
				return fmt.Errorf("failed to add comment: %w", err)
			}

			outputFormat := viper.GetString("output_format")
			return formatters.FormatOutput(newComment, outputFormat)
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

			if !confirm {
				return fmt.Errorf("--confirm flag required for write operations")
			}

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

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
