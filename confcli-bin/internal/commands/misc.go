package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"confcli/pkg/confluence"
	"confcli/internal/formatter"
)

// NewSearchCmd creates the search command
func NewSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search for pages",
		Long:  "Search for Confluence pages using CQL or simple query",
		Args:  cobra.ArbitraryArgs, // Allow no arguments for CQL-only searches
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			
			cql, _ := cmd.Flags().GetString("cql")
			limit, _ := cmd.Flags().GetInt("limit")
			space, _ := cmd.Flags().GetString("space")

			// Create API client
			apiClient, err := confluence.NewClientFromViper()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			ctx := context.Background()

			// Build CQL query
			finalCQL := cql
			if finalCQL == "" {
				if query != "" {
					finalCQL = fmt.Sprintf("text ~ \"%s\"", query)
				} else {
					return fmt.Errorf("either a query or --cql must be provided")
				}
			}

			// Add space filter if specified
			if space != "" {
				finalCQL = fmt.Sprintf("space = \"%s\" AND (%s)", space, finalCQL)
			}

			// Perform search
			results, err := apiClient.Search(ctx, finalCQL, limit)
			if err != nil {
				return fmt.Errorf("failed to search: %w", err)
			}

			// Determine output format
			outputFormat := viper.GetString("output_format")

			// Format and output results
			return formatter.FormatOutput(results, outputFormat)
		},
	}

	cmd.Flags().String("cql", "", "CQL query")
	cmd.Flags().Int("limit", 25, "Result limit")
	cmd.Flags().String("space", "", "Space to search in")

	return cmd
}

// NewCompletionCmd creates the completion command
func NewCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for various shells.
To load completions:
Bash:
  $ source <(confcli completion bash)
  # To load for each session, execute once:
  # Linux: $ confcli completion bash > /etc/bash_completion.d/confcli
  # macOS: $ confcli completion bash > /usr/local/etc/bash_completion.d/confcli

Zsh:
  $ source <(confcli completion zsh)
  # To load for each session, execute once:
  $ confcli completion zsh > "${fpath[1]}/_confcli"

Fish:
  $ confcli completion fish | source
  # To load for each session, execute once:
  $ confcli completion fish > ~/.config/fish/completions/confcli.fish
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		Run: func(cmd *cobra.Command, args []string) {
			shell := args[0]
			switch shell {
			case "bash":
				cmd.Root().GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			}
		},
	}
	
	return cmd
}