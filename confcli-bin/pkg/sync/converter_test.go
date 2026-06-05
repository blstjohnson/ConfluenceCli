package sync

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"confcli/pkg/transforms"
)

func TestBuildPathMap_CoversWalkedFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md":        {Data: []byte("a")},
		"docs/b.md":   {Data: []byte("b")},
		"drafts/c.md": {Data: []byte("c")},
		"docs/_x.md":  {Data: []byte("x")},
	}
	profile := &transforms.ImportProfile{Kind: "import"}
	profile.Tree.Skip = []string{"drafts", "**/_*.md"}

	pm, err := BuildPathMap(profile, fsys)
	if err != nil {
		t.Fatalf("BuildPathMap: %v", err)
	}
	wantKeys := []string{"a.md", "docs/b.md"}
	for _, k := range wantKeys {
		ref, ok := pm[k]
		if !ok {
			t.Errorf("missing %q in pathMap; got %+v", k, pm)
			continue
		}
		// Title is the filename without .md (basic derivation).
		if ref.Title == "" {
			t.Errorf("%q: title is empty", k)
		}
	}
	if _, ok := pm["drafts/c.md"]; ok {
		t.Errorf("drafts/c.md should be skipped, but appears in map")
	}
}

func TestNewMarkdownConverter_RewritesLinkAndConverts(t *testing.T) {
	fsys := fstest.MapFS{
		"src.md":   {Data: []byte("see [other](other.md)")},
		"other.md": {Data: []byte("# other")},
	}
	profile := &transforms.ImportProfile{Kind: "import"}
	pm, err := BuildPathMap(profile, fsys)
	if err != nil {
		t.Fatalf("BuildPathMap: %v", err)
	}

	conv := NewMarkdownConverter(pm, nil, nil, nil)
	out, err := conv(context.Background(), []byte("see [other](other.md)"), "src.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	// Rewriter emits ac:link with the target page's title (here: "other").
	if !strings.Contains(out, "ac:link") {
		t.Errorf("expected ac:link in output, got: %s", out)
	}
	if !strings.Contains(out, `ri:content-title="other"`) {
		t.Errorf("expected ri:content-title=\"other\" in output, got: %s", out)
	}
}

func TestNewMarkdownConverter_AcLinkSurvivesGoldmark(t *testing.T) {
	// Regression: <ac:link> has a colon in the tag name, which makes it
	// invalid as a CommonMark HTML tag. Without the placeholder shim it
	// gets autolinked and HTML-escaped. The shim should restore the
	// raw XML in the storage output verbatim.
	fsys := fstest.MapFS{
		"src.md":   {Data: []byte("see [Other](other.md) end")},
		"other.md": {Data: []byte("# other")},
	}
	pm, err := BuildPathMap(&transforms.ImportProfile{Kind: "import"}, fsys)
	if err != nil {
		t.Fatalf("BuildPathMap: %v", err)
	}
	conv := NewMarkdownConverter(pm, nil, nil, nil)
	out, err := conv(context.Background(), []byte("see [Other](other.md) end"), "src.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	// Look for the literal opening tag and the unescaped CDATA wrapper.
	if !strings.Contains(out, "<ac:link>") || !strings.Contains(out, "</ac:link>") {
		t.Errorf("ac:link tags should be intact (not escaped) in output:\n%s", out)
	}
	if strings.Contains(out, "&lt;ac:link") || strings.Contains(out, "&lt;ri:page") {
		t.Errorf("rewritten XML must not be HTML-escaped in output:\n%s", out)
	}
}

func TestNewMarkdownConverter_IncludeMacroSurvivesGoldmark(t *testing.T) {
	// ![text](page.md) becomes an "include" structured-macro that nests an
	// <ac:link>. The stash must capture the whole macro (structured-macro
	// before ac:link), or the inner link's placeholder is left un-restored.
	fsys := fstest.MapFS{
		"src.md":   {Data: []byte("x")},
		"other.md": {Data: []byte("# other")},
	}
	pm, err := BuildPathMap(&transforms.ImportProfile{Kind: "import"}, fsys)
	if err != nil {
		t.Fatalf("BuildPathMap: %v", err)
	}
	conv := NewMarkdownConverter(pm, nil, nil, nil)
	out, err := conv(context.Background(), []byte("before ![Other](other.md) after"), "src.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, `<ac:structured-macro ac:name="include"`) {
		t.Errorf("include macro should be present:\n%s", out)
	}
	if !strings.Contains(out, `<ri:page ri:content-title="other" />`) {
		t.Errorf("include macro should reference the target page:\n%s", out)
	}
	if strings.Contains(out, "confcli-xml-") {
		t.Errorf("no stash placeholder should leak into output:\n%s", out)
	}
}

func TestNewMarkdownConverter_VoidTagsSelfClosed(t *testing.T) {
	// User-authored <br> in a table cell (very common in Russian docs)
	// must come out as <br /> so Confluence's strict XHTML parser accepts it.
	src := []byte("| A | B |\n|---|---|\n| line1<br>line2 | x |\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if strings.Contains(out, "<br>") || strings.Contains(out, "<br >") {
		t.Errorf("unclosed <br> in output: %s", out)
	}
	if !strings.Contains(out, "<br />") {
		t.Errorf("expected <br /> in output: %s", out)
	}
}

func TestNewMarkdownConverter_VoidTagsInsideCDATAPreserved(t *testing.T) {
	// Code blocks become <ac:structured-macro><ac:plain-text-body>
	// <![CDATA[...]]>. Literal <br> inside CDATA is user text and
	// must not be rewritten.
	src := []byte("```html\n<br>not a tag here\n```\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, "<br>not a tag here") {
		t.Errorf("CDATA content was rewritten; got: %s", out)
	}
}

func TestNewMarkdownConverter_UnknownTagsEscaped(t *testing.T) {
	// CamelCase / unknown tag-looking text in user-authored markdown
	// (e.g. API field-name placeholders) must be HTML-escaped so the
	// Confluence storage parser doesn't reject the page.
	src := []byte("Field: <FIWalletId> and <clientType>\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if strings.Contains(out, "<FIWalletId>") || strings.Contains(out, "<clientType>") {
		t.Errorf("placeholder tags should be escaped: %s", out)
	}
	if !strings.Contains(out, "&lt;FIWalletId&gt;") {
		t.Errorf("expected escaped form &lt;FIWalletId&gt;: %s", out)
	}
}

func TestNewMarkdownConverter_MixedCaseLookalikesEscaped(t *testing.T) {
	// Mixed-case names like <Object> and <Data> are placeholders even
	// though "object" and "data" are valid HTML tags in lowercase.
	// XHTML is case-sensitive; the lowercase forms are real tags, the
	// mixed-case forms are user-authored literal text.
	src := []byte("Type: <Object>, payload: <Data>\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if strings.Contains(out, "<Object>") || strings.Contains(out, "<Data>") {
		t.Errorf("mixed-case placeholder tags must be escaped: %s", out)
	}
	if !strings.Contains(out, "&lt;Object&gt;") || !strings.Contains(out, "&lt;Data&gt;") {
		t.Errorf("expected escaped forms in output: %s", out)
	}
}

func TestNewMarkdownConverter_KnownHTMLTagsPreserved(t *testing.T) {
	// Real HTML tags must continue to pass through unchanged so users
	// can mix HTML when they need to.
	src := []byte("inline <em>x</em> and <strong>y</strong>\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, "<em>") || !strings.Contains(out, "<strong>") {
		t.Errorf("known HTML tags must not be escaped: %s", out)
	}
}

func TestNewMarkdownConverter_NamespacedTagsPreserved(t *testing.T) {
	// Confluence storage tags (<ac:link>, <ri:page>) must survive — the
	// bare-tag regex only matches no-namespace tags, so they're skipped.
	fsys := fstest.MapFS{
		"src.md":   {Data: []byte("see [other](other.md)")},
		"other.md": {Data: []byte("# other")},
	}
	pm, err := BuildPathMap(&transforms.ImportProfile{Kind: "import"}, fsys)
	if err != nil {
		t.Fatalf("BuildPathMap: %v", err)
	}
	conv := NewMarkdownConverter(pm, nil, nil, nil)
	out, err := conv(context.Background(), []byte("see [other](other.md)"), "src.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, "<ac:link>") || !strings.Contains(out, "<ri:page") {
		t.Errorf("namespaced storage tags must not be escaped: %s", out)
	}
}

func TestNewMarkdownConverter_UnknownTagsInsideCDATAPreserved(t *testing.T) {
	// Code blocks → CDATA. The escaper must skip CDATA so code samples
	// containing <Tag>-style placeholders survive verbatim.
	src := []byte("```xml\n<FIWalletId>123</FIWalletId>\n```\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, "<FIWalletId>123</FIWalletId>") {
		t.Errorf("CDATA content was rewritten; got: %s", out)
	}
}

func TestNewMarkdownConverter_EmptyInputBecomesPlaceholder(t *testing.T) {
	// Empty marker files should still produce a non-empty page so the
	// folder gets a parent stub on Confluence.
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), []byte(""), "marker.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("expected non-empty placeholder, got %q", out)
	}
}

func TestNewMarkdownConverter_WhitespaceOnlyBecomesPlaceholder(t *testing.T) {
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), []byte("   \n\n  \n"), "marker.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("expected placeholder for whitespace-only input, got %q", out)
	}
}

func TestNewMarkdownConverter_PlantUMLMacroSurvivesGoldmark(t *testing.T) {
	// End-to-end: pass a plantuml rewriter into the converter, feed a
	// markdown link to a .puml file, and verify the rendered storage
	// contains the intact <ac:structured-macro> XML.
	puml := &transforms.RewritePlantUMLLinks{
		Macro: "view-git-file",
		Parameters: map[string]string{
			"path":   "{path}",
			"branch": "refs/remotes/origin/{branch}",
		},
		Branch:          "main",
		SyncRootRelRepo: "Docs/requirements",
	}
	conv := NewMarkdownConverter(nil, puml, nil, nil)
	out, err := conv(context.Background(),
		[]byte("see **[foo.puml](../Diagrams/foo.puml)**"),
		"Client_Workflow_Diagrams.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, `<ac:structured-macro ac:name="view-git-file"`) {
		t.Errorf("macro should survive goldmark intact:\n%s", out)
	}
	if strings.Contains(out, "&lt;ac:structured-macro") {
		t.Errorf("macro tags must not be HTML-escaped:\n%s", out)
	}
}

func TestNewMarkdownConverter_NilPathMapPassesLinksThrough(t *testing.T) {
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), []byte("see [x](x.md)"), "a.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	// Without rewriting, goldmark renders the link as a plain <a>.
	if strings.Contains(out, "ac:link") {
		t.Errorf("nil path map should disable rewriting; got: %s", out)
	}
	if !strings.Contains(out, "<a") {
		t.Errorf("expected plain anchor, got: %s", out)
	}
}
