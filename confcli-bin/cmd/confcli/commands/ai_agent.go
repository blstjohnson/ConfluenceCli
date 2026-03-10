package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	var mcpOutput string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize AI agent configuration",
		Long:  "Initialize AI agent configuration and create skill/slash command files by walking the cobra command tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputDir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				outputDir = filepath.Join(home, ".confcli", "ai-agents")
			}

			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}

			// Find the ai-agent command to walk its subtrees
			aiAgentCmd := cmd.Parent()

			if err := generateAgentConfig(aiAgentCmd, outputDir, agent); err != nil {
				return fmt.Errorf("failed to generate agent config: %w", err)
			}

			// Generate MCP tool definitions if requested
			if mcpOutput != "" {
				if err := generateMCPTools(aiAgentCmd, mcpOutput); err != nil {
					return fmt.Errorf("failed to generate MCP tools: %w", err)
				}
				fmt.Printf("MCP tool definitions written to %s\n", mcpOutput)
			}

			fmt.Printf("AI agent configuration initialized in %s\n", outputDir)

			// List generated commands dynamically
			slashCmd, _, _ := aiAgentCmd.Find([]string{"slash"})
			skillCmd, _, _ := aiAgentCmd.Find([]string{"skill"})

			if slashCmd != nil {
				fmt.Println("\nAvailable slash commands:")
				for _, sub := range slashCmd.Commands() {
					if !sub.Hidden {
						fmt.Printf("  /%s - %s\n", sub.Name(), sub.Short)
					}
				}
			}
			if skillCmd != nil {
				fmt.Println("\nAvailable skills:")
				for _, sub := range skillCmd.Commands() {
					if !sub.Hidden {
						fmt.Printf("  %s - %s\n", sub.Name(), sub.Short)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&agent, "agent", "all", "AI agent type: qwen, claude, or all")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for configuration files")
	cmd.Flags().StringVar(&mcpOutput, "mcp", "", "Generate MCP tool definitions to file")

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

// commandMeta holds extracted metadata from a cobra.Command for template rendering.
type commandMeta struct {
	Name        string
	Description string
	LongDesc    string
	Use         string
	UseLine     string
	Category    string // "slash" or "skill"
	FullPath    string // e.g. "confcli ai-agent slash get-page"
	Flags       []flagMeta
	Siblings    []siblingMeta
}

// flagMeta holds extracted metadata for a single flag.
type flagMeta struct {
	Name      string
	Shorthand string
	Usage     string
	Default   string
	Type      string
	Required  bool
	Hidden    bool
	Enum      []string
}

// siblingMeta holds minimal info about related commands.
type siblingMeta struct {
	Name        string
	Description string
}

// extractCommandMeta extracts metadata from a cobra.Command.
func extractCommandMeta(cmd *cobra.Command, category string, siblings []*cobra.Command) commandMeta {
	meta := commandMeta{
		Name:        cmd.Name(),
		Description: cmd.Short,
		LongDesc:    cmd.Long,
		Use:         cmd.Use,
		UseLine:     cmd.UseLine(),
		Category:    category,
		FullPath:    cmd.CommandPath(),
	}

	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		// Strip trailing "(default: ...)" and "(required)" from usage to avoid
		// duplication with the template's own annotations.
		usage := flag.Usage
		if idx := strings.Index(usage, "(default:"); idx > 0 {
			usage = strings.TrimSpace(usage[:idx])
		}
		if idx := strings.Index(usage, "(required)"); idx > 0 {
			usage = strings.TrimSpace(usage[:idx])
		}
		fm := flagMeta{
			Name:      flag.Name,
			Shorthand: flag.Shorthand,
			Usage:     usage,
			Default:   flag.DefValue,
			Type:      flag.Value.Type(),
			Required:  isFlagRequired(flag),
			Hidden:    flag.Hidden,
			Enum:      parseEnumValues(flag.Usage),
		}
		meta.Flags = append(meta.Flags, fm)
	})

	for _, sib := range siblings {
		if sib.Name() != cmd.Name() && !sib.Hidden {
			meta.Siblings = append(meta.Siblings, siblingMeta{
				Name:        sib.Name(),
				Description: sib.Short,
			})
		}
	}

	return meta
}

// buildArgumentHint generates a usage hint from command metadata.
func buildArgumentHint(meta commandMeta) string {
	var parts []string
	// Use the args portion of the Use field
	if idx := strings.IndexByte(meta.Use, ' '); idx >= 0 {
		parts = append(parts, meta.Use[idx+1:])
	}
	for _, f := range meta.Flags {
		if f.Required {
			parts = append(parts, fmt.Sprintf("--%s <%s>", f.Name, f.Type))
		}
	}
	var optional []string
	for _, f := range meta.Flags {
		if !f.Required {
			if f.Type == "bool" {
				optional = append(optional, fmt.Sprintf("[--%s]", f.Name))
			} else {
				optional = append(optional, fmt.Sprintf("[--%s <%s>]", f.Name, f.Type))
			}
		}
	}
	parts = append(parts, optional...)
	return strings.Join(parts, " ")
}

// templateFuncs provides helper functions for templates.
var templateFuncs = template.FuncMap{
	"joinEnum": func(vals []string) string { return strings.Join(vals, ", ") },
}

// Skill markdown template with YAML front matter.
var skillTemplate = template.Must(template.New("skill").Funcs(templateFuncs).Parse(`---
name: {{ .Name }}
description: {{ .Description }}
argument-hint: {{ .ArgumentHint }}
---

## Purpose

{{ .LongDesc }}

## Command

` + "```" + `bash
{{ .FullPath }}{{ range .Flags }}{{ if .Required }} --{{ .Name }} <{{ .Type }}>{{ end }}{{ end }} [flags]
` + "```" + `

## Flags
{{ range .Flags }}
- ` + "`" + `--{{ .Name }}{{ if .Shorthand }}, -{{ .Shorthand }}{{ end }} <{{ .Type }}>` + "`" + ` — {{ .Usage }}{{ if .Required }} **(required)**{{ end }}{{ if ne .Default "" }}{{ if ne .Default "false" }}{{ if ne .Default "0" }} (default: {{ .Default }}){{ end }}{{ end }}{{ end }}{{ if .Enum }} (values: {{ joinEnum .Enum }}){{ end }}
{{- end }}

## Examples

` + "```" + `bash
# Basic usage
{{ .FullPath }}{{ range .Flags }}{{ if .Required }} --{{ .Name }} <{{ .Type }}>{{ end }}{{ end }}
{{ if .HasOptionalFlags }}
# With optional flags
{{ .FullPath }}{{ range .Flags }}{{ if .Required }} --{{ .Name }} <{{ .Type }}>{{ end }}{{ end }}{{ range .Flags }}{{ if not .Required }}{{ if eq .Type "bool" }} --{{ .Name }}{{ end }}{{ end }}{{ end }}
{{ end }}` + "```" + `
{{ if .Siblings }}
## Related

{{ range .Siblings }}- {{ .Name }} — {{ .Description }}
{{ end }}{{ end }}`))

// Command markdown template (simpler, for slash commands).
var commandTemplate = template.Must(template.New("command").Funcs(templateFuncs).Parse(`---
description: {{ .Description }}
argument-hint: {{ .ArgumentHint }}
---

## Command

` + "```" + `bash
{{ .FullPath }}{{ range .Flags }}{{ if .Required }} --{{ .Name }} <{{ .Type }}>{{ end }}{{ end }} [flags]
` + "```" + `

## Flags
{{ range .Flags }}
- ` + "`" + `--{{ .Name }}{{ if .Shorthand }}, -{{ .Shorthand }}{{ end }} <{{ .Type }}>` + "`" + ` — {{ .Usage }}{{ if .Required }} **(required)**{{ end }}{{ if ne .Default "" }}{{ if ne .Default "false" }}{{ if ne .Default "0" }} (default: {{ .Default }}){{ end }}{{ end }}{{ end }}
{{- end }}

## Examples

` + "```" + `bash
{{ .FullPath }}{{ range .Flags }}{{ if .Required }} --{{ .Name }} <{{ .Type }}>{{ end }}{{ end }}
` + "```" + `
`))

// templateData extends commandMeta with template-specific computed fields.
type templateData struct {
	commandMeta
	ArgumentHint     string
	HasOptionalFlags bool
}

func newTemplateData(meta commandMeta) templateData {
	hasOptional := false
	for _, f := range meta.Flags {
		if !f.Required {
			hasOptional = true
			break
		}
	}
	return templateData{
		commandMeta:      meta,
		ArgumentHint:     buildArgumentHint(meta),
		HasOptionalFlags: hasOptional,
	}
}

// renderTemplate renders a template with the given data and returns the result.
func renderTemplate(tmpl *template.Template, data templateData) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}
	return buf.String(), nil
}

// generateAgentConfig walks the ai-agent command tree and generates skill/command files.
func generateAgentConfig(aiAgentCmd *cobra.Command, outputDir, agent string) error {
	skillsDir := filepath.Join(outputDir, "skills")
	commandsDir := filepath.Join(outputDir, "commands")

	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	// Walk the "slash" subtree — generates both skills and commands
	slashCmd, _, _ := aiAgentCmd.Find([]string{"slash"})
	if slashCmd != nil {
		for _, sub := range slashCmd.Commands() {
			if sub.Hidden {
				continue
			}
			meta := extractCommandMeta(sub, "slash", slashCmd.Commands())
			data := newTemplateData(meta)

			// Generate skill file
			content, err := renderTemplate(skillTemplate, data)
			if err != nil {
				return fmt.Errorf("rendering skill %s: %w", sub.Name(), err)
			}
			dir := filepath.Join(skillsDir, sub.Name())
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, sub.Name()+".md"), []byte(content), 0644); err != nil {
				return err
			}

			// Generate command file
			content, err = renderTemplate(commandTemplate, data)
			if err != nil {
				return fmt.Errorf("rendering command %s: %w", sub.Name(), err)
			}
			dir = filepath.Join(commandsDir, sub.Name())
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, sub.Name()+".md"), []byte(content), 0644); err != nil {
				return err
			}
		}
	}

	// Walk the "skill" subtree — generates skill files only
	skillCmdGroup, _, _ := aiAgentCmd.Find([]string{"skill"})
	if skillCmdGroup != nil {
		// Collect all sibling commands across both subtrees for cross-referencing
		var allSiblings []*cobra.Command
		if slashCmd != nil {
			allSiblings = append(allSiblings, slashCmd.Commands()...)
		}
		allSiblings = append(allSiblings, skillCmdGroup.Commands()...)

		for _, sub := range skillCmdGroup.Commands() {
			if sub.Hidden {
				continue
			}
			meta := extractCommandMeta(sub, "skill", allSiblings)
			data := newTemplateData(meta)

			content, err := renderTemplate(skillTemplate, data)
			if err != nil {
				return fmt.Errorf("rendering skill %s: %w", sub.Name(), err)
			}
			dir := filepath.Join(skillsDir, sub.Name())
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, sub.Name()+".md"), []byte(content), 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

// mcpTool represents an MCP tool definition.
type mcpTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// generateMCPTools generates MCP tool definitions from the cobra command tree.
func generateMCPTools(aiAgentCmd *cobra.Command, outputPath string) error {
	var tools []mcpTool

	// Walk slash and skill subtrees
	for _, groupName := range []string{"slash", "skill"} {
		groupCmd, _, _ := aiAgentCmd.Find([]string{groupName})
		if groupCmd == nil {
			continue
		}
		for _, sub := range groupCmd.Commands() {
			if sub.Hidden {
				continue
			}

			properties := map[string]interface{}{}
			var required []string

			sub.Flags().VisitAll(func(flag *pflag.Flag) {
				if flag.Hidden {
					return
				}
				prop := map[string]interface{}{
					"description": flag.Usage,
				}
				switch flag.Value.Type() {
				case "int", "int32", "int64":
					prop["type"] = "integer"
				case "bool":
					prop["type"] = "boolean"
				case "float32", "float64":
					prop["type"] = "number"
				default:
					prop["type"] = "string"
				}
				if flag.DefValue != "" && flag.DefValue != "0" && flag.DefValue != "false" {
					prop["default"] = flag.DefValue
				}
				if enums := parseEnumValues(flag.Usage); len(enums) > 0 {
					prop["enum"] = enums
				}
				properties[flag.Name] = prop
				if isFlagRequired(flag) {
					required = append(required, flag.Name)
				}
			})

			schema := map[string]interface{}{
				"type":       "object",
				"properties": properties,
			}
			if len(required) > 0 {
				schema["required"] = required
			}

			tools = append(tools, mcpTool{
				Name:        fmt.Sprintf("confcli_%s_%s", groupName, strings.ReplaceAll(sub.Name(), "-", "_")),
				Description: sub.Short,
				InputSchema: schema,
			})
		}
	}

	data, err := json.MarshalIndent(map[string]interface{}{"tools": tools}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
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
