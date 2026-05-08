package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"confcli/pkg/transforms"
)

// NewTransformCmd creates the transform command group.
func NewTransformCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transform",
		Short: "Manage transform profiles",
		Long:  "List, show, and create transform profiles used for content transformation during export",
	}

	cmd.AddCommand(newTransformListCmd())
	cmd.AddCommand(newTransformShowCmd())
	cmd.AddCommand(newTransformInitCmd())

	return cmd
}

func newTransformListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available transform profiles",
		Long:  "List named transform profiles from ~/.confcli/transformations/",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}

			dir := filepath.Join(home, ".confcli", "transformations")
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No transform profiles found.")
					fmt.Printf("Create one with: confcli transform init <name>\n")
					return nil
				}
				return fmt.Errorf("failed to read transformations directory: %w", err)
			}

			found := false
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
					profileName := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
					fmt.Println(profileName)
					found = true
				}
			}

			if !found {
				fmt.Println("No transform profiles found.")
				fmt.Printf("Create one with: confcli transform init <name>\n")
			}

			return nil
		},
	}
}

func newTransformShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a transform profile",
		Long:  "Print the YAML contents of a named transform profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, err := transforms.ResolveProfile(args[0])
			if err != nil {
				return err
			}
			_ = profile // ResolveProfile validates it parses; re-read raw for display

			// Re-read raw file for display
			path, err := resolveProfilePath(args[0])
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read profile: %w", err)
			}
			fmt.Print(string(data))
			return nil
		},
	}
}

func newTransformInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Create a starter transform profile",
		Long:  "Create a starter YAML transform profile template in ~/.confcli/transformations/",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			name := args[0]

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}

			dir := filepath.Join(home, ".confcli", "transformations")
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create transformations directory: %w", err)
			}

			filePath := filepath.Join(dir, name+".yaml")
			if _, err := os.Stat(filePath); err == nil && !force {
				return fmt.Errorf("profile %q already exists at %s (use --force to overwrite)", name, filePath)
			}

			if err := os.WriteFile(filePath, []byte(starterTemplate), 0644); err != nil {
				return fmt.Errorf("failed to write profile: %w", err)
			}

			fmt.Printf("Created transform profile: %s\n", filePath)
			return nil
		},
	}

	cmd.Flags().Bool("force", false, "Overwrite existing profile")
	return cmd
}

// resolveProfilePath finds the file path for a named profile.
func resolveProfilePath(value string) (string, error) {
	if _, err := os.Stat(value); err == nil {
		return value, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(home, ".confcli", "transformations")
	candidates := []string{
		filepath.Join(dir, value+".yaml"),
		filepath.Join(dir, value+".yml"),
		filepath.Join(dir, value),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("transform profile %q not found", value)
}

const starterTemplate = `# Transform profile for confcli
# Use with: confcli hierarchy space --transform <name> ...
# Override values with: --set page.strip_toc=true

folder:
  naming: slug        # slug, title, or id
  length_limit: 80    # max folder name length (0 = no limit)
  flat_leaves: false   # save leaf pages directly in parent folder

page:
  format: markdown     # markdown, storage, html, plain, export
  strip_toc: false     # remove table of contents
  save_metadata: false # save .meta.json files

  # transforms:        # content transformation pipeline (uncomment to use)
  #   - type: remove_macro
  #     params:
  #       macro_names: [toc, status]
  #
  #   - type: remove_element
  #     params:
  #       selectors: [".confluence-information-macro"]
  #
  #   - type: modify_content
  #     params:
  #       phase: post  # pre (before conversion) or post (after conversion)
  #       rules:
  #         - find: "old-text"
  #           replace: "new-text"
  #
  #   - type: modify_links
  #     params:
  #       rules:
  #         - find: "https://old-domain.com"
  #           replace: "https://new-domain.com"

# pages:               # per-page overrides (uncomment to use)
#   - id: 12345
#     strip_toc: true
#   - path: "*/Archive/*"
#     skip: true
#   - id: 67890
#     flatten: true    # deep flatten: every descendant of this page (children,
#                      # grandchildren, ...) is saved directly in this page's
#                      # folder, with no nested directories. Internal links are
#                      # rewritten to the new flat paths automatically.
#                      # NOTE: differs from folder.flat_leaves, which only
#                      # flattens leaf pages globally.
`
