package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Profile represents a configuration profile
type Profile struct {
	URL            string `mapstructure:"url"`
	Token          string `mapstructure:"token"`
	Username       string `mapstructure:"username"`
	AuthType       string `mapstructure:"auth_type"`
	ImpersonateAs  string `mapstructure:"impersonate_as"`  // User to impersonate
	UseDomainAuth  bool   `mapstructure:"use_domain_auth"` // Use current domain user for authentication
	SessionCookie  string `mapstructure:"session_cookie"`  // Session cookie for browser-based auth
	SAMLAuthCookie string `mapstructure:"saml_auth_cookie"` // SAML auth cookie for identity provider
	ReadOnly       bool   `mapstructure:"read_only"`
}

// Config represents the main configuration
type Config struct {
	CurrentProfile string              `mapstructure:"current_profile"`
	Profiles       map[string]*Profile `mapstructure:"profiles"`
}

// DefaultProfileName is the name of the default profile
const DefaultProfileName = "default"

// LoadConfig loads the configuration from the config file
func LoadConfig() (*Config, error) {
	// Set defaults
	viper.SetDefault("current_profile", DefaultProfileName)
	viper.SetDefault("profiles.default.url", "")
	viper.SetDefault("profiles.default.token", "")
	viper.SetDefault("profiles.default.username", "")
	viper.SetDefault("profiles.default.auth_type", "bearer") // Default to bearer token
	viper.SetDefault("profiles.default.impersonate_as", "")  // No impersonation by default
	viper.SetDefault("profiles.default.use_domain_auth", false) // Don't use domain auth by default
	viper.SetDefault("profiles.default.session_cookie", "")   // No session cookie by default
	viper.SetDefault("profiles.default.saml_auth_cookie", "") // No SAML auth cookie by default
	viper.SetDefault("profiles.default.read_only", false)

	// Set config file path
	configDir, err := GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	viper.AddConfigPath(configDir)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Read the config file
	if err := viper.ReadInConfig(); err != nil {
		// If config file doesn't exist, create a default one
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			defaultConfig := &Config{
				CurrentProfile: DefaultProfileName,
				Profiles: map[string]*Profile{
					DefaultProfileName: {
						URL:           "",
						Token:         "",
						Username:      "",
						AuthType:      "bearer", // Default to bearer token
						ImpersonateAs: "",       // No impersonation by default
						UseDomainAuth: false,    // Don't use domain auth by default
						ReadOnly:      false,
					},
				},
			}

			if err := SaveConfig(defaultConfig); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}

			// Reload the newly created config
			if err := viper.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("failed to read default config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	// Unmarshal the config
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Ensure current profile exists
	if config.Profiles[config.CurrentProfile] == nil {
		// If current profile doesn't exist, fall back to default
		config.CurrentProfile = DefaultProfileName
		if config.Profiles[config.CurrentProfile] == nil {
			// If default doesn't exist either, create it
			config.Profiles[config.CurrentProfile] = &Profile{
				URL:            "",
				Token:          "",
				Username:       "",
				AuthType:       "bearer", // Default to bearer token
				ImpersonateAs:  "",       // No impersonation by default
				UseDomainAuth:  false,    // Don't use domain auth by default
				SessionCookie:  "",       // No session cookie by default
				SAMLAuthCookie: "",       // No SAML auth cookie by default
				ReadOnly:       false,
			}
		}
	}

	return &config, nil
}

// SaveConfig saves the configuration to the config file
func SaveConfig(config *Config) error {
	configDir, err := GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Set viper values
	viper.Set("current_profile", config.CurrentProfile)
	for name, profile := range config.Profiles {
		viper.Set(fmt.Sprintf("profiles.%s.url", name), profile.URL)
		viper.Set(fmt.Sprintf("profiles.%s.token", name), profile.Token)
		viper.Set(fmt.Sprintf("profiles.%s.username", name), profile.Username)
		viper.Set(fmt.Sprintf("profiles.%s.auth_type", name), profile.AuthType)
		viper.Set(fmt.Sprintf("profiles.%s.impersonate_as", name), profile.ImpersonateAs)
		viper.Set(fmt.Sprintf("profiles.%s.use_domain_auth", name), profile.UseDomainAuth)
		viper.Set(fmt.Sprintf("profiles.%s.session_cookie", name), profile.SessionCookie)
		viper.Set(fmt.Sprintf("profiles.%s.saml_auth_cookie", name), profile.SAMLAuthCookie)
		viper.Set(fmt.Sprintf("profiles.%s.read_only", name), profile.ReadOnly)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	if err := viper.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// GetConfigDir returns the configuration directory path
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".confcli"), nil
}

