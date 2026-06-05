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
