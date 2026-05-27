package transforms

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"testing/fstest"
)

func newGitFileRewriter() *RewriteGitFileLinks {
	return &RewriteGitFileLinks{
		Macro: "view-git-file",
		Parameters: map[string]string{
			"path":          "{path}",
			"branch":        "refs/remotes/origin/{branch}",
			"repository-id": "6",
			"renderpanel":   "true",
		},
		Branch:          "feature/x",
		SyncRootRelRepo: "Docs/requirements",
	}
}

func TestRewriteGitFileLinks_YAMLLinkBecomesMacro(t *testing.T) {
	r := newGitFileRewriter()
	ctx := &TransformContext{
		PreContent: "see [config](../config/app.yaml) for details",
		PagePath:   "intro.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, `<ac:structured-macro ac:name="view-git-file"`) {
		t.Fatalf("expected macro: %s", ctx.PreContent)
	}
	if !strings.Contains(ctx.PreContent, `<ac:parameter ac:name="path">Docs/config/app.yaml</ac:parameter>`) {
		t.Errorf("wrong path; got: %s", ctx.PreContent)
	}
	if strings.Contains(ctx.PreContent, "renderpuml") {
		t.Errorf("renderpuml must not appear unless configured: %s", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_MarkdownAndPumlLeftAlone(t *testing.T) {
	// .md and .puml links are handled by other rewriters; this catch-all
	// must skip them so it doesn't double-rewrite.
	r := newGitFileRewriter()
	cases := []string{
		"[doc](other.md)",
		"[doc](other.markdown)",
		"[diag](diag.puml)",
		"[diag](diag.plantuml)",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			ctx := &TransformContext{PreContent: src, PagePath: "p.md"}
			if err := r.Apply(ctx); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if ctx.PreContent != src {
				t.Errorf("%q must pass through; got %q", src, ctx.PreContent)
			}
		})
	}
}

func TestRewriteGitFileLinks_ImagesAndExternalsSkipped(t *testing.T) {
	r := newGitFileRewriter()
	cases := []string{
		"![alt](pic.png)",                          // image embed
		"[ext](https://example.com/x.yaml)",         // external
		"[mail](mailto:a@b.com)",                    // mailto
		"[anchor](#section)",                        // pure anchor
		"[pic](photo.jpg)",                          // image extension
		"[wiki](/display/SPACE/Page)",               // absolute path — wiki URL
		"[wiki](/pages/viewpage.action?pageId=42)",  // confluence action URL
		"[query](relative/file.yaml?foo=bar)",       // query string in href
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			ctx := &TransformContext{PreContent: src, PagePath: "p.md"}
			if err := r.Apply(ctx); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if ctx.PreContent != src {
				t.Errorf("%q must pass through; got %q", src, ctx.PreContent)
			}
		})
	}
}

func TestRewriteGitFileLinks_ExtensionWhitelist(t *testing.T) {
	r := newGitFileRewriter()
	r.Extensions = []string{"yaml", ".json"} // mixed with/without dot, mixed case allowed
	ctx := &TransformContext{
		PreContent: "[a](a.yaml) [b](b.json) [c](c.sql)",
		PagePath:   "p.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// yaml and json should be rewritten; sql should pass through.
	if strings.Contains(ctx.PreContent, "[a](a.yaml)") {
		t.Errorf("yaml should be rewritten: %s", ctx.PreContent)
	}
	if strings.Contains(ctx.PreContent, "[b](b.json)") {
		t.Errorf("json should be rewritten: %s", ctx.PreContent)
	}
	if !strings.Contains(ctx.PreContent, "[c](c.sql)") {
		t.Errorf("sql should pass through when not in whitelist: %s", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_AnchorStripped(t *testing.T) {
	// view-git-file targets a whole file; drop any trailing #anchor.
	r := newGitFileRewriter()
	ctx := &TransformContext{
		PreContent: "[cfg](../config/app.yaml#section)",
		PagePath:   "p.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if strings.Contains(ctx.PreContent, "#section") {
		t.Errorf("anchor should not survive into the macro: %s", ctx.PreContent)
	}
	if !strings.Contains(ctx.PreContent, "Docs/config/app.yaml") {
		t.Errorf("path should be in macro body: %s", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_ExtensionlessHrefSkipped(t *testing.T) {
	// Links without extension (e.g. directory references) are too risky
	// to wrap blindly; leave them alone.
	r := newGitFileRewriter()
	src := "[dir](../some/folder)"
	ctx := &TransformContext{PreContent: src, PagePath: "p.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if ctx.PreContent != src {
		t.Errorf("extensionless link should pass through; got %q", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_FencedCodeSkipped(t *testing.T) {
	r := newGitFileRewriter()
	src := "```yaml\n[ref](a.yaml)\n```\n"
	ctx := &TransformContext{PreContent: src, PagePath: "p.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if ctx.PreContent != src {
		t.Errorf("code-fence content must not be rewritten; got %q", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_BoldWrapperStripped(t *testing.T) {
	r := newGitFileRewriter()
	ctx := &TransformContext{
		PreContent: "**[cfg](a.yaml)**",
		PagePath:   "p.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(ctx.PreContent), "**") {
		t.Errorf("outer ** should be stripped; got %q", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_EscapesRepoRootLogged(t *testing.T) {
	r := newGitFileRewriter()
	r.SyncRootRelRepo = "" // sync root == repo root, so ../foo.yaml escapes
	var buf bytes.Buffer
	r.Logger = log.New(&buf, "", 0)

	ctx := &TransformContext{
		PreContent: "[x](../escape.yaml)",
		PagePath:   "p.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(buf.String(), "escapes repo root") {
		t.Errorf("expected warning, got: %q", buf.String())
	}
	if !strings.Contains(ctx.PreContent, "[x](../escape.yaml)") {
		t.Errorf("unresolvable link should pass through unchanged; got %q", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_NoConfigIsNoOp(t *testing.T) {
	r := &RewriteGitFileLinks{} // no macro, no params
	src := "[x](a.yaml)"
	ctx := &TransformContext{PreContent: src, PagePath: "p.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if ctx.PreContent != src {
		t.Errorf("empty config should leave content untouched; got %q", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_InsideTableSkipped(t *testing.T) {
	// Confluence storage format doesn't render block-level structured
	// macros inside table cells — leave links inside markdown tables
	// as plain markdown.
	r := newGitFileRewriter()
	src := strings.Join([]string{
		"intro [a](top.yaml) here",
		"",
		"| name | file |",
		"| --- | --- |",
		"| cfg | [c](inside.yaml) |",
		"",
		"after [b](after.yaml)",
	}, "\n")
	ctx := &TransformContext{PreContent: src, PagePath: "p.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if strings.Contains(ctx.PreContent, "[c](inside.yaml)") == false {
		t.Errorf("link inside table should pass through unchanged; got:\n%s", ctx.PreContent)
	}
	if strings.Contains(ctx.PreContent, "[a](top.yaml)") {
		t.Errorf("link outside table should be rewritten; got:\n%s", ctx.PreContent)
	}
	if strings.Contains(ctx.PreContent, "[b](after.yaml)") {
		t.Errorf("link after table should be rewritten; got:\n%s", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_InlineEmitsCodeMacro(t *testing.T) {
	r := newGitFileRewriter()
	r.Mode = "inline"
	r.SyncRootRelRepo = "" // --from == repo root so sync-rel == repo-rel
	r.FSys = fstest.MapFS{
		"config/app.yaml": {Data: []byte("server: localhost\nport: 8080\n")},
	}
	ctx := &TransformContext{
		PreContent: "see [config](config/app.yaml) for details",
		PagePath:   "intro.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, `<ac:structured-macro ac:name="code"`) {
		t.Fatalf("expected code macro: %s", ctx.PreContent)
	}
	if !strings.Contains(ctx.PreContent, `<ac:parameter ac:name="language">yaml</ac:parameter>`) {
		t.Errorf("expected language=yaml: %s", ctx.PreContent)
	}
	if !strings.Contains(ctx.PreContent, `<ac:parameter ac:name="title">config/app.yaml</ac:parameter>`) {
		t.Errorf("expected title with sync-rel path: %s", ctx.PreContent)
	}
	if !strings.Contains(ctx.PreContent, "<![CDATA[server: localhost\nport: 8080\n]]>") {
		t.Errorf("expected file body inside CDATA: %s", ctx.PreContent)
	}
	// view-git-file (link mode) macro must not appear.
	if strings.Contains(ctx.PreContent, "view-git-file") {
		t.Errorf("inline mode should not emit view-git-file: %s", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_InlineFallsBackOnMissingFile(t *testing.T) {
	r := newGitFileRewriter()
	r.Mode = "inline"
	r.FSys = fstest.MapFS{} // no files
	var buf bytes.Buffer
	r.Logger = log.New(&buf, "", 0)

	ctx := &TransformContext{
		PreContent: "see [config](../config/app.yaml)",
		PagePath:   "intro.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, `ac:name="view-git-file"`) {
		t.Errorf("expected fallback to link mode: %s", ctx.PreContent)
	}
	if !strings.Contains(buf.String(), "falling back to link") {
		t.Errorf("expected fallback warning, got: %q", buf.String())
	}
}

func TestRewriteGitFileLinks_InlineFallsBackOnOversize(t *testing.T) {
	r := newGitFileRewriter()
	r.Mode = "inline"
	r.InlineMaxBytes = 10
	r.SyncRootRelRepo = ""
	r.FSys = fstest.MapFS{
		"big.yaml": {Data: []byte("0123456789ABCDEF")}, // 16 bytes > 10
	}
	var buf bytes.Buffer
	r.Logger = log.New(&buf, "", 0)

	ctx := &TransformContext{
		PreContent: "[c](big.yaml)",
		PagePath:   "p.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, `ac:name="view-git-file"`) {
		t.Errorf("expected fallback to link mode for oversize: %s", ctx.PreContent)
	}
	if !strings.Contains(buf.String(), "over inline cap") {
		t.Errorf("expected oversize warning, got: %q", buf.String())
	}
}

func TestRewriteGitFileLinks_InlineFallsBackWhenNoFSys(t *testing.T) {
	r := newGitFileRewriter()
	r.Mode = "inline" // but FSys is nil
	var buf bytes.Buffer
	r.Logger = log.New(&buf, "", 0)

	ctx := &TransformContext{
		PreContent: "[c](config.yaml)",
		PagePath:   "p.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, `ac:name="view-git-file"`) {
		t.Errorf("expected fallback to link mode with no FSys: %s", ctx.PreContent)
	}
	if !strings.Contains(buf.String(), "no source filesystem") {
		t.Errorf("expected no-fsys warning, got: %q", buf.String())
	}
}

func TestRewriteGitFileLinks_PerExtensionOverridesMode(t *testing.T) {
	// Global mode is inline; per-extension override forces sql back to link.
	r := newGitFileRewriter()
	r.Mode = "inline"
	r.PerExtension = map[string]string{"sql": "link"}
	r.SyncRootRelRepo = ""
	r.FSys = fstest.MapFS{
		"app.yaml":   {Data: []byte("k: v")},
		"schema.sql": {Data: []byte("CREATE TABLE t (x int);")},
	}
	ctx := &TransformContext{
		PreContent: "[y](app.yaml) [s](schema.sql)",
		PagePath:   "p.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// yaml → inline code macro
	if !strings.Contains(ctx.PreContent, `<ac:parameter ac:name="language">yaml</ac:parameter>`) {
		t.Errorf("yaml should be inline: %s", ctx.PreContent)
	}
	// sql → view-git-file (link mode)
	if !strings.Contains(ctx.PreContent, `<ac:structured-macro ac:name="view-git-file"`) {
		t.Errorf("sql should be link mode: %s", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_InlineEscapesCDATAClose(t *testing.T) {
	r := newGitFileRewriter()
	r.Mode = "inline"
	r.SyncRootRelRepo = ""
	r.FSys = fstest.MapFS{
		"data.xml": {Data: []byte("<a><![CDATA[old]]></a>")},
	}
	ctx := &TransformContext{
		PreContent: "[d](data.xml)",
		PagePath:   "p.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// The literal "]]>" must be split so the outer CDATA isn't closed early.
	if strings.Contains(ctx.PreContent, "<![CDATA[<a><![CDATA[old]]></a>]]>") {
		t.Errorf("CDATA close inside body must be escaped: %s", ctx.PreContent)
	}
	if !strings.Contains(ctx.PreContent, "]]]]><![CDATA[>") {
		t.Errorf("expected split CDATA close marker: %s", ctx.PreContent)
	}
}

func TestRewriteGitFileLinks_InlineSyncRootEscapeFallback(t *testing.T) {
	// SyncRootRelRepo non-empty means --from is below the repo root; a
	// link that's reachable in the repo but escapes --from can't be read
	// for inline. We expect a link-mode fallback with a warning.
	r := newGitFileRewriter()
	r.Mode = "inline"
	r.SyncRootRelRepo = "Docs/requirements"
	r.FSys = fstest.MapFS{} // empty; the escape check fires first
	var buf bytes.Buffer
	r.Logger = log.New(&buf, "", 0)

	ctx := &TransformContext{
		PreContent: "[c](../../shared/app.yaml)",
		PagePath:   "intro.md",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(ctx.PreContent, `ac:name="view-git-file"`) {
		t.Errorf("expected fallback to link mode: %s", ctx.PreContent)
	}
	if !strings.Contains(buf.String(), "escapes --from") {
		t.Errorf("expected escape warning, got: %q", buf.String())
	}
}

func TestRewriteGitFileLinks_DeterministicParamOrder(t *testing.T) {
	r := newGitFileRewriter()
	ctx1 := &TransformContext{PreContent: "[c](a.yaml)", PagePath: "p.md"}
	ctx2 := &TransformContext{PreContent: "[c](a.yaml)", PagePath: "p.md"}
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
