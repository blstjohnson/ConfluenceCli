package commands

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// githubRelease represents a GitHub release API response
type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
	HTMLURL    string        `json:"html_url"`
}

// githubAsset represents a GitHub release asset
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

const githubRepo = "blstjohnson/ConfluenceCli"

// NewUpdateCmd creates the update command
func NewUpdateCmd() *cobra.Command {
	var check bool
	var force bool
	var preRelease bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update confcli to the latest version",
		Long: `Check for and install updates from GitHub Releases.

By default, downloads and installs the latest stable release.
Use --check to only check for updates without installing.
Use --pre-release to include pre-release versions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd, check, force, preRelease)
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "Only check for updates, don't install")
	cmd.Flags().BoolVar(&force, "force", false, "Skip version comparison, download anyway")
	cmd.Flags().BoolVar(&preRelease, "pre-release", false, "Include pre-release versions")

	return cmd
}

func runUpdate(cmd *cobra.Command, check, force, preRelease bool) error {
	release, err := fetchLatestRelease(preRelease)
	if err != nil {
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	remoteVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := Version

	if !force && currentVersion == remoteVersion {
		fmt.Fprintf(cmd.OutOrStdout(), "confcli is already up to date (%s)\n", currentVersion)
		return nil
	}

	if check {
		if currentVersion != remoteVersion {
			fmt.Fprintf(cmd.OutOrStdout(), "Update available: %s -> %s\n", currentVersion, remoteVersion)
			fmt.Fprintf(cmd.OutOrStdout(), "Release: %s\n", release.HTMLURL)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "confcli is up to date (%s)\n", currentVersion)
		}
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Updating confcli %s -> %s\n", currentVersion, remoteVersion)

	assetName, err := expectedAssetName()
	if err != nil {
		return err
	}

	asset, err := findAsset(release, assetName)
	if err != nil {
		return err
	}

	checksumAsset, err := findAsset(release, "checksums.txt")
	if err != nil {
		return fmt.Errorf("checksums.txt not found in release assets")
	}

	tmpDir, err := os.MkdirTemp("", "confcli-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download asset
	fmt.Fprintf(cmd.OutOrStdout(), "Downloading %s...\n", assetName)
	assetPath := filepath.Join(tmpDir, assetName)
	if err := downloadFile(asset.BrowserDownloadURL, assetPath); err != nil {
		return fmt.Errorf("failed to download %s: %w", assetName, err)
	}

	// Download and verify checksum
	checksumPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(checksumAsset.BrowserDownloadURL, checksumPath); err != nil {
		return fmt.Errorf("failed to download checksums: %w", err)
	}

	if err := verifyChecksum(assetPath, checksumPath, assetName); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Checksum verified.")

	// Extract binary
	binaryPath, err := extractBinary(assetPath, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to extract binary: %w", err)
	}

	// Replace current binary
	currentBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine current executable path: %w", err)
	}
	currentBin, err = filepath.EvalSymlinks(currentBin)
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}

	if err := replaceBinary(currentBin, binaryPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Successfully updated to %s\n", remoteVersion)
	return nil
}

func fetchLatestRelease(preRelease bool) (*githubRelease, error) {
	if !preRelease {
		// Use the /latest endpoint which skips pre-releases
		url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
		return fetchRelease(url)
	}

	// For pre-release, list all releases and pick the first one
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=1", githubRepo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub response: %w", err)
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found")
	}
	return &releases[0], nil
}

func fetchRelease(url string) (*githubRelease, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub response: %w", err)
	}
	return &release, nil
}

func expectedAssetName() (string, error) {
	osName := runtime.GOOS
	archName := runtime.GOARCH

	ext := "tar.gz"
	if osName == "windows" {
		ext = "zip"
	}

	name := fmt.Sprintf("confcli-%s-%s.%s", osName, archName, ext)

	// Verify this is a supported combination
	supported := map[string]bool{
		"confcli-linux-amd64.tar.gz":  true,
		"confcli-darwin-arm64.tar.gz": true,
		"confcli-windows-amd64.zip":   true,
	}
	if !supported[name] {
		return "", fmt.Errorf("no release asset available for %s/%s", osName, archName)
	}

	return name, nil
}

func findAsset(release *githubRelease, name string) (*githubAsset, error) {
	for i := range release.Assets {
		if release.Assets[i].Name == name {
			return &release.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("asset %q not found in release %s", name, release.TagName)
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func verifyChecksum(filePath, checksumFile, assetName string) error {
	// Compute SHA256 of the downloaded file
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))

	// Parse checksums.txt for expected hash
	data, err := os.ReadFile(checksumFile)
	if err != nil {
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == assetName {
			expected := parts[0]
			if actual != expected {
				return fmt.Errorf("expected %s, got %s", expected, actual)
			}
			return nil
		}
	}

	return fmt.Errorf("no checksum found for %s in checksums.txt", assetName)
}

func extractBinary(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destDir)
	}
	return extractTarGz(archivePath, destDir)
}

func extractTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	binaryName := "confcli"

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// Look for the confcli binary (could be at root or in a subdirectory)
		name := filepath.Base(hdr.Name)
		if name != binaryName {
			continue
		}

		destPath := filepath.Join(destDir, binaryName)
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", err
		}
		out.Close()
		return destPath, nil
	}

	return "", fmt.Errorf("binary %q not found in archive", binaryName)
}

func extractZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	binaryName := "confcli.exe"

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name != binaryName {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return "", err
		}

		destPath := filepath.Join(destDir, binaryName)
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			rc.Close()
			return "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return "", err
		}
		out.Close()
		rc.Close()
		return destPath, nil
	}

	return "", fmt.Errorf("binary %q not found in archive", binaryName)
}
