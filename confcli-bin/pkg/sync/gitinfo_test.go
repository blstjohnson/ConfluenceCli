package sync

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeGitFixture builds a minimal .git/ inside dir with HEAD and config
// matching head and origin.
func writeGitFixture(t *testing.T, dir, head, origin string) {
	t.Helper()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(head), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	cfg := "[core]\n\trepositoryformatversion = 0\n"
	if origin != "" {
		cfg += "[remote \"origin\"]\n\turl = " + origin + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestFindGitInfo_RootAndBranchAndRemote(t *testing.T) {
	root := t.TempDir()
	writeGitFixture(t, root, "ref: refs/heads/main\n", "https://example.com/owner/repo.git")
	sub := filepath.Join(root, "docs", "requirements")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	info, err := FindGitInfo(sub)
	if err != nil {
		t.Fatalf("FindGitInfo: %v", err)
	}
	gotRoot, _ := filepath.Abs(root)
	if info.Root != gotRoot {
		t.Errorf("Root = %q, want %q", info.Root, gotRoot)
	}
	if info.Branch != "main" {
		t.Errorf("Branch = %q, want main", info.Branch)
	}
	if info.RemoteURL != "https://example.com/owner/repo.git" {
		t.Errorf("RemoteURL = %q", info.RemoteURL)
	}
}

func TestFindGitInfo_BranchWithSlash(t *testing.T) {
	root := t.TempDir()
	writeGitFixture(t, root, "ref: refs/heads/feature/isfile-tag-req-v1\n", "https://x/y.git")

	info, err := FindGitInfo(root)
	if err != nil {
		t.Fatalf("FindGitInfo: %v", err)
	}
	if info.Branch != "feature/isfile-tag-req-v1" {
		t.Errorf("Branch = %q", info.Branch)
	}
}

func TestFindGitInfo_DetachedHEAD(t *testing.T) {
	root := t.TempDir()
	writeGitFixture(t, root, "a1b2c3d4e5f6\n", "https://x/y.git")

	info, err := FindGitInfo(root)
	if err != nil {
		t.Fatalf("FindGitInfo: %v", err)
	}
	if info.Branch != "" {
		t.Errorf("Branch = %q, want empty for detached HEAD", info.Branch)
	}
	if info.RemoteURL == "" {
		t.Errorf("RemoteURL should still be populated when HEAD is detached")
	}
}

func TestFindGitInfo_NoOriginConfigured(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	info, err := FindGitInfo(root)
	if err != nil {
		t.Fatalf("FindGitInfo: %v", err)
	}
	if info.RemoteURL != "" {
		t.Errorf("RemoteURL = %q, want empty when no origin", info.RemoteURL)
	}
	if info.Branch != "main" {
		t.Errorf("Branch = %q, want main", info.Branch)
	}
}

func TestFindGitInfo_NotInRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := FindGitInfo(dir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestFindGitInfo_WorktreeGitFile(t *testing.T) {
	// Linked worktrees have a `.git` FILE (not directory) containing
	// "gitdir: <path>". Verify we follow the indirection.
	mainRepo := t.TempDir()
	writeGitFixture(t, mainRepo, "ref: refs/heads/main\n", "https://x/y.git")
	mainGitDir := filepath.Join(mainRepo, ".git", "worktrees", "wt1")
	if err := os.MkdirAll(mainGitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainGitDir, "HEAD"), []byte("ref: refs/heads/wt-branch\n"), 0o644); err != nil {
		t.Fatalf("write worktree HEAD: %v", err)
	}
	// Reuse main config; worktrees share remote definitions via commondir
	// but our simple reader just looks at the gitdir's own config first.
	// For this test, copy the main config into the worktree gitdir.
	cfg, _ := os.ReadFile(filepath.Join(mainRepo, ".git", "config"))
	if err := os.WriteFile(filepath.Join(mainGitDir, "config"), cfg, 0o644); err != nil {
		t.Fatalf("write worktree config: %v", err)
	}

	worktreeRoot := t.TempDir()
	gitFile := filepath.Join(worktreeRoot, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+mainGitDir+"\n"), 0o644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}

	info, err := FindGitInfo(worktreeRoot)
	if err != nil {
		t.Fatalf("FindGitInfo: %v", err)
	}
	if info.Branch != "wt-branch" {
		t.Errorf("Branch = %q, want wt-branch (read from worktree gitdir HEAD)", info.Branch)
	}
}
