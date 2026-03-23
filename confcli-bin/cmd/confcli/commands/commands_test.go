package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"confcli/pkg/formatters"
)

func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()
	
	if cmd == nil {
		t.Fatal("NewRootCmd returned nil")
	}
	
	if cmd.Use != "confcli" {
		t.Errorf("Expected root command to use 'confcli', got '%s'", cmd.Use)
	}
	
	// Check that subcommands exist
	subcommands := cmd.Commands()
	if len(subcommands) == 0 {
		t.Error("Expected root command to have subcommands")
	}
	
	// Verify some key subcommands exist
	expectedSubcommands := []string{"page", "hierarchy", "config"}
	for _, expected := range expectedSubcommands {
		found := false
		for _, subcmd := range subcommands {
			if subcmd.Use == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find subcommand '%s'", expected)
		}
	}
}

func TestNewPageCmd(t *testing.T) {
	cmd := NewPageCmd()
	
	if cmd == nil {
		t.Fatal("NewPageCmd returned nil")
	}
	
	if cmd.Use != "page" {
		t.Errorf("Expected page command to use 'page', got '%s'", cmd.Use)
	}
	
	// Check that page subcommands exist
	subcommands := cmd.Commands()
	if len(subcommands) == 0 {
		t.Error("Expected page command to have subcommands")
	}
}

func TestNewHierarchyCmd(t *testing.T) {
	cmd := NewHierarchyCmd()
	
	if cmd == nil {
		t.Fatal("NewHierarchyCmd returned nil")
	}
	
	if cmd.Use != "hierarchy" {
		t.Errorf("Expected hierarchy command to use 'hierarchy', got '%s'", cmd.Use)
	}
	
	// Check that hierarchy subcommands exist
	subcommands := cmd.Commands()
	if len(subcommands) == 0 {
		t.Error("Expected hierarchy command to have subcommands")
	}
	
	// Verify the space subcommand exists
	foundSpace := false
	for _, subcmd := range subcommands {
		if subcmd.Use == "space" {
			foundSpace = true
			break
		}
	}
	if !foundSpace {
		t.Error("Expected to find hierarchy space subcommand")
	}
}

func TestNewConfigCmd(t *testing.T) {
	cmd := NewConfigCmd()
	
	if cmd == nil {
		t.Fatal("NewConfigCmd returned nil")
	}
	
	if cmd.Use != "config" {
		t.Errorf("Expected config command to use 'config', got '%s'", cmd.Use)
	}
	
	// Check that config subcommands exist
	subcommands := cmd.Commands()
	if len(subcommands) == 0 {
		t.Error("Expected config command to have subcommands")
	}
}

func TestNewSearchCmd(t *testing.T) {
	cmd := NewSearchCmd()
	
	if cmd == nil {
		t.Fatal("NewSearchCmd returned nil")
	}
	
	if cmd.Use != "search [query]" {
		t.Errorf("Expected search command to use 'search [query]', got '%s'", cmd.Use)
	}
}

func TestNewCompletionCmd(t *testing.T) {
	cmd := NewCompletionCmd()
	
	if cmd == nil {
		t.Fatal("NewCompletionCmd returned nil")
	}
	
	if cmd.Use != "completion [bash|zsh|fish|powershell]" {
		t.Errorf("Expected completion command to use 'completion [bash|zsh|fish|powershell]', got '%s'", cmd.Use)
	}
}

func TestPageGetCmdExecution(t *testing.T) {
	// Create a temporary config directory for testing
	tempDir := t.TempDir()
	originalConfigDir := viper.ConfigFileUsed()
	
	// Set up a fake config file
	configPath := filepath.Join(tempDir, "config.yaml")
	configContent := `current_profile: "default"
profiles:
  default:
    url: "https://example.atlassian.net/wiki"
    token: "fake-token"
    username: "test@example.com"
    auth_type: "bearer"
    cache_ttl: 5
    read_only: false
`
	
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	
	// Set viper to use our test config
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("Failed to read test config: %v", err)
	}
	
	// Create the command
	cmd := newPageGetCmd()
	
	// Capture output
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	
	// Set up the command with test arguments
	cmd.SetArgs([]string{"--help"})
	
	// Execute the command
	err := cmd.Execute()
	if err != nil {
		t.Errorf("Error executing page get command: %v", err)
	}
	
	// Restore original config
	viper.SetConfigFile(originalConfigDir)
	
	output := buf.String()
	if len(output) == 0 {
		t.Error("Expected help output from page get command")
	}
}

func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	_, output, err = executeCommandC(root, args...)
	return output, err
}

func executeCommandC(root *cobra.Command, args ...string) (c *cobra.Command, output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	
	c, err = root.ExecuteC()
	output = buf.String()
	return c, output, err
}

// Test that the help-json command works
func TestHelpJsonCommand(t *testing.T) {
	rootCmd := NewRootCmd()
	
	// Execute the help-json command
	_, err := executeCommand(rootCmd, "help-json")
	if err != nil {
		t.Errorf("Error executing help-json command: %v", err)
	}
}

func TestNewInstallCmd(t *testing.T) {
	cmd := NewInstallCmd()

	if cmd == nil {
		t.Fatal("NewInstallCmd returned nil")
	}

	if cmd.Use != "install" {
		t.Errorf("Expected install command to use 'install', got '%s'", cmd.Use)
	}

	// Check that uninstall subcommand exists
	subcommands := cmd.Commands()
	foundUninstall := false
	for _, subcmd := range subcommands {
		if subcmd.Use == "uninstall" {
			foundUninstall = true
			break
		}
	}
	if !foundUninstall {
		t.Error("Expected to find install uninstall subcommand")
	}

	// Verify flags exist
	flags := []string{"dir", "copy", "force"}
	for _, flagName := range flags {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("Expected install command to have --%s flag", flagName)
		}
	}
}

func TestInstallHelp(t *testing.T) {
	cmd := NewInstallCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("Error executing install --help: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("Expected help output from install command")
	}
}

func TestNewUpdateCmd(t *testing.T) {
	cmd := NewUpdateCmd()

	if cmd == nil {
		t.Fatal("NewUpdateCmd returned nil")
	}

	if cmd.Use != "update" {
		t.Errorf("Expected update command to use 'update', got '%s'", cmd.Use)
	}

	// Verify flags exist
	flags := []string{"check", "force", "pre-release"}
	for _, flagName := range flags {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Errorf("Expected update command to have --%s flag", flagName)
		}
	}
}

func TestUpdateHelp(t *testing.T) {
	cmd := NewUpdateCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Errorf("Error executing update --help: %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("Expected help output from update command")
	}
}

func TestExpectedAssetName(t *testing.T) {
	name, err := expectedAssetName()
	if err != nil {
		// On unsupported platforms, we expect an error
		t.Skipf("Unsupported platform: %v", err)
	}
	if name == "" {
		t.Error("Expected non-empty asset name")
	}
}

func TestUpdateRegistered(t *testing.T) {
	rootCmd := NewRootCmd()

	found := false
	for _, subcmd := range rootCmd.Commands() {
		if subcmd.Use == "update" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find update subcommand in root")
	}
}

// Additional tests to verify the functionality of the commands
func TestMockClient(t *testing.T) {
	// This is a mock test to verify that the formatter package can be imported
	// In a real test, we would create a mock client with test server
	_ = context.Background()
	_ = formatters.FormatOutput
}