package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewAIAgentCmd creates the AI agent commands
func NewAIAgentCmd() *cobra.Command {
	aiAgentCmd := &cobra.Command{
		Use:   "ai-agent",
		Short: "Commands for AI agent integration",
		Long: `Commands designed for AI agent integration.

The ai-agent init command generates configuration files and MCP tool definitions
for AI agent integration. For page operations, use the regular page commands
with --output-format=json for machine-readable output:

  confcli page get --id 123 --output-format json
  confcli page get --id 123 --with-descendants --output-format json
  confcli page diff 123 --old-version 1 --new-version 2 --summary --output-format json
  confcli page update 123 --content-file modified.md --confirm`,
	}

	aiAgentCmd.AddCommand(newInitCmd())

	return aiAgentCmd
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

			// Walk the root command tree to generate config for all commands
			rootCmd := cmd.Root()

			if err := generateAgentConfig(rootCmd, outputDir, agent); err != nil {
				return fmt.Errorf("failed to generate agent config: %w", err)
			}

			// Generate MCP tool definitions if requested
			if mcpOutput != "" {
				if err := generateMCPTools(rootCmd, mcpOutput); err != nil {
					return fmt.Errorf("failed to generate MCP tools: %w", err)
				}
				fmt.Printf("MCP tool definitions written to %s\n", mcpOutput)
			}

			fmt.Printf("AI agent configuration initialized in %s\n", outputDir)

			return nil
		},
	}

	cmd.Flags().StringVar(&agent, "agent", "all", "AI agent type: qwen, claude, or all")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory for configuration files")
	cmd.Flags().StringVar(&mcpOutput, "mcp", "", "Generate MCP tool definitions to file")

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
	FullPath    string // e.g. "confcli page get"
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

// generateAgentConfig walks the command tree and generates skill/command files.
func generateAgentConfig(rootCmd *cobra.Command, outputDir, agent string) error {
	skillsDir := filepath.Join(outputDir, "skills")
	commandsDir := filepath.Join(outputDir, "commands")

	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	// Walk all top-level commands and their subcommands
	for _, topCmd := range rootCmd.Commands() {
		if topCmd.Hidden || topCmd.Name() == "help" || topCmd.Name() == "completion" {
			continue
		}

		subCommands := topCmd.Commands()
		if len(subCommands) == 0 {
			// Top-level command with no subcommands
			meta := extractCommandMeta(topCmd, "command", rootCmd.Commands())
			data := newTemplateData(meta)

			content, err := renderTemplate(commandTemplate, data)
			if err != nil {
				return fmt.Errorf("rendering command %s: %w", topCmd.Name(), err)
			}
			dir := filepath.Join(commandsDir, topCmd.Name())
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, topCmd.Name()+".md"), []byte(content), 0644); err != nil {
				return err
			}
			continue
		}

		// Command with subcommands — generate for each leaf
		for _, sub := range subCommands {
			if sub.Hidden {
				continue
			}

			// Determine if this is a write operation (skill) or read (command)
			category := "command"
			if sub.Name() == "create" || sub.Name() == "update" || sub.Name() == "delete" ||
				strings.HasPrefix(sub.Name(), "comment") || strings.HasPrefix(sub.Name(), "label") {
				category = "skill"
			}

			meta := extractCommandMeta(sub, category, subCommands)
			data := newTemplateData(meta)

			tmpl := commandTemplate
			if category == "skill" {
				tmpl = skillTemplate
			}

			content, err := renderTemplate(tmpl, data)
			if err != nil {
				return fmt.Errorf("rendering %s %s: %w", category, sub.Name(), err)
			}

			targetDir := commandsDir
			if category == "skill" {
				targetDir = skillsDir
			}

			dir := filepath.Join(targetDir, fmt.Sprintf("%s-%s", topCmd.Name(), sub.Name()))
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			filename := fmt.Sprintf("%s-%s.md", topCmd.Name(), sub.Name())
			if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
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
func generateMCPTools(rootCmd *cobra.Command, outputPath string) error {
	var tools []mcpTool

	for _, topCmd := range rootCmd.Commands() {
		if topCmd.Hidden || topCmd.Name() == "help" || topCmd.Name() == "completion" {
			continue
		}

		subCommands := topCmd.Commands()
		if len(subCommands) == 0 {
			// Top-level command
			tool := buildMCPTool(topCmd, topCmd.Name())
			tools = append(tools, tool)
			continue
		}

		for _, sub := range subCommands {
			if sub.Hidden {
				continue
			}
			toolName := fmt.Sprintf("confcli_%s_%s", topCmd.Name(), strings.ReplaceAll(sub.Name(), "-", "_"))
			tool := buildMCPTool(sub, toolName)
			tools = append(tools, tool)
		}
	}

	data, err := json.MarshalIndent(map[string]interface{}{"tools": tools}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}

// buildMCPTool creates an MCP tool definition from a cobra command.
func buildMCPTool(cmd *cobra.Command, name string) mcpTool {
	properties := map[string]interface{}{}
	var required []string

	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
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

	return mcpTool{
		Name:        name,
		Description: cmd.Short,
		InputSchema: schema,
	}
}
