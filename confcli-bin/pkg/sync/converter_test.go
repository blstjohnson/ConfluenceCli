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
		"a.md":         {Data: []byte("a")},
		"docs/b.md":    {Data: []byte("b")},
		"drafts/c.md":  {Data: []byte("c")},
		"docs/_x.md":   {Data: []byte("x")},
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
		"src.md":    {Data: []byte("see [other](other.md)")},
		"other.md":  {Data: []byte("# other")},
	}
	profile := &transforms.ImportProfile{Kind: "import"}
	pm, err := BuildPathMap(profile, fsys)
	if err != nil {
		t.Fatalf("BuildPathMap: %v", err)
	}

	conv := NewMarkdownConverter(pm, nil)
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
	conv := NewMarkdownConverter(pm, nil)
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

func TestNewMarkdownConverter_NilPathMapPassesLinksThrough(t *testing.T) {
	conv := NewMarkdownConverter(nil, nil)
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
