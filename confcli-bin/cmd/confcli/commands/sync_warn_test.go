package commands

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func capture(macro string, params map[string]string, branch string) string {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	warnMacroParams(logger, "plantuml", macro, params, branch)
	return buf.String()
}

func TestWarnMacroParams_PlaceholderQuestionMark(t *testing.T) {
	out := capture("view-git-file", map[string]string{
		"path":          "{path}",
		"repository-id": "?",
		"branch":        "{branch}",
	}, "refs/remotes/origin/feature/C2B")
	if !strings.Contains(out, "repository-id") || !strings.Contains(out, "placeholder") {
		t.Errorf("expected placeholder warning for repository-id, got: %q", out)
	}
}

func TestWarnMacroParams_BareLiteralBranch(t *testing.T) {
	out := capture("view-git-file", map[string]string{
		"branch": "feature/C2B",
	}, "feature/C2B")
	if !strings.Contains(out, "bare name") {
		t.Errorf("expected bare-branch warning, got: %q", out)
	}
}

func TestWarnMacroParams_FullRefBranchSilent(t *testing.T) {
	out := capture("view-git-file", map[string]string{
		"branch":        "{branch}",
		"repository-id": "41",
	}, "refs/remotes/origin/feature/C2B")
	if out != "" {
		t.Errorf("a correct config should produce no warnings, got: %q", out)
	}
}

func TestWarnMacroParams_BranchWarningOnlyForViewGitFile(t *testing.T) {
	out := capture("plantuml", map[string]string{"branch": "main"}, "main")
	if strings.Contains(out, "bare name") {
		t.Errorf("non view-git-file macro should not get the branch warning, got: %q", out)
	}
}

func TestSanitizeBranchParams(t *testing.T) {
	// Literal short branch is expanded by branch_ref.
	got := sanitizeBranchParams(map[string]string{
		"path":   "{path}",
		"branch": "feature/C2B",
	}, "remote")
	if got["branch"] != "refs/remotes/origin/feature/C2B" {
		t.Errorf("literal branch should expand, got %q", got["branch"])
	}
	if got["path"] != "{path}" {
		t.Errorf("non-branch params must be untouched, got %q", got["path"])
	}

	// A {branch} placeholder is left for the rewriter's expanded Branch field.
	keep := sanitizeBranchParams(map[string]string{"branch": "{branch}"}, "remote")
	if keep["branch"] != "{branch}" {
		t.Errorf("{branch} placeholder must be left untouched, got %q", keep["branch"])
	}

	// Without branch_ref, a literal bare branch is left as-is (warning only).
	none := sanitizeBranchParams(map[string]string{"branch": "feature/C2B"}, "")
	if none["branch"] != "feature/C2B" {
		t.Errorf("no branch_ref should leave the value unchanged, got %q", none["branch"])
	}

	// Already-full ref is idempotent.
	full := sanitizeBranchParams(map[string]string{"branch": "refs/remotes/origin/x"}, "remote")
	if full["branch"] != "refs/remotes/origin/x" {
		t.Errorf("full ref must be idempotent, got %q", full["branch"])
	}
}
