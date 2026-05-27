package sync

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitInfo describes the discovered git working tree a sync runs inside.
// Fields are populated by FindGitInfo by reading .git/config and
// .git/HEAD directly — no `git` binary required, no clone state needed
// beyond what's already on disk.
type GitInfo struct {
	// Root is the absolute path of the directory containing the .git
	// directory (or .git file for worktrees).
	Root string
	// RemoteURL is the URL of the "origin" remote, or empty if no
	// origin is configured.
	RemoteURL string
	// Branch is the short branch name from HEAD, or empty if HEAD is
	// detached.
	Branch string
}

// FindGitInfo walks up from startDir looking for a .git entry (directory
// or worktree file). When found it reads enough of the config and HEAD
// to fill GitInfo. Returns os.ErrNotExist (wrapped) if no .git is found
// in any ancestor.
func FindGitInfo(startDir string) (*GitInfo, error) {
	root, gitDir, err := findGitDir(startDir)
	if err != nil {
		return nil, err
	}
	info := &GitInfo{Root: root}
	info.Branch, _ = readBranch(gitDir)
	info.RemoteURL, _ = readOriginURL(gitDir)
	return info, nil
}

// findGitDir returns (workTreeRoot, gitDir, error). gitDir is the actual
// directory containing config/HEAD/refs — usually <root>/.git, but for
// linked worktrees the .git file points elsewhere.
func findGitDir(startDir string) (string, string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", "", fmt.Errorf("abs path of %q: %w", startDir, err)
	}
	for {
		candidate := filepath.Join(dir, ".git")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return dir, candidate, nil
			}
			// .git is a file — worktree. Resolve "gitdir: <path>".
			data, rerr := os.ReadFile(candidate)
			if rerr != nil {
				return "", "", fmt.Errorf("read %q: %w", candidate, rerr)
			}
			line := strings.TrimSpace(string(data))
			const prefix = "gitdir:"
			if !strings.HasPrefix(line, prefix) {
				return "", "", fmt.Errorf("malformed worktree .git file at %q", candidate)
			}
			gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if !filepath.IsAbs(gitDir) {
				gitDir = filepath.Join(dir, gitDir)
			}
			return dir, gitDir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("no .git directory found at or above %q: %w", startDir, os.ErrNotExist)
		}
		dir = parent
	}
}

// readBranch returns the short branch name from HEAD, or "" if HEAD is
// detached. "ref: refs/heads/feature/x" → "feature/x".
func readBranch(gitDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "ref: refs/heads/"
	if !strings.HasPrefix(line, prefix) {
		return "", nil // detached HEAD or unexpected format
	}
	return strings.TrimPrefix(line, prefix), nil
}

// readOriginURL parses .git/config (INI-ish) for [remote "origin"]/url.
// We don't use a full INI library because the format is line-oriented and
// well-bounded; a 30-line scanner is less surface area than a dependency.
func readOriginURL(gitDir string) (string, error) {
	f, err := os.Open(filepath.Join(gitDir, "config"))
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inOrigin := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inOrigin = sectionIsOrigin(line)
			continue
		}
		if !inOrigin {
			continue
		}
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "url") {
			return strings.TrimSpace(val), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("git config: no remote.origin.url")
}

// sectionIsOrigin matches both [remote "origin"] and [remote.origin]
// section headers — git accepts the latter as a shorthand in some tooling.
func sectionIsOrigin(header string) bool {
	inner := strings.TrimSpace(header[1 : len(header)-1])
	if strings.EqualFold(inner, `remote "origin"`) {
		return true
	}
	if strings.EqualFold(inner, "remote.origin") {
		return true
	}
	return false
}

func splitKV(line string) (string, string, bool) {
	idx := strings.IndexByte(line, '=')
	if idx <= 0 {
		return "", "", false
	}
	return line[:idx], line[idx+1:], true
}
