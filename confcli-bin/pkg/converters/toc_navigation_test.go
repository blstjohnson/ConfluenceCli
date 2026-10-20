package converters

import (
	"strings"
	"testing"
)

func TestTOCNavigationMovesToLeadingExpandWithoutLosingContent(t *testing.T) {
	toc := `<ac:structured-macro ac:name="toc"><ac:parameter ac:name="maxLevel">3</ac:parameter></ac:structured-macro>`
	empty := `<ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">Оглавление</ac:parameter><ac:rich-text-body><p><br/></p></ac:rich-text-body></ac:structured-macro>`
	content := `<h1>Start</h1><ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">Оглавление</ac:parameter><ac:rich-text-body><p>Keep these notes</p></ac:rich-text-body></ac:structured-macro><h2>End</h2>`
	out, err := WithLeadingTableOfContents(content+empty+toc+toc, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, `<ac:structured-macro ac:name="expand"`) || strings.Count(out, `ac:name="toc"`) != 1 || !strings.Contains(out, `<ac:rich-text-body>`+toc+`</ac:rich-text-body>`) || !strings.HasSuffix(out, content) {
		t.Fatal(out)
	}
	if again, err := WithLeadingTableOfContents(out, true); err != nil || again != out {
		t.Fatal("TOC normalization is not idempotent", again, err)
	}
}

func TestTOCExportDropsNavigationWrappersButKeepsAuthoredExpand(t *testing.T) {
	for _, inner := range []string{"", `<p><br/></p>`, `<ac:structured-macro ac:name="toc"/>`} {
		t.Run(inner, func(t *testing.T) {
			nav := `<ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">Оглавление</ac:parameter><ac:rich-text-body>` + inner + `</ac:rich-text-body></ac:structured-macro>`
			authored := `<ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">Оглавление</ac:parameter><ac:rich-text-body><p>Actual notes</p></ac:rich-text-body></ac:structured-macro>`
			prepared, restore, err := PrepareRoundTripExport(nav+`<h1>Body</h1>`+authored, "", RoundTripOptions{})
			if err != nil {
				t.Fatal(err)
			}
			md, err := StorageToMarkdownAdvanced(prepared, "")
			if err != nil {
				t.Fatal(err)
			}
			md = restore(md)
			if strings.Count(md, "<!-- expand-start: Оглавление -->") != 1 || !strings.Contains(md, "Actual notes") || !strings.Contains(md, "# Body") {
				t.Fatal(md)
			}
		})
	}
}

func TestTOCRemovalBeforeExportDoesNotLeaveEmptyExpand(t *testing.T) {
	source := `<ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">Оглавление</ac:parameter><ac:rich-text-body><ac:structured-macro ac:name="toc"/></ac:rich-text-body></ac:structured-macro><h1>Body</h1>`
	stripped, err := StripStorageMacros(source, func(name string) bool { return name == "toc" }, true)
	if err != nil {
		t.Fatal(err)
	}
	prepared, restore, err := PrepareRoundTripExport(stripped, "", RoundTripOptions{})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	if md = restore(md); strings.Contains(md, "expand-") || strings.Contains(md, "Оглавление") || !strings.Contains(md, "# Body") {
		t.Fatal(md)
	}
}
