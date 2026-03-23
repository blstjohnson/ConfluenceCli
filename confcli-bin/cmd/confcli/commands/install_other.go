//go:build !windows

package commands

// addToWindowsUserPATH is a no-op on non-Windows platforms.
func addToWindowsUserPATH(dir string) error {
	return nil
}

// removeFromWindowsUserPATH is a no-op on non-Windows platforms.
func removeFromWindowsUserPATH(dir string) error {
	return nil
}

// addToWindowsUserPATHElevated is a no-op on non-Windows platforms.
func addToWindowsUserPATHElevated(dir string) error {
	return nil
}
