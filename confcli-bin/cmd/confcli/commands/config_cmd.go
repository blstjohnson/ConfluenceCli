package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"confcli/pkg/config"
)

// NewConfigCmd creates the config command
func NewConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `Manage configuration for confcli`,
	}

	configCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize configuration file",
		Long:  `Initialize the configuration file with default settings`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Println("Configuration file initialized successfully")
			return nil
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "view",
		Short: "View current configuration",
		Long:  `View the current configuration settings`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			fmt.Printf("Current Profile: %s\n", cfg.CurrentProfile)
			fmt.Printf("Profiles:\n")
			for name, profile := range cfg.Profiles {
				activeMarker := ""
				if name == cfg.CurrentProfile {
					activeMarker = " *"
				}
				fmt.Printf("  %s%s:\n", name, activeMarker)
				fmt.Printf("    URL: %s\n", profile.URL)
				fmt.Printf("    Auth Type: %s\n", profile.AuthType)
				fmt.Printf("    Session Cookie: %s\n", hideSessionCookie(profile.SessionCookie))
				fmt.Printf("    SAML Auth Cookie: %s\n", hideSAMLAuthCookie(profile.SAMLAuthCookie))
				fmt.Printf("    Read Only: %t\n", profile.ReadOnly)

				// Don't show token in plain text for security
				if profile.Token != "" {
					fmt.Printf("    Token: ***HIDDEN***\n")
				} else {
					fmt.Printf("    Token: \n")
				}
			}
			return nil
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set configuration value",
		Long:  `Set a configuration value in the current profile`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			// Load current config
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Update the value in the current profile
			currentProfile := cfg.Profiles[cfg.CurrentProfile]
			switch key {
			case "url":
				currentProfile.URL = value
			case "token":
				currentProfile.Token = value
			case "username":
				currentProfile.Username = value
			case "auth_type":
				currentProfile.AuthType = value
			case "read_only":
				fmt.Sscanf(value, "%t", &currentProfile.ReadOnly)
			case "session_cookie":
				currentProfile.SessionCookie = value
			case "saml_auth_cookie":
				currentProfile.SAMLAuthCookie = value
			default:
				return fmt.Errorf("unknown configuration key: %s", key)
			}

			// Save the updated config
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Configuration %s set to %s\n", key, value)
			return nil
		},
	})

	return configCmd
}

// Helper function to hide session cookie value for security
func hideSessionCookie(cookie string) string {
	if cookie == "" {
		return ""
	}
	parts := strings.SplitN(cookie, "=", 2)
	if len(parts) < 2 {
		return "***HIDDEN***"
	}
	return fmt.Sprintf("%s=***HIDDEN***", parts[0])
}

// Helper function to hide SAML auth cookie value for security
func hideSAMLAuthCookie(cookie string) string {
	if cookie == "" {
		return ""
	}
	parts := strings.SplitN(cookie, "=", 2)
	if len(parts) < 2 {
		return "***HIDDEN***"
	}
	return fmt.Sprintf("%s=***HIDDEN***", parts[0])
}
