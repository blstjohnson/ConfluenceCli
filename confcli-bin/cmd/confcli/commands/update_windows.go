//go:build windows

package commands

import (
	"fmt"
	"os"
)

// replaceBinary replaces the current binary with the new one.
// On Windows, a running binary is locked, so we rename the old binary
// to .old first, then copy the new one into place.
func replaceBinary(currentPath, newPath string) error {
	oldPath := currentPath + ".old"

	// Clean up any leftover .old file from a previous update
	os.Remove(oldPath)

	// Rename current (locked) binary out of the way
	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("cannot rename current binary: %w", err)
	}

	// Copy new binary into place
	data, err := os.ReadFile(newPath)
	if err != nil {
		// Try to restore old binary
		os.Rename(oldPath, currentPath)
		return fmt.Errorf("cannot read new binary: %w", err)
	}

	if err := os.WriteFile(currentPath, data, 0755); err != nil {
		// Try to restore old binary
		os.Rename(oldPath, currentPath)
		return fmt.Errorf("cannot write new binary: %w", err)
	}

	return nil
}

// cleanupOldBinary removes the .old binary left from a previous update.
func CleanupOldBinary() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	os.Remove(exe + ".old")
}
