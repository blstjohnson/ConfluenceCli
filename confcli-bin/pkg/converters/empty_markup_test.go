package converters

import (
	"strings"
	"testing"
)

func TestEmptyMarkupCleanupRetainsSpacingAndFunctionalMetadata(t *testing.T) {
	protected := "<a id=\"target\"></a> <span id=\"section\"></span> ` <span></span> `\n\n```html\n<em></em>\n<!-- expand-start:  -->\n<!-- expand-end -->\n```\n\n<!-- confcli-code-params: eyJjb2xsYXBzZSI6InRydWUifQ== -->\n\n<!-- expand-start:  -->\n![Include](Other.md)\n<!-- expand-end -->\n\n<!-- info-start: Important title -->\n<!-- info-end -->\n"
	source := "word<em> </em>word<span></span><strong><span> </span></strong>end\n\n<!-- expand-start: Оглавление -->\n\n<!-- expand-end -->\n\n<!-- panel-start:  -->\n<!-- panel-end -->\n" + protected
	out := CleanEmptyMarkdownMarkup(source)
	if !strings.HasPrefix(out, "word word end\n") || !strings.HasSuffix(out, protected) || strings.Contains(out, "<!-- expand-start: Оглавление -->") || strings.Contains(out, "<!-- panel-start:  -->") {
		t.Fatal(out)
	}
	if CleanEmptyMarkdownMarkup(out) != out {
		t.Fatal("cleanup is not idempotent")
	}
}

func TestEmptyStorageWrappersDoNotExportMarkers(t *testing.T) {
	source := `<p>Before</p><ac:structured-macro ac:name="panel"><ac:rich-text-body><p><span> </span></p></ac:rich-text-body></ac:structured-macro><ac:structured-macro ac:name="expand"><ac:rich-text-body><ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-id="42" ri:content-title="Other"/></ac:link></ac:parameter></ac:structured-macro></ac:rich-text-body></ac:structured-macro><p>After</p>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{BaseURL: "https://wiki.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "https://wiki.example.com")
	if err != nil {
		t.Fatal(err)
	}
	md = restore(md)
	if strings.Contains(md, "panel-start") || !strings.Contains(md, "<!-- expand-start:  -->") || !strings.Contains(md, "![Other]") || !strings.Contains(md, "After") {
		t.Fatal(md)
	}
}

func TestBlankListSpacingCleanupPreservesHardBreaksAndCode(t *testing.T) {
	source := "1. Parent\n   \n   <ol type=\"a\"><li>Child</li></ol>\n\nHard break  \nnext\n\n```text\n   \n```\n"
	want := "1. Parent\n\n   <ol type=\"a\"><li>Child</li></ol>\n\nHard break  \nnext\n\n```text\n   \n```\n"
	if out := CleanEmptyMarkdownMarkup(source); out != want {
		t.Fatalf("lost list indentation, hard break or code whitespace: %q", out)
	}
}
