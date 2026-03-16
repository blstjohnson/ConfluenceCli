//go:build windows

package commands

import (
	"fmt"
	"os/exec"
	"strings"
)

// addToWindowsUserPATH adds a directory to the user's persistent PATH
// environment variable via the Windows registry. It reads only the user PATH
// (not system PATH) to avoid corrupting the system-level value.
func addToWindowsUserPATH(dir string) error {
	current, err := getUserPATH()
	if err != nil {
		return err
	}

	// Check if directory is already in PATH (case-insensitive on Windows)
	dirLower := strings.ToLower(dir)
	for _, entry := range strings.Split(current, ";") {
		if strings.ToLower(strings.TrimSpace(entry)) == dirLower {
			return nil // already present
		}
	}

	// Append to PATH
	newPath := current
	if newPath != "" && !strings.HasSuffix(newPath, ";") {
		newPath += ";"
	}
	newPath += dir

	return setUserPATH(newPath)
}

// removeFromWindowsUserPATH removes a directory from the user's persistent PATH.
func removeFromWindowsUserPATH(dir string) error {
	current, err := getUserPATH()
	if err != nil {
		return err
	}

	dirLower := strings.ToLower(dir)
	var kept []string
	found := false
	for _, entry := range strings.Split(current, ";") {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if strings.ToLower(trimmed) == dirLower {
			found = true
			continue
		}
		kept = append(kept, trimmed)
	}

	if !found {
		return nil // wasn't in PATH
	}

	return setUserPATH(strings.Join(kept, ";"))
}

// getUserPATH reads the user-level PATH from the Windows registry.
func getUserPATH() (string, error) {
	out, err := exec.Command("reg", "query", `HKCU\Environment`, "/v", "Path").CombinedOutput()
	if err != nil {
		// If the key doesn't exist, the user has no custom PATH entries
		if strings.Contains(string(out), "unable to find") {
			return "", nil
		}
		return "", fmt.Errorf("failed to query user PATH: %w", err)
	}

	// Parse reg query output: lines like "    Path    REG_SZ    value" or "    Path    REG_EXPAND_SZ    value"
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "path") {
			continue
		}
		// Split on REG_SZ or REG_EXPAND_SZ
		for _, regType := range []string{"REG_EXPAND_SZ", "REG_SZ"} {
			idx := strings.Index(line, regType)
			if idx >= 0 {
				value := strings.TrimSpace(line[idx+len(regType):])
				return value, nil
			}
		}
	}

	return "", nil
}

// setUserPATH writes a new value for the user-level PATH using setx.
func setUserPATH(value string) error {
	cmd := exec.Command("setx", "PATH", value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("setx failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}
