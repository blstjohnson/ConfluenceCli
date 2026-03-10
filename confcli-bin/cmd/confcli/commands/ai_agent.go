package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"confcli/pkg/clients"
	"confcli/pkg/converters"
	"confcli/pkg/usecases"
)

// NewAIAgentCmd creates the AI agent commands
func NewAIAgentCmd() *cobra.Command {
	aiAgentCmd := &cobra.Command{
		Use:   "ai-agent",
		Short: "Commands for AI agent integration",
		Long:  `Commands designed for AI agent integration including skills and slash commands`,
	}

	aiAgentCmd.AddCommand(newSlashCmd())
	aiAgentCmd.AddCommand(newSkillCmd())
	aiAgentCmd.AddCommand(newInitCmd())

	return aiAgentCmd
}

// newSlashCmd creates the slash command subcommand
func newSlashCmd() *cobra.Command {
	slashCmd := &cobra.Command{
		Use:   "slash",
		Short: "AI agent slash commands",
		Long:  `Slash commands for AI agents to interact with Confluence`,
	}

	slashCmd.AddCommand(newSlashGetPageCmd())
	slashCmd.AddCommand(newSlashGetPageDiffCmd())

	return slashCmd
}

// newSkillCmd creates the skill subcommand
func newSkillCmd() *cobra.Command {
	skillCmd := &cobra.Command{
		Use:   "skill",
		Short: "AI agent skills",
		Long:  `Skills for AI agents to perform complex operations in Confluence`,
	}

	skillCmd.AddCommand(newSkillEditPageCmd())

	return skillCmd
}

// newInitCmd initializes AI agent configuration
func newInitCmd() *cobra.Command {
	var agent string
	var outputDir string
	
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize AI agent configuration",
		Long:  "Initialize AI agent configuration and create skill/slash command files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputDir == "" {
				// Default to user's home directory for AI agent config
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				outputDir = filepath.Join(home, ".confcli", "ai-agents")
			}

			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			// Generate configuration files based on agent type
			switch agent {
			case "qwen", "qwen-code":
				if err := generateQwenConfig(outputDir); err != nil {
					return fmt.Errorf("failed to generate Qwen config: %w", err)
				}
			case "claude", "claude-code":
				if err := generateClaudeConfig(outputDir); err != nil {
					return fmt.Errorf("failed to generate Claude config: %w", err)
				}
			case "all":
				if err := generateQwenConfig(outputDir); err != nil {
					return fmt.Errorf("failed to generate Qwen config: %w", err)
				}
				if err := generateClaudeConfig(outputDir); err != nil {
					return fmt.Errorf("failed to generate Claude config: %w", err)
				}
			default:
				// Generate for all supported agents
				if err := generateQwenConfig(outputDir); err != nil {
					return fmt.Errorf("failed to generate Qwen config: %w", err)
				}
				if err := generateClaudeConfig(outputDir); err != nil {
					return fmt.Errorf("failed to generate Claude config: %w", err)
				}
			}

			fmt.Printf("AI agent configuration initialized in %s\n", outputDir)
			fmt.Println("\nAvailable slash commands:")
			fmt.Println("  /get-page <id>           - Get page by ID in export view (supports --with-descendants)")
			fmt.Println("  /get-page-diff <id> <v1> <v2> - Get diff between page versions")
			fmt.Println("\nAvailable skills:")
			fmt.Println("  edit-page <id> <changes> - Edit page with AI assistance")

			return nil
		},
	}
	
	cmd.Flags().StringVar(&agent, "agent", "all", "AI agent type: qwen, claude, or all")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for configuration files")
	
	return cmd
}

// newSlashGetPageCmd implements the /get-page slash command
func newSlashGetPageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-page <id>",
		Short: "Get page by ID in export view",
		Long:  "Retrieve a Confluence page by ID in export view format, optimized for AI agent consumption",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, _ := cmd.Flags().GetInt("id")
			withLabels, _ := cmd.Flags().GetBool("with-labels")
			withComments, _ := cmd.Flags().GetBool("with-comments")
			withDescendants, _ := cmd.Flags().GetBool("with-descendants")
			depth, _ := cmd.Flags().GetInt("depth")
			skipContent, _ := cmd.Flags().GetBool("skip-content")
			outputFile, _ := cmd.Flags().GetString("output")

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			pageUseCase := usecases.NewPageUseCase(apiClient)
			ctx := context.Background()

			// Get page with export view format
			req := &usecases.GetPageWithContentRequest{
				PageID:       pageID,
				Format:       "export",
				WithComments: withComments,
				WithLabels:   withLabels,
			}

			resp, err := pageUseCase.GetPageWithContent(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to get page: %w", err)
			}

			// Convert export view to clean markdown
			baseURL := viper.GetString("url")
			content, err := converters.ExportViewToMarkdown(resp.Content, baseURL)
			if err != nil {
				return fmt.Errorf("failed to convert export view to markdown: %w", err)
			}

			// Create output structure for AI agent
			output := map[string]interface{}{
				"page_id":      resp.Page.ID.String(),
				"title":        resp.Page.Title,
				"space":        resp.Page.Space.Key,
				"space_name":   resp.Page.Space.Name,
				"version":      resp.Page.Version.Number,
				"content":      content,
				"web_url":      resp.Page.Links["webui"],
				"edit_url":     resp.Page.Links["editui"],
				"updated_at":   resp.Page.Version.UpdatedAt,
			}

			if withLabels {
				output["labels"] = resp.Labels
			}
			if withComments {
				output["comments"] = resp.Comments
			}

			// Fetch descendants if requested
			if withDescendants {
				hierReq := &usecases.GetPageHierarchyRequest{
					PageID: pageID,
					Depth:  depth,
				}
				hierResp, err := pageUseCase.GetPageHierarchy(ctx, hierReq)
				if err != nil {
					return fmt.Errorf("failed to get descendants: %w", err)
				}

				descendants := make([]map[string]interface{}, 0, len(hierResp.Descendants))
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
							descContent, err := apiClient.GetPageContent(ctx, descID, "export_view", 0)
							if err == nil {
								md, err := converters.ExportViewToMarkdown(descContent, baseURL)
								if err == nil {
									descEntry["content"] = md
								}
							}
						}
					}

					descendants = append(descendants, descEntry)
				}
				output["descendants"] = descendants
			}

			// Output as JSON for AI agent consumption
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")

			if outputFile != "" {
				file, err := os.Create(outputFile)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer file.Close()
				encoder = json.NewEncoder(file)
				encoder.SetIndent("", "  ")
			}

			if err := encoder.Encode(output); err != nil {
				return fmt.Errorf("failed to encode output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().Int("id", 0, "Page ID")
	cmd.Flags().Bool("with-labels", false, "Include labels in output")
	cmd.Flags().Bool("with-comments", false, "Include comments in output")
	cmd.Flags().Bool("with-descendants", false, "Include descendant pages in output")
	cmd.Flags().Int("depth", 0, "Maximum depth for descendant traversal (0 = unlimited, requires --with-descendants)")
	cmd.Flags().Bool("skip-content", false, "Omit page content from descendants (structure only, requires --with-descendants)")
	cmd.Flags().String("output", "", "Save output to file")

	return cmd
}

// newSlashGetPageDiffCmd implements the /get-page-diff slash command
func newSlashGetPageDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-page-diff <page-id>",
		Short: "Get diff between two versions of a page",
		Long:  "Show differences between two versions of a Confluence page, optimized for AI agent review",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, _ := cmd.Flags().GetInt("page-id")
			oldVersion, _ := cmd.Flags().GetInt("old-version")
			newVersion, _ := cmd.Flags().GetInt("new-version")
			format, _ := cmd.Flags().GetString("format")
			outputFile, _ := cmd.Flags().GetString("output")

			if pageID <= 0 {
				return fmt.Errorf("invalid page ID: must be positive")
			}

			if oldVersion <= 0 || newVersion <= 0 {
				return fmt.Errorf("version numbers must be positive")
			}

			if oldVersion >= newVersion {
				return fmt.Errorf("old-version must be less than new-version")
			}

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Get old version content in export view
			oldStorageContent, err := apiClient.GetPageContent(ctx, pageID, "export_view", oldVersion)
			if err != nil {
				return fmt.Errorf("failed to get old version content: %w", err)
			}

			// Get new version content in export view
			newStorageContent, err := apiClient.GetPageContent(ctx, pageID, "export_view", newVersion)
			if err != nil {
				return fmt.Errorf("failed to get new version content: %w", err)
			}

			// Convert to markdown for cleaner diff
			baseURL := viper.GetString("url")
			oldContent, err := converters.ExportViewToMarkdown(oldStorageContent, baseURL)
			if err != nil {
				return fmt.Errorf("failed to convert old version: %w", err)
			}

			newContent, err := converters.ExportViewToMarkdown(newStorageContent, baseURL)
			if err != nil {
				return fmt.Errorf("failed to convert new version: %w", err)
			}

			// Generate unified diff
			diffOutput, err := generateUnifiedDiff(pageID, oldVersion, newVersion, oldContent, newContent)
			if err != nil {
				return fmt.Errorf("failed to generate diff: %w", err)
			}

			// Create structured output for AI agent
			output := map[string]interface{}{
				"page_id":       pageID,
				"old_version":   oldVersion,
				"new_version":   newVersion,
				"format":        format,
				"diff":          diffOutput,
				"changes_summary": summarizeChanges(oldContent, newContent),
			}

			// Output as JSON for AI agent consumption
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")

			if outputFile != "" {
				file, err := os.Create(outputFile)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer file.Close()
				encoder = json.NewEncoder(file)
				encoder.SetIndent("", "  ")
			}

			if err := encoder.Encode(output); err != nil {
				return fmt.Errorf("failed to encode output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().Int("page-id", 0, "Page ID")
	cmd.Flags().Int("old-version", 0, "Old version number to compare from (required)")
	cmd.Flags().Int("new-version", 0, "New version number to compare to (required)")
	cmd.Flags().String("format", "unified", "Output format (default: unified)")
	cmd.Flags().String("output", "", "Save diff to file")

	cmd.MarkFlagRequired("page-id")
	cmd.MarkFlagRequired("old-version")
	cmd.MarkFlagRequired("new-version")

	return cmd
}

// newSkillEditPageCmd implements the edit-page skill
func newSkillEditPageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit-page <id>",
		Short: "Edit a page with AI assistance",
		Long: `Edit a Confluence page with AI assistance.
This skill:
1. Retrieves the page in edit view format
2. Saves it to a temporary file
3. Allows AI to modify the content
4. Updates the page with the modified content
5. Cleans up the temporary file`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pageID, _ := cmd.Flags().GetInt("id")
			contentFile, _ := cmd.Flags().GetString("content-file")
			versionComment, _ := cmd.Flags().GetString("version-comment")
			tempDir, _ := cmd.Flags().GetString("temp-dir")
			keepTemp, _ := cmd.Flags().GetBool("keep-temp")
			confirm, _ := cmd.Flags().GetBool("confirm")

			if !confirm && contentFile == "" {
				return fmt.Errorf("--confirm flag required for write operations, or provide --content-file")
			}

			apiClient, err := clients.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()
			pageUseCase := usecases.NewPageUseCase(apiClient)

			// Step 1: Get page in edit view format
			fmt.Fprintf(os.Stderr, "Fetching page %d in edit view...\n", pageID)
			req := &usecases.GetPageWithContentRequest{
				PageID: pageID,
				Format: "edit",
			}

			resp, err := pageUseCase.GetPageWithContent(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to get page: %w", err)
			}

			// Step 2: Save to temporary file
			if tempDir == "" {
				tempDir, err = os.MkdirTemp("", "confcli-edit-*")
				if err != nil {
					return fmt.Errorf("failed to create temp directory: %w", err)
				}
				if !keepTemp {
					defer os.RemoveAll(tempDir)
				}
			}

			tempFile := filepath.Join(tempDir, fmt.Sprintf("page_%d.md", pageID))

			if contentFile == "" {
				// Save current content to temp file for AI to edit
				if err := os.WriteFile(tempFile, []byte(resp.Content), 0644); err != nil {
					return fmt.Errorf("failed to write temp file: %w", err)
				}
				fmt.Fprintf(os.Stderr, "Page content saved to: %s\n", tempFile)
				fmt.Fprintf(os.Stderr, "AI agent should modify this file and then run the command again with --content-file\n")

				// Output info for AI agent
				output := map[string]interface{}{
					"status":       "temp_file_created",
					"page_id":      pageID,
					"title":        resp.Page.Title,
					"version":      resp.Page.Version.Number,
					"temp_file":    tempFile,
					"content":      resp.Content,
					"next_step":    "Modify the content and run: confcli ai-agent skill edit-page <id> --content-file <modified_file> --version-comment '<comment>' --confirm",
				}

				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(output); err != nil {
					return fmt.Errorf("failed to encode output: %w", err)
				}

				return nil
			}

			// Step 3: Read modified content
			fmt.Fprintf(os.Stderr, "Reading modified content from: %s\n", contentFile)
			modifiedContent, err := os.ReadFile(contentFile)
			if err != nil {
				return fmt.Errorf("failed to read modified content: %w", err)
			}

			// Step 4: Update the page
			fmt.Fprintf(os.Stderr, "Updating page %d...\n", pageID)
			updateReq := &usecases.UpdatePageWithVersionRequest{
				PageID:         pageID,
				Content:        string(modifiedContent),
				VersionComment: versionComment,
				Format:         "edit",
			}

			updateResp, err := pageUseCase.UpdatePageWithVersion(ctx, updateReq)
			if err != nil {
				return fmt.Errorf("failed to update page: %w", err)
			}

			// Step 5: Clean up temp file if not keeping
			if !keepTemp {
				if err := os.Remove(tempFile); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to remove temp file: %v\n", err)
				}
			}

			// Output result for AI agent
			output := map[string]interface{}{
				"status":            "page_updated",
				"page_id":           pageID,
				"title":             updateResp.Page.Title,
				"old_version":       resp.Page.Version.Number,
				"new_version":       updateResp.Page.Version.Number,
				"version_comment":   versionComment,
				"web_url":           updateResp.Page.Links["webui"],
				"edit_url":          updateResp.Page.Links["editui"],
				"temp_file_removed": !keepTemp,
			}

			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(output); err != nil {
				return fmt.Errorf("failed to encode output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().Int("id", 0, "Page ID")
	cmd.Flags().String("content-file", "", "Path to modified content file")
	cmd.Flags().String("version-comment", "", "Comment for the version update")
	cmd.Flags().String("temp-dir", "", "Custom temp directory")
	cmd.Flags().Bool("keep-temp", false, "Keep temp file after update")
	cmd.Flags().Bool("confirm", false, "Confirm the update operation")

	return cmd
}

// generateQwenConfig generates configuration files for Qwen Code
func generateQwenConfig(outputDir string) error {
	// Create skills directory
	skillsDir := filepath.Join(outputDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return err
	}

	// Create commands directory
	commandsDir := filepath.Join(outputDir, "commands")
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	// Generate get-page-by-id skill
	getPageSkill := `---
name: get-page-by-id
description: Get a Confluence page by ID in export view format
argument-hint: <page-id> [--with-labels] [--with-comments] [--with-descendants] [--depth N] [--skip-content] [--output <file>]
---

## Purpose

Retrieves a Confluence page by its ID and returns the content in export view format (clean markdown) with JSON output optimized for AI agent consumption.

## When to Use

- When you need to read a specific Confluence page by its ID
- When you need page content in a clean, readable markdown format
- When you need structured JSON output with metadata (space, version, URLs)
- When you want to include labels and/or comments with the page content

## Command

` + "```bash" + `
confcli ai-agent slash get-page --id <page-id> [flags]
` + "```" + `

## Arguments

- ` + "`<page-id>`" + ` - The Confluence page ID (required, use ` + "`--id`" + ` flag)

## Flags

- ` + "`--id <int>`" + ` - Page ID to retrieve (required)
- ` + "`--with-labels`" + ` - Include page labels in the output
- ` + "`--with-comments`" + ` - Include page comments in the output
- ` + "`--with-descendants`" + ` - Include descendant pages with content converted to markdown
- ` + "`--depth <int>`" + ` - Maximum depth for descendant traversal (0 = unlimited, requires --with-descendants)
- ` + "`--skip-content`" + ` - Omit page content from descendants (structure only, requires --with-descendants)
- ` + "`--output <string>`" + ` - Save JSON output to a file instead of stdout

## Output Format

The command returns JSON with the following structure:

` + "```json" + `
{
  "page_id": "123456",
  "title": "Page Title",
  "space": "DEV",
  "space_name": "Development",
  "version": 5,
  "content": "# Page Content in Markdown...",
  "web_url": "/spaces/DEV/pages/123456",
  "edit_url": "/pages/edit.action?pageId=123456",
  "updated_at": "2024-01-01T00:00:00Z"
}
` + "```" + `

When ` + "`--with-labels`" + ` is specified, a ` + "`labels`" + ` array is included.
When ` + "`--with-comments`" + ` is specified, a ` + "`comments`" + ` array is included.
When ` + "`--with-descendants`" + ` is specified, a ` + "`descendants`" + ` array is included with each entry containing page_id, title, version, web_url, updated_at, and content (unless --skip-content is used).

## Examples

` + "```bash" + `
# Basic page retrieval
confcli ai-agent slash get-page --id 123456

# Get page with labels and comments
confcli ai-agent slash get-page --id 123456 --with-labels --with-comments

# Get page with all descendant pages and their content
confcli ai-agent slash get-page --id 123456 --with-descendants

# Get page tree structure only (no content for descendants), max 2 levels deep
confcli ai-agent slash get-page --id 123456 --with-descendants --depth 2 --skip-content

# Save output to file
confcli ai-agent slash get-page --id 123456 --output page.json
` + "```" + `

## Related Skills

- edit-page - Edit a page with AI assistance
- get-page-diff - Compare two versions of a page
`
	getPageSkillDir := filepath.Join(skillsDir, "get-page-by-id")
	if err := os.MkdirAll(getPageSkillDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(getPageSkillDir, "get-page-by-id.md"), []byte(getPageSkill), 0644); err != nil {
		return err
	}

	// Generate get-page-diff skill
	getPageDiffSkill := `---
name: get-page-diff
description: Get diff between two versions of a Confluence page
argument-hint: <page-id> --old-version <v1> --new-version <v2> [--output <file>]
---

## Purpose

Shows the differences between two versions of a Confluence page using unified diff format. Returns structured JSON output with the diff and change statistics.

## When to Use

- When you need to compare two versions of a Confluence page
- When reviewing changes made between versions
- When you need to understand what content was added or removed
- When generating change reports or audit trails

## Command

` + "```bash" + `
confcli ai-agent slash get-page-diff --page-id <page-id> --old-version <v1> --new-version <v2> [flags]
` + "```" + `

## Arguments

- ` + "`<page-id>`" + ` - The Confluence page ID (required, use ` + "`--page-id`" + ` flag)

## Flags

- ` + "`--page-id <int>`" + ` - Page ID to compare (required)
- ` + "`--old-version <int>`" + ` - Older version number to compare from (required)
- ` + "`--new-version <int>`" + ` - Newer version number to compare to (required)
- ` + "`--format <string>`" + ` - Output format (default: unified)
- ` + "`--output <string>`" + ` - Save JSON output to a file instead of stdout

## Output Format

The command returns JSON with the following structure:

` + "```json" + `
{
  "page_id": 123456,
  "old_version": 1,
  "new_version": 2,
  "format": "unified",
  "diff": "--- version 1\n+++ version 2\n...",
  "changes_summary": {
    "old_line_count": 100,
    "new_line_count": 105,
    "lines_added": 10,
    "lines_removed": 5,
    "net_change": 5
  }
}
` + "```" + `

## Examples

` + "```bash" + `
# Compare versions 1 and 2
confcli ai-agent slash get-page-diff --page-id 123456 --old-version 1 --new-version 2

# Save diff to file
confcli ai-agent slash get-page-diff --page-id 123456 --old-version 1 --new-version 2 --output diff.json
` + "```" + `

## Related Skills

- get-page-by-id - Get page content and metadata
- edit-page - Edit a page with AI assistance
`
	getPageDiffSkillDir := filepath.Join(skillsDir, "get-page-diff")
	if err := os.MkdirAll(getPageDiffSkillDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(getPageDiffSkillDir, "get-page-diff.md"), []byte(getPageDiffSkill), 0644); err != nil {
		return err
	}

	// Generate edit-page skill
	editPageSkill := `---
name: edit-page
description: Edit a Confluence page with AI assistance using a two-step workflow
argument-hint: <page-id> [--content-file <file>] [--version-comment <text>] [--confirm]
---

## Purpose

Edits a Confluence page with AI assistance. This skill uses a two-step workflow:
1. First call retrieves the page in edit view format and saves it to a temporary file
2. Second call updates the page with the AI-modified content

## When to Use

- When you need to modify the content of an existing Confluence page
- When you want AI to help rewrite, update, or improve page content
- When you need to preserve Confluence editor formatting
- When you want a safe, two-step edit process with explicit confirmation

## Two-Step Workflow

### Step 1: Retrieve Page for Editing

` + "```bash" + `
confcli ai-agent skill edit-page --id <page-id>
` + "```" + `

This will:
1. Fetch the page in edit view format from Confluence
2. Save the content to a temporary file
3. Output JSON with the temp file location and next steps

Output:
` + "```json" + `
{
  "status": "temp_file_created",
  "page_id": 123456,
  "title": "Page Title",
  "version": 5,
  "temp_file": "/tmp/confcli-edit-abc123/page_123456.md",
  "content": "...",
  "next_step": "Modify the content and run: confcli ai-agent skill edit-page <id> --content-file <modified_file> --version-comment '<comment>' --confirm"
}
` + "```" + `

### Step 2: Update Page with Modified Content

After the AI agent modifies the content in the temp file:

` + "```bash" + `
confcli ai-agent skill edit-page --id <page-id> --content-file <modified-file> --version-comment "<comment>" --confirm
` + "```" + `

This will:
1. Read the modified content from the specified file
2. Update the Confluence page with the new content
3. Clean up the temporary file (unless ` + "`--keep-temp`" + ` is specified)
4. Output JSON with the update result

Output:
` + "```json" + `
{
  "status": "page_updated",
  "page_id": 123456,
  "title": "Page Title",
  "old_version": 5,
  "new_version": 6,
  "version_comment": "Updated by AI",
  "web_url": "/spaces/DEV/pages/123456",
  "edit_url": "/pages/edit.action?pageId=123456",
  "temp_file_removed": true
}
` + "```" + `

## Flags

- ` + "`--id <int>`" + ` - Page ID to edit (required)
- ` + "`--content-file <string>`" + ` - Path to the modified content file (required for step 2)
- ` + "`--version-comment <string>`" + ` - Comment describing the changes (recommended for step 2)
- ` + "`--temp-dir <string>`" + ` - Custom temporary directory for the edit file
- ` + "`--keep-temp`" + ` - Keep the temporary file after update (default: false)
- ` + "`--confirm`" + ` - Confirm the update operation (required for step 2)

## Examples

` + "```bash" + `
# Step 1: Get the page for editing
confcli ai-agent skill edit-page --id 123456

# AI agent modifies the temp file content...

# Step 2: Update the page with changes
confcli ai-agent skill edit-page --id 123456 --content-file /tmp/confcli-edit-abc123/page_123456.md --version-comment "Improved documentation" --confirm
` + "```" + `

## Important Notes

1. **Two-Step Process**: The skill requires two separate invocations - one to get the content, one to update
2. **Confirmation Required**: The ` + "`--confirm`" + ` flag is required for the update step to prevent accidental changes
3. **Edit View Format**: Content is retrieved in Confluence editor format to preserve formatting
4. **Temp File Cleanup**: Temporary files are automatically removed after successful update unless ` + "`--keep-temp`" + ` is specified
5. **Version Tracking**: Each update increments the page version number

## Related Skills

- get-page-by-id - Get page content in export view
- get-page-diff - Compare page versions
`
	editPageSkillDir := filepath.Join(skillsDir, "edit-page")
	if err := os.MkdirAll(editPageSkillDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(editPageSkillDir, "edit-page.md"), []byte(editPageSkill), 0644); err != nil {
		return err
	}

	// Generate get-page command
	getPageCmd := `---
description: Get a Confluence page by ID in export view format
argument-hint: <page-id> [--with-labels] [--with-comments] [--with-descendants] [--depth N] [--skip-content] [--output <file>]
---

## Workflow

1. Call the command with the page ID
2. Parse the JSON output to get page content and metadata
3. Use the content for analysis or as context for other operations

## Command

` + "```bash" + `
confcli ai-agent slash get-page --id <page-id> [--with-labels] [--with-comments] [--with-descendants] [--depth N] [--skip-content] [--output <file>]
` + "```" + `

## Examples

` + "```bash" + `
# Basic usage
confcli ai-agent slash get-page --id 123456

# With labels and comments
confcli ai-agent slash get-page --id 123456 --with-labels --with-comments

# With descendant pages
confcli ai-agent slash get-page --id 123456 --with-descendants --depth 2

# Structure only (no descendant content)
confcli ai-agent slash get-page --id 123456 --with-descendants --skip-content
` + "```" + `

## Output

Returns JSON with: page_id, title, space, space_name, version, content, web_url, edit_url, updated_at
When --with-descendants is used, includes a descendants array with page_id, title, version, web_url, updated_at, and content (unless --skip-content)
`
	getPageCmdDir := filepath.Join(commandsDir, "get-page")
	if err := os.MkdirAll(getPageCmdDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(getPageCmdDir, "get-page.md"), []byte(getPageCmd), 0644); err != nil {
		return err
	}

	// Generate get-page-diff command
	getPageDiffCmd := `---
description: Get diff between two versions of a Confluence page
argument-hint: <page-id> --old-version <v1> --new-version <v2> [--output <file>]
---

## Workflow

1. Use get-page to find the current version number
2. Call get-page-diff with old and new version numbers
3. Analyze the diff output to understand changes

## Command

` + "```bash" + `
confcli ai-agent slash get-page-diff --page-id <page-id> --old-version <v1> --new-version <v2> [--output <file>]
` + "```" + `

## Examples

` + "```bash" + `
# Compare versions 1 and 2
confcli ai-agent slash get-page-diff --page-id 123456 --old-version 1 --new-version 2
` + "```" + `

## Output

Returns JSON with: page_id, old_version, new_version, format, diff, changes_summary
`
	getPageDiffCmdDir := filepath.Join(commandsDir, "get-page-diff")
	if err := os.MkdirAll(getPageDiffCmdDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(getPageDiffCmdDir, "get-page-diff.md"), []byte(getPageDiffCmd), 0644); err != nil {
		return err
	}

	return nil
}

// generateClaudeConfig generates configuration files for Claude Code
func generateClaudeConfig(outputDir string) error {
	// Claude uses the same format as Qwen - just call the same function
	return generateQwenConfig(outputDir)
}

// generateUnifiedDiff generates a unified diff between two strings
func generateUnifiedDiff(pageID, oldVersion, newVersion int, oldContent, newContent string) (string, error) {
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

	// Run git diff --no-index with unified format
	diffCmd := exec.Command("git", "diff", "--no-index", "-U3", "--", oldFile, newFile)
	output, err := diffCmd.Output()
	exitErr, ok := err.(*exec.ExitError)

	// git diff --no-index returns exit code 1 when files differ, which is expected
	if err != nil && (!ok || exitErr.ExitCode() != 1) {
		// If git is not available, fall back to simple diff
		return generateSimpleDiffStr(pageID, oldVersion, newVersion, oldContent, newContent), nil
	}

	// Replace file paths in diff output with version numbers
	diffOutput := string(output)
	diffOutput = strings.ReplaceAll(diffOutput, oldFile, fmt.Sprintf("version %d", oldVersion))
	diffOutput = strings.ReplaceAll(diffOutput, newFile, fmt.Sprintf("version %d", newVersion))

	return diffOutput, nil
}

// generateSimpleDiffStr generates a simple unified diff when git is not available
func generateSimpleDiffStr(pageID, oldVersion, newVersion int, oldContent, newContent string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	var result strings.Builder

	// Header
	result.WriteString(fmt.Sprintf("--- version %d\n", oldVersion))
	result.WriteString(fmt.Sprintf("+++ version %d\n", newVersion))

	// Simple line-by-line comparison (unified diff style)
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
			continue
		}

		if oldLine != "" {
			result.WriteString(fmt.Sprintf("- %s\n", oldLine))
		}
		if newLine != "" {
			result.WriteString(fmt.Sprintf("+ %s\n", newLine))
		}
	}

	return result.String()
}

// summarizeChanges provides a simple summary of changes between two versions
func summarizeChanges(oldContent, newContent string) map[string]interface{} {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	oldLineCount := len(oldLines)
	newLineCount := len(newLines)

	added := 0
	removed := 0

	// Simple line comparison
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
