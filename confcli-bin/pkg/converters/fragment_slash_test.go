package converters

import (
	"strings"
	"testing"
)

func TestExportHeadingSlashRemainsLiteralForObsidian(t *testing.T) {
	storage := `<h2>FLOW.1.1/1.2 Return HTTP 500, retry</h2><p><ac:link ac:anchor="retry"><ac:plain-text-link-body><![CDATA[Retry]]></ac:plain-text-link-body></ac:link></p>`
	// Use a real named target, as with an Anchor macro on a published page.
	storage = strings.Replace(storage, `<h2>`, `<h2><ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">retry</ac:parameter></ac:structured-macro>`, 1)
	opts := RoundTripOptions{BaseURL: "https://wiki.test", PageID: 42}
	prepared, restore, err := PrepareRoundTripExport(storage, "", opts)
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := StorageToMarkdownAdvanced(prepared, opts.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	markdown = RewriteLinks(restore(markdown), &LinkRewriteConfig{
		PageMap: map[int]string{42: "docs/Gateway.md"}, CurrentPageDir: "docs", ConfBaseURL: "wiki.test",
	})
	if !strings.Contains(markdown, "Gateway.md#FLOW.1.1/1.2%20Return%20HTTP%20500,%20retry") || strings.Contains(strings.ToLower(markdown), "%2f") || strings.Contains(strings.ToLower(markdown), "%2c") {
		t.Fatalf("Obsidian needs a literal slash in the heading fragment: %s", markdown)
	}
}

func TestRewriteExportedFragmentDecodesSlashWithoutEncodingItAgain(t *testing.T) {
	cfg := &LinkRewriteConfig{PageMap: map[int]string{42: "docs/Gateway.md"}, CurrentPageDir: "docs", ConfBaseURL: "wiki.test"}
	for _, tc := range []struct{ anchor, want string }{
		{"FLOW.1.1/1.2%20Return%20HTTP%20500", "FLOW.1.1/1.2%20Return%20HTTP%20500"},
		{"FLOW.1.1%2F1.2%20Return%20HTTP%20500", "FLOW.1.1/1.2%20Return%20HTTP%20500"},
		{"FLOW.1.1%2F1.2%20Return%20HTTP%20500%2C%20retry", "FLOW.1.1/1.2%20Return%20HTTP%20500,%20retry"},
	} {
		got := RewriteLinks("[Retry](https://wiki.test/pages/viewpage.action?pageId=42#"+tc.anchor+")", cfg)
		want := "[Retry](Gateway.md#" + tc.want + ")"
		if got != want {
			t.Errorf("fragment must retain a literal slash: got %q, want %q", got, want)
		}
	}
}
