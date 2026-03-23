//go:build !windows

package commands

import "os"

// replaceBinary replaces the current binary with the new one.
// On Unix, we can simply rename over the existing binary.
func replaceBinary(currentPath, newPath string) error {
	return os.Rename(newPath, currentPath)
}

// cleanupOldBinary is a no-op on non-Windows platforms.
func CleanupOldBinary() {
}
