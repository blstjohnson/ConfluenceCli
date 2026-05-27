package transforms

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// newViewGitFileRewriter mirrors the ekassir "view-git-file" macro
// shape: repository-id, branch (refs/remotes/origin/<name> form),
// path, plus renderpuml/renderpanel toggles. The transform should be
// macro-agnostic, but this is the actual real-world consumer.
func newViewGitFileRewriter() *RewritePlantUMLLinks {
	return &RewritePlantUMLLinks{
		Macro: "view-git-file",
		Parameters: map[string]string{
			"path":          "{path}",
			"branch":        "refs/remotes/origin/{branch}",
			"repository-id": "6",
			"renderpuml":    "true",
			"renderpanel":   "true",
		},
		Branch:          "feature/x",
		SyncRootRelRepo: "Docs/requirements",
	}
}

func TestRewritePlantUMLLinks_ViewGitFileMacroShape(t *testing.T) {
	r := newViewGitFileRewriter()
	ctx := &TransformContext{
		PreContent: "see [diagram](../Diagrams/foo.puml) here",
		PagePath:   "Client_Workflow_Diagrams.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := []string{
		`<ac:structured-macro ac:name="view-git-file" ac:schema-version="1">`,
		`<ac:parameter ac:name="path">Docs/Diagrams/foo.puml</ac:parameter>`,
		`<ac:parameter ac:name="branch">refs/remotes/origin/feature/x</ac:parameter>`,
		`<ac:parameter ac:name="repository-id">6</ac:parameter>`,
		`<ac:parameter ac:name="renderpuml">true</ac:parameter>`,
		`<ac:parameter ac:name="renderpanel">true</ac:parameter>`,
		`</ac:structured-macro>`,
	}
	for _, fragment := range want {
		if !strings.Contains(ctx.PreContent, fragment) {
			t.Errorf("output missing %q\nfull: %s", fragment, ctx.PreContent)
		}
	}
}

func TestRewritePlantUMLLinks_ParametersAreDeterministic(t *testing.T) {
	// Map iteration is randomized; emitted XML should still be stable
	// across runs so hash-based change detection (skip vs update) works.
	r := newViewGitFileRewriter()
	ctx1 := &TransformContext{PreContent: "[d](foo.puml)", PagePath: "Client_Workflow_Diagrams.md"}
	ctx2 := &TransformContext{PreContent: "[d](foo.puml)", PagePath: "Client_Workflow_Diagrams.md"}
	if err := r.Apply(ctx1); err != nil {
		t.Fatalf("Apply ctx1: %v", err)
	}
	if err := r.Apply(ctx2); err != nil {
		t.Fatalf("Apply ctx2: %v", err)
	}
	if ctx1.PreContent != ctx2.PreContent {
		t.Errorf("non-deterministic output:\n  %s\n  %s", ctx1.PreContent, ctx2.PreContent)
	}
}

func TestRewritePlantUMLLinks_CyrillicPathPreserved(t *testing.T) {
	// view-git-file expects a literal repo-relative path (no URL
	// encoding) — only XML body escaping is needed, and Cyrillic is
	// pass-through.
	r := newViewGitFileRewriter()
	r.SyncRootRelRepo = "Docs"
	ctx := &TransformContext{
		PreContent: "[ц](Сценарий/файл.puml)",
		PagePath:   "requirements/index.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, `>Docs/requirements/Сценарий/файл.puml<`) {
		t.Errorf("Cyrillic path should be emitted literally; got: %s", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_XMLEscapingInParamValues(t *testing.T) {
	// If a path contains & or < (rare but possible), it must be
	// XML-escaped in the parameter body so the resulting storage
	// XHTML parses.
	r := &RewritePlantUMLLinks{
		Macro:      "view-git-file",
		Parameters: map[string]string{"path": "{path}"},
		Branch:     "main",
	}
	ctx := &TransformContext{
		PreContent: "[d](a&b<c.puml)",
		PagePath:   "p.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, "a&amp;b&lt;c.puml") {
		t.Errorf("special chars not escaped in body; got: %s", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_ImageEmbedSkipped(t *testing.T) {
	r := newViewGitFileRewriter()
	src := "![alt](sibling.puml)"
	ctx := &TransformContext{PreContent: src, PagePath: "a.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if ctx.PreContent != src {
		t.Errorf("image embed must be left alone; got %q", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_FencedCodeBlockSkipped(t *testing.T) {
	r := newViewGitFileRewriter()
	src := "```\n[diagram](sibling.puml)\n```\n"
	ctx := &TransformContext{PreContent: src, PagePath: "a.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if ctx.PreContent != src {
		t.Errorf("fenced code content must not be rewritten; got %q", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_BoldWrapperStripped(t *testing.T) {
	r := newViewGitFileRewriter()
	ctx := &TransformContext{
		PreContent: "**[name.puml](../Diagrams/name.puml)**",
		PagePath:   "Client_Workflow_Diagrams.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(ctx.PreContent), "**") {
		t.Errorf("outer ** should be stripped; got %q", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_BoldKeptWhenWrappingMore(t *testing.T) {
	r := newViewGitFileRewriter()
	ctx := &TransformContext{
		PreContent: "**before [n](n.puml) after**",
		PagePath:   "a.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.HasPrefix(ctx.PreContent, "**before ") || !strings.HasSuffix(ctx.PreContent, " after**") {
		t.Errorf("surrounding ** must be preserved when not bracketing the link; got %q", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_EscapesRepoRootLogged(t *testing.T) {
	r := newViewGitFileRewriter()
	r.SyncRootRelRepo = "" // sync root == repo root, so ../foo.puml escapes
	var buf bytes.Buffer
	r.Logger = log.New(&buf, "", 0)

	ctx := &TransformContext{
		PreContent: "[x](../escape.puml)",
		PagePath:   "a.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(buf.String(), "escapes repo root") {
		t.Errorf("expected warning, got: %q", buf.String())
	}
	if !strings.Contains(ctx.PreContent, "[x](../escape.puml)") {
		t.Errorf("unresolvable link should pass through unchanged; got %q", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_NoConfigIsNoOp(t *testing.T) {
	r := &RewritePlantUMLLinks{} // no macro, no params
	src := "[x](a.puml)"
	ctx := &TransformContext{PreContent: src, PagePath: "a.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if ctx.PreContent != src {
		t.Errorf("empty config should leave content untouched; got %q", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_EmptyParametersIsNoOp(t *testing.T) {
	r := &RewritePlantUMLLinks{Macro: "view-git-file"} // macro set but no params
	src := "[x](a.puml)"
	ctx := &TransformContext{PreContent: src, PagePath: "a.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if ctx.PreContent != src {
		t.Errorf("no parameters → no rewrite; got %q", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_InsideTableSkipped(t *testing.T) {
	// Block-level view-git-file macro can't live inside a Confluence
	// table cell; leave .puml links inside markdown tables as plain
	// markdown.
	r := newViewGitFileRewriter()
	src := strings.Join([]string{
		"see [outer](outer.puml) first",
		"",
		"| diagram | desc |",
		"| --- | --- |",
		"| [inside](inside.puml) | row |",
		"",
		"and [trailing](trailing.puml)",
	}, "\n")
	ctx := &TransformContext{PreContent: src, PagePath: "a.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, "[inside](inside.puml)") {
		t.Errorf("link inside table should pass through unchanged; got:\n%s", ctx.PreContent)
	}
	if strings.Contains(ctx.PreContent, "[outer](outer.puml)") {
		t.Errorf("link before table should be rewritten; got:\n%s", ctx.PreContent)
	}
	if strings.Contains(ctx.PreContent, "[trailing](trailing.puml)") {
		t.Errorf("link after table should be rewritten; got:\n%s", ctx.PreContent)
	}
}

func TestRewritePlantUMLLinks_PlantumlExtensionAlsoMatched(t *testing.T) {
	r := newViewGitFileRewriter()
	ctx := &TransformContext{
		PreContent: "[d](../Diagrams/foo.plantuml)",
		PagePath:   "a.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, "ac:structured-macro") {
		t.Errorf(".plantuml extension should also match: %s", ctx.PreContent)
	}
}
