package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"confcli/pkg/config"
)

// NewRootCmd creates the root command
func NewRootCmd() *cobra.Command {
	var readOnly bool
	var format string
	var debug bool

	rootCmd := &cobra.Command{
		Use:   "confcli",
		Short: "A CLI tool for interacting with Atlassian Confluence",
		Long: `confcli is a command-line interface for Atlassian Confluence.
It allows you to retrieve, search, and manage Confluence pages.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Load configuration from default config file only
			loadConfig()

			// Set read-only mode if flag is set
			if readOnly {
				viper.Set("read_only", true)
			}

			// Set debug mode if flag is set
			if debug {
				viper.Set("debug", true)
				// Print config file location in debug mode
				fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
			}

			// Override format if specified
			if format != "" {
				viper.Set("output_format", format)
			} else {
				// Default to text format
				if !viper.IsSet("output_format") {
					viper.Set("output_format", "text")
				}
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().BoolVar(&readOnly, "read-only", false, "Enable read-only mode - prevents any modifying operations")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging - shows HTTP requests and responses")
	rootCmd.PersistentFlags().StringVar(&format, "format", "", "Output format: text, json, yaml (default: text)")

	// Add commands
	rootCmd.AddCommand(NewPageCmd())
	rootCmd.AddCommand(NewHierarchyCmd())
	rootCmd.AddCommand(NewDescendantsCmd())
	rootCmd.AddCommand(NewSearchCmd())
	rootCmd.AddCommand(NewConfigCmd())
	rootCmd.AddCommand(NewCompletionCmd())
	rootCmd.AddCommand(NewLoginCmd())
	rootCmd.AddCommand(NewAIAgentCmd())

	// Add help-json command
	rootCmd.AddCommand(&cobra.Command{
		Use:    "help-json",
		Hidden: true,
		Short:  "Output command structure in JSON format",
		Run: func(cmd *cobra.Command, args []string) {
			printJSONHelp(rootCmd)
		},
	})

	return rootCmd
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// Find home directory.
	home, err := os.UserHomeDir()
	cobra.CheckErr(err)

	// Search config in home directory with name ".confcli" (without extension).
	viper.AddConfigPath(filepath.Join(home, ".confcli"))
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		// Config file loaded successfully - message will be printed in PersistentPreRun if debug is enabled
	}
}

// loadConfig loads configuration from the default config file only
func loadConfig() {
	// Initialize config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Set values in viper from the default profile
	currentProfile := cfg.Profiles[cfg.CurrentProfile]
	viper.Set("url", currentProfile.URL)
	viper.Set("token", currentProfile.Token)
	viper.Set("username", currentProfile.Username)
	viper.Set("auth_type", currentProfile.AuthType)
	viper.Set("read_only", currentProfile.ReadOnly)

	// Set default output format if not already set
	if !viper.IsSet("output_format") {
		viper.Set("output_format", "text")
	}
}

// printJSONHelp prints the command structure in JSON format
func printJSONHelp(cmd *cobra.Command) {
	// Generate JSON representation of the command structure
	helpStruct := generateCommandHelp(cmd)
	
	// Output as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(helpStruct)
}

// generateCommandHelp generates a structure representing the command hierarchy
func generateCommandHelp(cmd *cobra.Command) map[string]interface{} {
	result := map[string]interface{}{
		"name":        cmd.Use,
		"description": cmd.Short,
		"long_desc":   cmd.Long,
		"usage":       cmd.UseLine(),
	}

	// Process flags
	flags := []map[string]interface{}{}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		flagInfo := map[string]interface{}{
			"name":        flag.Name,
			"shorthand":   flag.Shorthand,
			"usage":       flag.Usage,
			"default":     flag.DefValue,
			"value":       flag.Value.String(),
			"changed":     flag.Changed,
			"hidden":      flag.Hidden,
		}
		flags = append(flags, flagInfo)
	})

	if len(flags) > 0 {
		result["flags"] = flags
	}

	// Process persistent flags
	persistentFlags := []map[string]interface{}{}
	cmd.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		flagInfo := map[string]interface{}{
			"name":        flag.Name,
			"shorthand":   flag.Shorthand,
			"usage":       flag.Usage,
			"default":     flag.DefValue,
			"value":       flag.Value.String(),
			"changed":     flag.Changed,
			"hidden":      flag.Hidden,
		}
		persistentFlags = append(persistentFlags, flagInfo)
	})

	if len(persistentFlags) > 0 {
		result["persistent_flags"] = persistentFlags
	}

	// Process subcommands
	subcommands := []map[string]interface{}{}
	for _, subcmd := range cmd.Commands() {
		if !subcmd.Hidden {
			subcommands = append(subcommands, generateCommandHelp(subcmd))
		}
	}

	if len(subcommands) > 0 {
		result["subcommands"] = subcommands
	}

	return result
}