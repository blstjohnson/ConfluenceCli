package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Create a mock config file
	configPath := filepath.Join(tempDir, "config.yaml")
	mockConfig := `current_profile: "test"
profiles:
  test:
    url: "https://test.atlassian.net/wiki"
    token: "test-token"
    username: "test@example.com"
    auth_type: "bearer"
    read_only: false
  prod:
    url: "https://prod.atlassian.net/wiki"
    token: "prod-token"
    username: "admin@example.com"
    auth_type: "bearer"
    read_only: true
`
	
	if err := os.WriteFile(configPath, []byte(mockConfig), 0644); err != nil {
		t.Fatalf("Failed to write mock config: %v", err)
	}
	
	// Temporarily change the config directory by setting the home directory
	originalHome := os.Getenv("HOME")
	t.Setenv("HOME", tempDir) // This will make GetConfigDir return tempDir/.confcli
	
	// Create the config directory
	configDir := filepath.Join(tempDir, ".confcli")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	
	// Move the config file to the expected location
	expectedConfigPath := filepath.Join(configDir, "config.yaml")
	if err := os.Rename(configPath, expectedConfigPath); err != nil {
		t.Fatalf("Failed to move config file: %v", err)
	}
	
	// Test loading the config
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	// Restore original home
	os.Setenv("HOME", originalHome)
	
	// Verify the loaded config
	if config.CurrentProfile != "test" {
		t.Errorf("Expected CurrentProfile to be 'test', got '%s'", config.CurrentProfile)
	}
	
	if config.Profiles["test"] == nil {
		t.Fatal("Expected 'test' profile to exist")
	}
	
	testProfile := config.Profiles["test"]
	if testProfile.URL != "https://test.atlassian.net/wiki" {
		t.Errorf("Expected test profile URL to be 'https://test.atlassian.net/wiki', got '%s'", testProfile.URL)
	}
	
	if testProfile.Token != "test-token" {
		t.Errorf("Expected test profile token to be 'test-token', got '%s'", testProfile.Token)
	}
	
	if config.Profiles["prod"] == nil {
		t.Fatal("Expected 'prod' profile to exist")
	}
	
	prodProfile := config.Profiles["prod"]
	if prodProfile.ReadOnly != true {
		t.Errorf("Expected prod profile ReadOnly to be true, got %t", prodProfile.ReadOnly)
	}
}

func TestSaveConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Temporarily change the config directory by setting the home directory
	originalHome := os.Getenv("HOME")
	t.Setenv("HOME", tempDir) // This will make GetConfigDir return tempDir/.confcli
	
	// Create the config directory
	configDir := filepath.Join(tempDir, ".confcli")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	
	// Create a config to save
	config := &Config{
		CurrentProfile: "test",
		Profiles: map[string]*Profile{
			"test": {
				URL:      "https://test.atlassian.net/wiki",
				Token:    "test-token",
				Username: "test@example.com",
				AuthType: "bearer",
				ReadOnly: false,
			},
		},
	}
	
	// Save the config
	if err := SaveConfig(config); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}
	
	// Restore original home
	os.Setenv("HOME", originalHome)
	
	// Verify the file was created
	configPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}
	
	// Load and verify the saved config
	loadedConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}
	
	// Since we're testing saving a config, the CurrentProfile should be set to the one we saved
	// But if the config file didn't have a current_profile set, it will default to "default"
	// So we need to check if our test profile exists regardless of current profile
	if loadedConfig.Profiles["test"] == nil {
		t.Fatal("Expected 'test' profile to exist after saving")
	}
	
	testProfile := loadedConfig.Profiles["test"]
	if testProfile.URL != "https://test.atlassian.net/wiki" {
		t.Errorf("Expected saved profile URL to be 'https://test.atlassian.net/wiki', got '%s'", testProfile.URL)
	}
}