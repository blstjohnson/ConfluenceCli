//go:build windows

package commands

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
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

// addToWindowsUserPATHElevated attempts to add a directory to the user PATH
// by re-running the setx command with elevated privileges via runas/ShellExecute.
func addToWindowsUserPATHElevated(dir string) error {
	current, err := getUserPATH()
	if err != nil {
		return err
	}

	// Check if already present
	dirLower := strings.ToLower(dir)
	for _, entry := range strings.Split(current, ";") {
		if strings.ToLower(strings.TrimSpace(entry)) == dirLower {
			return nil
		}
	}

	newPath := current
	if newPath != "" && !strings.HasSuffix(newPath, ";") {
		newPath += ";"
	}
	newPath += dir

	// Use ShellExecute with "runas" verb to elevate setx
	return runElevated("setx", "PATH \""+newPath+"\"")
}

// runElevated runs a command with elevated privileges using ShellExecute's "runas" verb.
func runElevated(exe, args string) error {
	verb, _ := syscall.UTF16PtrFromString("runas")
	exePath, _ := syscall.UTF16PtrFromString(exe)
	argPtr, _ := syscall.UTF16PtrFromString(args)

	// ShellExecute with "runas" triggers the UAC elevation prompt
	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")

	ret, _, err := shellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(exePath)),
		uintptr(unsafe.Pointer(argPtr)),
		0,
		0, // SW_HIDE — don't show a console window
	)

	// ShellExecuteW returns > 32 on success
	if ret <= 32 {
		if err != nil && err != syscall.Errno(0) {
			return fmt.Errorf("elevation failed: %w", err)
		}
		return fmt.Errorf("elevation failed (code %d)", ret)
	}
	return nil
}
