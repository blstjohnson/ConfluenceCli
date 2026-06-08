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

func TestNewMarkdownConverter_HyphenatedTagsEscaped(t *testing.T) {
	// Hyphenated names are valid HTML5 custom-element tags, so goldmark's
	// WithUnsafe renderer passes them through; Confluence then rejects the
	// page ("expected </base-dn>"). They're really field placeholders in
	// the source, so escape them. Covers <base-dn>, <digest-value>, <Y-X>.
	src := []byte("host is <base-dn> and digest <digest-value>, range <Y-X>\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	for _, raw := range []string{"<base-dn>", "<digest-value>", "<Y-X>"} {
		if strings.Contains(out, raw) {
			t.Errorf("hyphenated placeholder %q not escaped: %s", raw, out)
		}
	}
	if !strings.Contains(out, "&lt;base-dn&gt;") {
		t.Errorf("expected escaped &lt;base-dn&gt;: %s", out)
	}
}

func TestNewMarkdownConverter_BareAttributeTagsEscaped(t *testing.T) {
	// "<TCP Port>" parses as a valid HTML5 tag with a boolean attribute,
	// so goldmark keeps it; Confluence wants attr="value" and fails with
	// "expected '='". It's placeholder text — escape the whole token.
	src := []byte("address [<TCP Port>] and <xrate service url>\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if strings.Contains(out, "<TCP Port>") || strings.Contains(out, "<xrate service url>") {
		t.Errorf("bare-attribute placeholder not escaped: %s", out)
	}
	if !strings.Contains(out, "&lt;TCP Port&gt;") {
		t.Errorf("expected escaped &lt;TCP Port&gt;: %s", out)
	}
}

func TestNewMarkdownConverter_AttributedKnownTagsPreserved(t *testing.T) {
	// Real HTML with attributes (a known tag) must still pass through —
	// the broadened escaper keys off the tag name, not the presence of
	// attributes.
	src := []byte("see <a href=\"https://x/y\">link</a> and <td colspan=\"2\">c</td>\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, `<a href="https://x/y">`) || !strings.Contains(out, `<td colspan="2">`) {
		t.Errorf("attributed known tags must survive: %s", out)
	}
}

func TestNewMarkdownConverter_StrayAmpersandEscaped(t *testing.T) {
	// A literal "<" closing a code fence with glued text (```text) leaves
	// goldmark's fences mismatched, spilling raw text with unescaped "&"
	// into the output. The safety net must escape it. We reproduce the
	// stray "&" directly via inline raw HTML, which WithUnsafe passes
	// through verbatim.
	src := []byte("link: <a href=\"/p?x=1&y=2\">t</a>\nbare amp Tom & Jerry\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	// No '&' should remain that isn't part of a valid entity.
	for i := 0; i < len(out); i++ {
		if out[i] == '&' && !validEntityRe.MatchString(out[i:]) {
			t.Errorf("stray ampersand survived at %d: %s", i, out[i:min(i+12, len(out))])
		}
	}
}

func TestNewMarkdownConverter_StrayLessThanEscaped(t *testing.T) {
	// "<--" (a sequence-diagram arrow spilled out of a desynced code
	// fence) is "<" followed by non-markup; it must be escaped so the
	// parser doesn't choke on "content after '<'". Delivered via raw HTML
	// so it reaches the output unescaped, mimicking the spill.
	src := []byte("flow: <span><--</span>\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if strings.Contains(out, "<--") {
		t.Errorf("stray '<--' not escaped: %s", out)
	}
	if !strings.Contains(out, "&lt;--") {
		t.Errorf("expected escaped &lt;--: %s", out)
	}
	// The real <span> tags around it must survive.
	if !strings.Contains(out, "<span>") || !strings.Contains(out, "</span>") {
		t.Errorf("surrounding real tags must survive: %s", out)
	}
}

func TestNewMarkdownConverter_StrayMarkupSkipsCDATA(t *testing.T) {
	// Stray "&" / "<--" inside a code block (CDATA) is legitimate sample
	// text and must pass through verbatim.
	src := []byte("```text\na & b and x <-- y\n```\n")
	conv := NewMarkdownConverter(nil, nil, nil, nil)
	out, err := conv(context.Background(), src, "t.md")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(out, "a & b and x <-- y") {
		t.Errorf("CDATA content was altered; got: %s", out)
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
