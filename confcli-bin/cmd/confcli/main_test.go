package main

import (
	"bytes"
	"testing"

	"confcli/internal/commands"
)

func TestRootCmd(t *testing.T) {
	rootCmd := commands.NewRootCmd()
	
	// Test that the command exists and has the right name
	if rootCmd.Use != "confcli" {
		t.Errorf("Expected command name to be 'confcli', got '%s'", rootCmd.Use)
	}
	
	// Test that it has some of the main subcommands
	subcommands := rootCmd.Commands()
	if len(subcommands) == 0 {
		t.Error("Expected to find subcommands, but none were present")
	}
	
	// Verify a few key commands exist
	hasPage := false
	hasConfig := false
	for _, cmd := range subcommands {
		if cmd.Use == "page" {
			hasPage = true
		}
		if cmd.Use == "config" {
			hasConfig = true
		}
	}
	
	if !hasPage {
		t.Error("Expected to find 'page' subcommand")
	}
	
	if !hasConfig {
		t.Error("Expected to find 'config' subcommand")
	}
}

func TestHelpOutput(t *testing.T) {
	rootCmd := commands.NewRootCmd()
	
	// Capture output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--help"})
	
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("Error executing help command: %v", err)
	}
	
	// Check that output contains expected elements
	output := buf.String()
	if len(output) == 0 {
		t.Error("Expected help output, but got empty string")
	}
	
	if !contains(output, "confcli") {
		t.Error("Expected help output to contain 'confcli'")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || len(substr) == 0 ||
		   (len(s) > len(substr) && 
		   (s[:len(substr)] == substr || 
		   s[len(s)-len(substr):] == substr ||
		   containsHelper(s, substr))))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}