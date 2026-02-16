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

var cfgFile string

// NewRootCmd creates the root command
func NewRootCmd() *cobra.Command {
	var readOnly bool
	var format string
	var profile string
	var url string
	var token string
	var impersonateAs string
	var debug bool

	rootCmd := &cobra.Command{
		Use:   "confcli",
		Short: "A CLI tool for interacting with Atlassian Confluence",
		Long: `confcli is a command-line interface for Atlassian Confluence.
It allows you to retrieve, search, and manage Confluence pages.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Load configuration
			loadConfig(profile, url, token, impersonateAs, false) // Removed domain auth option

			// Set read-only mode if flag is set
			if readOnly {
				viper.Set("read_only", true)
			}

			// Set debug mode if flag is set
			if debug {
				viper.Set("debug", true)
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.confcli/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&readOnly, "read-only", false, "Enable read-only mode - prevents any modifying operations")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging - shows HTTP requests and responses")
	rootCmd.PersistentFlags().StringVar(&format, "format", "", "Output format: text, json, yaml (default: text)")
	rootCmd.PersistentFlags().StringVar(&profile, "profile", "", "Configuration profile to use")
	rootCmd.PersistentFlags().StringVar(&url, "url", "", "Confluence instance URL")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "Authentication token")
	rootCmd.PersistentFlags().StringVar(&impersonateAs, "impersonate-as", "", "Impersonate as user (requires admin privileges)")

	// Add commands
	rootCmd.AddCommand(NewPageCmd())
	rootCmd.AddCommand(NewHierarchyCmd())
	rootCmd.AddCommand(NewDescendantsCmd())
	rootCmd.AddCommand(NewSearchCmd())
	rootCmd.AddCommand(NewConfigCmd())
	rootCmd.AddCommand(NewCompletionCmd())
	rootCmd.AddCommand(NewLoginCmd())

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
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".confcli" (without extension).
		viper.AddConfigPath(filepath.Join(home, ".confcli"))
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

// loadConfig loads configuration with proper precedence: flag > env > config file > default
func loadConfig(profile, url, token, impersonateAs string, _ bool) {
	// Initialize config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Apply profile if specified via flag
	if profile != "" {
		cfg.CurrentProfile = profile
	}

	// Apply URL if specified via flag
	if url != "" {
		cfg.Profiles[cfg.CurrentProfile].URL = url
	}

	// Apply token if specified via flag
	if token != "" {
		cfg.Profiles[cfg.CurrentProfile].Token = token
	}

	// Apply impersonation if specified via flag
	if impersonateAs != "" {
		cfg.Profiles[cfg.CurrentProfile].ImpersonateAs = impersonateAs
	}

	// Set values in viper
	currentProfile := cfg.Profiles[cfg.CurrentProfile]
	viper.Set("url", currentProfile.URL)
	viper.Set("token", currentProfile.Token)
	viper.Set("username", currentProfile.Username)
	viper.Set("auth_type", currentProfile.AuthType)
	viper.Set("impersonate_as", currentProfile.ImpersonateAs)
	viper.Set("use_domain_auth", false) // Kept for backward compatibility but always false
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