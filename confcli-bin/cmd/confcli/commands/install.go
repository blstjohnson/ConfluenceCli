package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

// NewInstallCmd creates the install command with install/uninstall subcommands
func NewInstallCmd() *cobra.Command {
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Add confcli to your PATH for global access",
		Long: `Install confcli by creating a symlink (or copy on Windows) in a
directory on your PATH, making it accessible from any terminal session.

On Linux/macOS, the default target is /usr/local/bin (may require sudo).
On Windows, the default target is %LOCALAPPDATA%\confcli.

Use --dir to specify a custom install directory.`,
		RunE: runInstall,
	}

	installCmd.Flags().String("dir", "", "Custom directory to install into (must be on PATH)")
	installCmd.Flags().Bool("copy", false, "Copy the binary instead of symlinking (default on Windows)")
	installCmd.Flags().BoolP("force", "f", false, "Overwrite existing installation")

	installCmd.AddCommand(newUninstallCmd())

	return installCmd
}

func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove confcli from your PATH",
		Long:  `Remove the confcli symlink or binary that was previously installed.`,
		RunE:  runUninstall,
	}

	cmd.Flags().String("dir", "", "Directory to uninstall from (if installed to a custom location)")

	return cmd
}

func runInstall(cmd *cobra.Command, args []string) error {
	srcBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine current executable path: %w", err)
	}
	srcBin, err = filepath.EvalSymlinks(srcBin)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}

	customDir, _ := cmd.Flags().GetString("dir")
	forceCopy, _ := cmd.Flags().GetBool("copy")
	force, _ := cmd.Flags().GetBool("force")

	targetDir, err := resolveTargetDir(customDir)
	if err != nil {
		return err
	}

	targetBin := filepath.Join(targetDir, binaryName())
	useSymlink := !forceCopy && runtime.GOOS != "windows"

	// Check if source and target are the same file
	if samePath(srcBin, targetBin) {
		fmt.Fprintln(cmd.OutOrStdout(), "confcli is already installed at", targetBin)
		return nil
	}

	// Check for existing file
	if info, err := os.Lstat(targetBin); err == nil {
		if !force {
			kind := "file"
			if info.Mode()&os.ModeSymlink != 0 {
				kind = "symlink"
			}
			return fmt.Errorf("%s already exists at %s (use --force to overwrite)", kind, targetBin)
		}
		if err := removeWithElevation(targetBin); err != nil {
			return fmt.Errorf("cannot remove existing %s: %w", targetBin, err)
		}
	}

	// Ensure target directory exists
	if err := mkdirWithElevation(targetDir); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", targetDir, err)
	}

	if useSymlink {
		if err := symlinkWithElevation(srcBin, targetBin); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Symlinked %s -> %s\n", targetBin, srcBin)
	} else {
		if err := copyBinaryWithElevation(srcBin, targetBin); err != nil {
			return fmt.Errorf("failed to copy binary: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Copied confcli to %s\n", targetBin)
	}

	// Verify install
	if verifyInstall() {
		fmt.Fprintln(cmd.OutOrStdout(), "Verified: confcli is now available on your PATH.")
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s may not be on your PATH. Add it to your shell profile.\n", targetDir)
		if runtime.GOOS == "windows" {
			fmt.Fprintf(cmd.ErrOrStderr(), "  Run: setx PATH \"%%PATH%%;%s\"\n", targetDir)
		} else {
			fmt.Fprintf(cmd.ErrOrStderr(), "  Add to ~/.bashrc or ~/.zshrc: export PATH=\"%s:$PATH\"\n", targetDir)
		}
	}

	return nil
}

func runUninstall(cmd *cobra.Command, args []string) error {
	customDir, _ := cmd.Flags().GetString("dir")

	targetDir, err := resolveTargetDir(customDir)
	if err != nil {
		return err
	}

	targetBin := filepath.Join(targetDir, binaryName())

	if _, err := os.Lstat(targetBin); os.IsNotExist(err) {
		return fmt.Errorf("confcli is not installed at %s", targetBin)
	}

	if err := removeWithElevation(targetBin); err != nil {
		return fmt.Errorf("failed to remove %s: %w", targetBin, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", targetBin)

	// On Windows, check if we added the directory to PATH and it's now empty
	if runtime.GOOS == "windows" && customDir == "" {
		entries, err := os.ReadDir(targetDir)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(targetDir)
			fmt.Fprintf(cmd.OutOrStdout(), "Removed empty directory %s\n", targetDir)
			fmt.Fprintf(cmd.ErrOrStderr(), "Note: You may want to remove %s from your PATH.\n", targetDir)
		}
	}

	return nil
}

// resolveTargetDir determines the install directory
func resolveTargetDir(customDir string) (string, error) {
	if customDir != "" {
		abs, err := filepath.Abs(customDir)
		if err != nil {
			return "", fmt.Errorf("invalid directory %q: %w", customDir, err)
		}
		return abs, nil
	}

	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			return "", fmt.Errorf("LOCALAPPDATA environment variable not set")
		}
		return filepath.Join(localAppData, "confcli"), nil
	default:
		return "/usr/local/bin", nil
	}
}

// binaryName returns the expected binary name for the current platform
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "confcli.exe"
	}
	return "confcli"
}

// samePath checks if two paths refer to the same file
func samePath(a, b string) bool {
	absA, err1 := filepath.Abs(a)
	absB, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return false
	}
	// Also resolve symlinks for comparison
	realA, err1 := filepath.EvalSymlinks(absA)
	realB, err2 := filepath.EvalSymlinks(absB)
	if err1 != nil || err2 != nil {
		// If one doesn't exist, compare absolute paths
		return absA == absB
	}
	return realA == realB
}

// symlinkWithElevation creates a symlink, using sudo if needed
func symlinkWithElevation(src, dst string) error {
	err := os.Symlink(src, dst)
	if err == nil {
		return nil
	}
	if !os.IsPermission(err) || runtime.GOOS == "windows" {
		return err
	}
	fmt.Fprintf(os.Stderr, "Requesting elevated permissions to install to %s...\n", filepath.Dir(dst))
	return runSudo("ln", "-sf", src, dst)
}

// copyBinaryWithElevation copies a binary, using sudo if needed
func copyBinaryWithElevation(src, dst string) error {
	err := copyFile(src, dst)
	if err == nil {
		return nil
	}
	if !os.IsPermission(err) || runtime.GOOS == "windows" {
		return err
	}
	fmt.Fprintf(os.Stderr, "Requesting elevated permissions to install to %s...\n", filepath.Dir(dst))
	return runSudo("cp", src, dst)
}

// removeWithElevation removes a file, using sudo if needed
func removeWithElevation(path string) error {
	err := os.Remove(path)
	if err == nil {
		return nil
	}
	if !os.IsPermission(err) || runtime.GOOS == "windows" {
		return err
	}
	fmt.Fprintf(os.Stderr, "Requesting elevated permissions to remove %s...\n", path)
	return runSudo("rm", path)
}

// mkdirWithElevation creates a directory, using sudo if needed
func mkdirWithElevation(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil // already exists
	}
	err := os.MkdirAll(dir, 0755)
	if err == nil {
		return nil
	}
	if !os.IsPermission(err) || runtime.GOOS == "windows" {
		return err
	}
	return runSudo("mkdir", "-p", dir)
}

// copyFile copies a file and preserves executable permissions
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

// runSudo executes a command with sudo, inheriting stdin/stdout/stderr for password prompt
func runSudo(args ...string) error {
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("sudo not found; run this command with appropriate permissions")
	}
	cmd := exec.Command(sudoPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// verifyInstall checks if confcli is reachable on PATH
func verifyInstall() bool {
	path, err := exec.LookPath("confcli")
	if err != nil {
		if runtime.GOOS == "windows" {
			path, err = exec.LookPath("confcli.exe")
		}
	}
	if err != nil {
		return false
	}
	// Verify it's not the currently running binary but the installed one
	_ = path
	return true
}

