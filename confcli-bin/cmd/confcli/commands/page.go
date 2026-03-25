package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"confcli/pkg/clients"
	"confcli/pkg/config"
	"confcli/pkg/converters"
	"confcli/pkg/formatters"
	"confcli/pkg/transforms"
	"confcli/pkg/usecases"
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
	pageCmd.AddCommand(newPageDiffCmd())

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
			output, _ := cmd.Flags().GetString("output")
			outputDir, _ := cmd.Flags().GetString("output-dir")
			withComments, _ := cmd.Flags().GetBool("with-comments")
			withLabels, _ := cmd.Flags().GetBool("with-labels")
			withDescendants, _ := cmd.Flags().GetBool("with-descendants")
			depth, _ := cmd.Flags().GetInt("depth")
			skipContent, _ := cmd.Flags().GetBool("skip-content")
			rewriteTFSLinks, _ := cmd.Flags().GetBool("rewrite-tfs-links")
			transformProfile, _ := cmd.Flags().GetString("transform")
			setOverrides, _ := cmd.Flags().GetStringArray("set")

			// Resolve transform profile if specified
			var profile *transforms.TransformProfile
			if transformProfile != "" {
				var err error
				profile, err = transforms.ResolveProfile(transformProfile)
				if err != nil {
					return err
				}
				if len(setOverrides) > 0 {
					overrideMap := make(map[string]string)
					for _, s := range setOverrides {
						parts := strings.SplitN(s, "=", 2)
						if len(parts) != 2 {
							return fmt.Errorf("invalid --set format %q: expected key=value", s)
						}
						overrideMap[parts[0]] = parts[1]
					}
					if err := transforms.ApplySetOverrides(profile, overrideMap); err != nil {
						return err
					}
				}
				if !cmd.Flags().Changed("format") && profile.Page.Format != "" {
					format = profile.Page.Format
				}
			}

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
				Version:      version,
				WithComments: withComments,
				WithLabels:   withLabels,
			}

			resp, err := pageUseCase.GetPageWithContent(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to get page: %w", err)
			}

			// Apply content transformation if needed
			// Confluence supports "storage", "editor", and "export_view" formats natively
			// For other formats, we fetch the appropriate base format and convert
			transformedContent := resp.Content
			if format != "storage" && format != "edit" && format != "export" {
				switch format {
				case "markdown":
					baseURL := viper.GetString("url")
					// Use advanced converter with support for Confluence macros
					transformedContent, err = converters.StorageToMarkdownAdvanced(resp.Content, baseURL)
					if err != nil {
						return fmt.Errorf("failed to convert storage to markdown: %w", err)
					}
				case "plain":
					transformedContent = utils.StripHTMLTags(resp.Content)
				case "html":
					// Storage format is already HTML-based, use as-is
					transformedContent = resp.Content
				}
			} else if format == "export" {
				// export_view is already clean HTML, convert to markdown
				baseURL := viper.GetString("url")
				transformedContent, err = converters.ExportViewToMarkdown(resp.Content, baseURL)
				if err != nil {
					return fmt.Errorf("failed to convert export_view to markdown: %w", err)
				}
			}

			// Apply transform pipeline if profile is set
			if profile != nil {
				pageCfg, _ := profile.ResolvePageConfig(id, "")
				if len(pageCfg.Transforms) > 0 {
					reg := transforms.DefaultRegistry()
					pipeline, pipeErr := transforms.BuildPipeline(pageCfg.Transforms, reg)
					if pipeErr != nil {
						return fmt.Errorf("failed to build transform pipeline: %w", pipeErr)
					}
					tctx := &transforms.TransformContext{
						PreContent:  resp.Content,
						PostContent: transformedContent,
						PageID:      id,
						PageTitle:   resp.Page.Title,
						Format:      format,
					}
					if err := pipeline.Run(tctx); err != nil {
						return fmt.Errorf("transform failed: %w", err)
					}
					transformedContent = tctx.PostContent
				}
			}

			// Apply TFS link rewriting if enabled
			if rewriteTFSLinks {
				tfsBaseURL := ""
				localRepoPath := ""
				if cfg, cfgErr := config.LoadConfig(); cfgErr == nil {
					profile := cfg.Profiles[cfg.CurrentProfile]
					if profile != nil {
						tfsBaseURL = profile.TFSBaseURL
						localRepoPath = profile.LocalRepoPath
					}
				}
				if tfsBaseURL != "" {
					// Determine output file path for relative path computation
					currentFilePath := ""
					if output != "" {
						if abs, absErr := filepath.Abs(output); absErr == nil {
							currentFilePath = abs
						}
					}
					transformedContent = converters.RewriteLinks(transformedContent, &converters.LinkRewriteConfig{
						TFSBaseURL:      tfsBaseURL,
						LocalRepoPath:   localRepoPath,
						CurrentFilePath: currentFilePath,
					})
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
						"space":    resp.Page.Space.ID,
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
					if withDescendants && id != 0 {
						hierReq := &usecases.GetPageHierarchyRequest{
							PageID: id,
							Depth:  depth,
						}
						hierResp, hierErr := pageUseCase.GetPageHierarchy(ctx, hierReq)
						if hierErr != nil {
							return fmt.Errorf("failed to get descendants: %w", hierErr)
						}

						descendants := make([]map[string]interface{}, 0, len(hierResp.Descendants))
						baseURL := viper.GetString("url")
						for _, desc := range hierResp.Descendants {
							descEntry := map[string]interface{}{
								"page_id":    desc.ID.String(),
								"title":      desc.Title,
								"version":    desc.Version.Number,
								"web_url":    desc.Links["webui"],
								"updated_at": desc.Version.UpdatedAt,
							}

							if !skipContent {
								descID, ok := desc.ID.Int()
								if ok {
									descContent, descErr := apiClient.GetPageContent(ctx, descID, "export_view", 0)
									if descErr == nil {
										md, mdErr := converters.ExportViewToMarkdown(descContent, baseURL)
										if mdErr == nil {
											descEntry["content"] = md
										}
									}
								}
							}

							descendants = append(descendants, descEntry)
						}
						data["descendants"] = descendants
					}
					return formatters.FormatOutput(data, "json")
				} else {
					// For text/yaml output, display page info with content
					return formatters.FormatOutputWithContent(resp.Page, transformedContent, format)
				}
			}

			return nil
		},
	}

	cmd.Flags().Int("id", 0, "Page ID")
	cmd.Flags().String("space", "", "Space key")
	cmd.Flags().String("title", "", "Page title")
	cmd.Flags().String("path", "", "Page path (e.g., Space/Parent/Child)")
	cmd.Flags().StringP("format", "f", "markdown", "Content format: markdown, storage, html, plain, edit, export")
	cmd.Flags().Int("version", 0, "Page version number (0 for current)")
	cmd.Flags().StringP("output", "o", "", "Save content to file")
	cmd.Flags().String("output-dir", "", "Save full page to directory")
	cmd.Flags().Bool("with-comments", false, "Include comments in output")
	cmd.Flags().Bool("with-labels", false, "Include labels in output")
	cmd.Flags().Bool("with-descendants", false, "Include descendant pages in output (requires JSON output format)")
	cmd.Flags().Int("depth", 0, "Maximum depth for descendant traversal (0 = unlimited, requires --with-descendants)")
	cmd.Flags().Bool("skip-content", false, "Omit page content from descendants (structure only, requires --with-descendants)")
	cmd.Flags().Bool("rewrite-tfs-links", false, "Rewrite TFS/Git repository links to local paths using config")
	cmd.Flags().String("transform", "", "Transform profile name or file path")
	cmd.Flags().StringArray("set", nil, "Override profile values (repeatable): key=value, e.g. --set page.strip_toc=true")

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
			} else {
				return fmt.Errorf("content file must be provided with --content-file")
			}

			pageUseCase := usecases.NewPageUseCase(apiClient)
			req := &usecases.UpdatePageWithVersionRequest{
				PageID:         pageID,
				Content:        content,
				VersionComment: versionComment,
				Format:         format,
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
	cmd.Flags().StringP("format", "f", "storage", "Content format: storage, edit (editor)")
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

// newPageDiffCmd implements the page diff command
func newPageDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <id>",
		Short: "Show diff between two versions of a page",
		Long:  "Show a git-diff-like output comparing two versions of a Confluence page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid page ID: %w", err)
			}

			oldVersion, _ := cmd.Flags().GetInt("old-version")
			newVersion, _ := cmd.Flags().GetInt("new-version")
			format, _ := cmd.Flags().GetString("format")
			color, _ := cmd.Flags().GetBool("color")
			summary, _ := cmd.Flags().GetBool("summary")

			if oldVersion <= 0 || newVersion <= 0 {
				return fmt.Errorf("both --old-version and --new-version must be specified with positive version numbers")
			}

			if oldVersion == newVersion {
				return fmt.Errorf("old-version and new-version must be different")
			}

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Get old version content - Confluence supports "storage", "editor", and "export_view" formats
			apiFormat := format
			switch format {
			case "edit":
				apiFormat = "editor"
			case "export", "export_view":
				apiFormat = "export_view"
			case "markdown", "md":
				apiFormat = "export_view"
			}
			oldStorageContent, err := apiClient.GetPageContent(ctx, pageID, apiFormat, oldVersion)
			if err != nil {
				return fmt.Errorf("failed to get old version content: %w", err)
			}

			// Get new version content
			newStorageContent, err := apiClient.GetPageContent(ctx, pageID, apiFormat, newVersion)
			if err != nil {
				return fmt.Errorf("failed to get new version content: %w", err)
			}

			// Convert from storage to requested format if needed
			oldContent, err := utils.ConvertContentFromStorage(oldStorageContent, format, viper.GetString("url"))
			if err != nil {
				return fmt.Errorf("failed to convert old version content: %w", err)
			}

			newContent, err := utils.ConvertContentFromStorage(newStorageContent, format, viper.GetString("url"))
			if err != nil {
				return fmt.Errorf("failed to convert new version content: %w", err)
			}

			// Generate diff
			diffOutput, err := generateDiff(pageID, oldVersion, newVersion, oldContent, newContent, color)
			if err != nil {
				return fmt.Errorf("failed to generate diff: %w", err)
			}

			outputFormat := viper.GetString("output_format")
			if outputFormat == "json" {
				data := map[string]interface{}{
					"page_id":     pageID,
					"old_version": oldVersion,
					"new_version": newVersion,
					"format":      format,
					"diff":        diffOutput,
				}
				if summary {
					data["changes_summary"] = summarizeChanges(oldContent, newContent)
				}
				return formatters.FormatOutput(data, "json")
			}

			fmt.Print(diffOutput)
			if summary {
				changeSummary := summarizeChanges(oldContent, newContent)
				summaryJSON, jsonErr := json.MarshalIndent(changeSummary, "", "  ")
				if jsonErr == nil {
					fmt.Printf("\nChanges Summary:\n%s\n", string(summaryJSON))
				}
			}
			return nil
		},
	}

	cmd.Flags().Int("old-version", 0, "Old version number to compare from (required)")
	cmd.Flags().Int("new-version", 0, "New version number to compare to (required)")
	cmd.Flags().StringP("format", "f", "storage", "Content format: storage, edit (editor), export")
	cmd.Flags().Bool("color", true, "Use colored output")
	cmd.Flags().Bool("summary", false, "Include change statistics (lines added/removed)")

	cmd.MarkFlagRequired("old-version")
	cmd.MarkFlagRequired("new-version")

	return cmd
}

// generateDiff generates a git-diff-like output between two versions
func generateDiff(pageID, oldVersion, newVersion int, oldContent, newContent string, color bool) (string, error) {
	// Create temporary files for diff
	tmpDir, err := os.MkdirTemp("", "confcli-diff-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	oldFile := filepath.Join(tmpDir, "old")
	newFile := filepath.Join(tmpDir, "new")

	if err := os.WriteFile(oldFile, []byte(oldContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write old content: %w", err)
	}
	if err := os.WriteFile(newFile, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write new content: %w", err)
	}

	// Run git diff --no-index
	var colorFlag string
	if color {
		colorFlag = "--color=always"
	} else {
		colorFlag = "--color=never"
	}

	diffCmd := exec.Command("git", "diff", "--no-index", colorFlag, "--", oldFile, newFile)
	output, err := diffCmd.Output()
	exitErr, ok := err.(*exec.ExitError)
	
	// git diff --no-index returns exit code 1 when files differ, which is expected
	if err != nil && (!ok || exitErr.ExitCode() != 1) {
		// If git is not available, fall back to simple diff
		return generateSimpleDiff(pageID, oldVersion, newVersion, oldContent, newContent, color), nil
	}

	// Replace file paths in diff output with version numbers
	diffOutput := string(output)
	diffOutput = strings.ReplaceAll(diffOutput, oldFile, fmt.Sprintf("version %d", oldVersion))
	diffOutput = strings.ReplaceAll(diffOutput, newFile, fmt.Sprintf("version %d", newVersion))

	return diffOutput, nil
}

// generateSimpleDiff generates a simple unified diff when git is not available
func generateSimpleDiff(pageID, oldVersion, newVersion int, oldContent, newContent string, color bool) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	var result strings.Builder

	// Header
	result.WriteString(fmt.Sprintf("diff --page %d --old-version %d --new-version %d\n", pageID, oldVersion, newVersion))
	result.WriteString(fmt.Sprintf("--- version %d\n", oldVersion))
	result.WriteString(fmt.Sprintf("+++ version %d\n", newVersion))

	// Simple line-by-line comparison
	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}

	for i := 0; i < maxLen; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine == newLine {
			// Unchanged line - skip for brevity
			continue
		}

		if oldLine != "" {
			if color {
				result.WriteString(fmt.Sprintf("\033[31m- %s\033[0m\n", oldLine))
			} else {
				result.WriteString(fmt.Sprintf("- %s\n", oldLine))
			}
		}
		if newLine != "" {
			if color {
				result.WriteString(fmt.Sprintf("\033[32m+ %s\033[0m\n", newLine))
			} else {
				result.WriteString(fmt.Sprintf("+ %s\n", newLine))
			}
		}
	}

	return result.String()
}

// summarizeChanges provides a summary of changes between two content strings
func summarizeChanges(oldContent, newContent string) map[string]interface{} {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	oldLineCount := len(oldLines)
	newLineCount := len(newLines)

	added := 0
	removed := 0

	oldSet := make(map[string]bool)
	newSet := make(map[string]bool)

	for _, line := range oldLines {
		oldSet[line] = true
	}
	for _, line := range newLines {
		newSet[line] = true
	}

	for line := range newSet {
		if !oldSet[line] {
			added++
		}
	}
	for line := range oldSet {
		if !newSet[line] {
			removed++
		}
	}

	return map[string]interface{}{
		"old_line_count": oldLineCount,
		"new_line_count": newLineCount,
		"lines_added":    added,
		"lines_removed":  removed,
		"net_change":     added - removed,
	}
}
