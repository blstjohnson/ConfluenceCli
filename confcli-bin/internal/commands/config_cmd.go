package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"confcli/internal/config"
)

// NewConfigCmd creates the config command
func NewConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration profiles",
		Long:  `Manage configuration profiles for confcli`,
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
				fmt.Printf("    Cache TTL: %d minutes\n", profile.CacheTTL)
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
			case "cache_ttl":
				fmt.Sscanf(value, "%d", &currentProfile.CacheTTL)
			case "read_only":
				fmt.Sscanf(value, "%t", &currentProfile.ReadOnly)
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

	configCmd.AddCommand(&cobra.Command{
		Use:   "use-profile [profile-name]",
		Short: "Switch to a profile",
		Long:  `Switch to a different configuration profile`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName := args[0]

			// Load current config
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Check if profile exists
			if cfg.Profiles[profileName] == nil {
				return fmt.Errorf("profile %s does not exist", profileName)
			}

			// Switch to the profile
			cfg.CurrentProfile = profileName

			// Save the updated config
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			// Also update viper for current session
			viper.Set("current_profile", profileName)

			fmt.Printf("Switched to profile: %s\n", profileName)
			return nil
		},
	})

	return configCmd
}