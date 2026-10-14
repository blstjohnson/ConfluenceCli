package sync

import (
	"context"
	"encoding/xml"
	"io"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"confcli/pkg/converters"
	"confcli/pkg/transforms"
)

func assertStorageXML(t *testing.T, storage string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(`<root xmlns:ac="ac" xmlns:ri="ri">` + storage + `</root>`))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("invalid storage XML: %v\n%s", err, storage)
		}
	}
}

func TestDocumentationRoundTrip(t *testing.T) {
	source := `<p>Before</p><ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">Оглавление &amp; API</ac:parameter><ac:rich-text-body>
<p>Intro</p><ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-title="Other" /></ac:link></ac:parameter></ac:structured-macro>
<ac:structured-macro ac:name="panel"><ac:parameter ac:name="title">Inner</ac:parameter><ac:parameter ac:name="borderColor">red</ac:parameter><ac:rich-text-body>
<ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">api-anchor</ac:parameter></ac:structured-macro>
<p><ac:link ac:anchor="api-anchor"><ac:plain-text-link-body>Local API</ac:plain-text-link-body></ac:link></p>
<table><tbody><tr><th>Link</th></tr><tr><td><ac:link ac:anchor="api-anchor"><ri:page ri:content-title="Other"/><ac:plain-text-link-body><![CDATA[API]]></ac:plain-text-link-body></ac:link></td></tr></tbody></table>
<ac:structured-macro ac:name="plantuml"><ac:plain-text-body><![CDATA[@startuml
Alice -> Bob: <test> & check
@enduml]]></ac:plain-text-body></ac:structured-macro>
</ac:rich-text-body></ac:structured-macro>
</ac:rich-text-body></ac:structured-macro><p>After</p>`
	prepared, restore, err := converters.PrepareRoundTripExport(source, `<p>Already rendered include content must not be duplicated</p>`, converters.RoundTripOptions{
		BaseURL: "https://wiki.example.com", SpaceKey: "DEV",
		ResolvePage: func(space, title string) (int, error) {
			if space != "DEV" || title != "Other" {
				t.Fatalf("unexpected resolution: %s %s", space, title)
			}
			return 42, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := converters.StorageToMarkdownAdvanced(prepared, "https://wiki.example.com")
	if err != nil {
		t.Fatal(err)
	}
	markdown = converters.RewriteLinks(restore(markdown), &converters.LinkRewriteConfig{PageMap: map[int]string{42: "Other page.md"}, ConfBaseURL: "wiki.example.com"})
	if !strings.Contains(markdown, "```plantuml\n@startuml\nAlice -> Bob: <test> & check\n@enduml\n```") || !strings.Contains(markdown, "<!-- expand-start: Оглавление & API -->\n") {
		t.Fatalf("macro boundaries or PlantUML line breaks lost:\n%s", markdown)
	}
	for _, want := range []string{"<!-- expand-start: Оглавление & API -->", "<!-- expand-end -->", "![Other](Other%20page.md)", "[API](Other%20page.md#api-anchor)", "[Local API](#api-anchor)", `<a id="api-anchor"></a>`, "```plantuml", "Alice -> Bob: <test> & check"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("missing %q in markdown:\n%s", want, markdown)
		}
	}
	profile, err := transforms.LoadImportProfile([]byte("kind: import\ntree:\n  include_page_macro: true\ntransforms:\n  - type: restore_macro\n"))
	if err != nil {
		t.Fatal(err)
	}
	convert, err := NewProfileMarkdownConverter(profile, map[string]transforms.PageRef{"Other page.md": {Title: "Other"}}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := convert(context.Background(), []byte(markdown), "source.md")
	if err != nil {
		t.Fatal(err)
	}
	assertStorageXML(t, storage)
	for _, want := range []string{`ac:name="expand"`, `ac:name="panel"`, `ac:name="borderColor">red`, `ac:name="include"`, `ri:content-title="Other"`, `ac:anchor="api-anchor"`, `<ac:link-body>Local API</ac:link-body>`, `ac:name="anchor"`, `ac:name="plantuml"`, `<table>`, `<ac:rich-text-body>`, `Alice -> Bob: <test> & check`} {
		if !strings.Contains(storage, want) {
			t.Errorf("missing %q in storage:\n%s", want, storage)
		}
	}
	if strings.Count(storage, `ac:name="include"`) != 1 || strings.Contains(storage, "confcli-restore") || strings.Contains(storage, "CONFCLIRT") {
		t.Errorf("duplicate/missing restoration:\n%s", storage)
	}
	before, expand, include, after := strings.Index(storage, "Before"), strings.Index(storage, `ac:name="expand"`), strings.Index(storage, `ac:name="include"`), strings.Index(storage, "After")
	if !(before < expand && expand < include && include < after) {
		t.Errorf("document order lost: %s", storage)
	}
}

func TestImportPipelineErrorsAndCodeExamples(t *testing.T) {
	profile, err := transforms.LoadImportProfile([]byte("kind: import\ntransforms:\n  - type: restore_macro\n"))
	if err != nil {
		t.Fatal(err)
	}
	convert, err := NewProfileMarkdownConverter(profile, map[string]transforms.PageRef{"other.md": {Title: "Other"}}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"<!-- expand-start: A -->\ntext", "<!-- expand-end -->", "<!-- expand-start: A -->\n<!-- panel-end -->"} {
		if _, err := convert(context.Background(), []byte(source), "p.md"); err == nil {
			t.Errorf("expected malformed marker error for %q", source)
		}
	}
	source := "`![Other](other.md)`\n\n````md\n<!-- expand-start: Example -->\n```\n![Other](other.md)\n<!-- expand-end -->\n````\n\n<!-- expand-start: Real -->\n![Other](other.md)\n<!-- expand-end -->"
	storage, err := convert(context.Background(), []byte(source), "p.md")
	if err != nil {
		t.Fatal(err)
	}
	assertStorageXML(t, storage)
	if strings.Count(storage, `ac:name="include"`) != 1 || strings.Count(storage, `ac:name="expand"`) != 1 || !strings.Contains(storage, "<!-- expand-start: Example -->") {
		t.Fatalf("code example changed or real macro missing: %s", storage)
	}
}

func TestTFSFileURLsInTables(t *testing.T) {
	profile, err := transforms.LoadImportProfile([]byte("kind: import\ngit_files:\n  mode: url\n  url: https://tfs.example.com/Collection/Project/_git/Repo\n"))
	if err != nil {
		t.Fatal(err)
	}
	rewriter := &transforms.RewriteGitFileLinks{Mode: profile.GitFiles.Mode, URL: profile.GitFiles.URL, Branch: "refs/remotes/origin/feature/docs", SyncRootRelRepo: "docs/requirements"}
	convert, err := NewProfileMarkdownConverter(profile, nil, nil, rewriter, nil)
	if err != nil {
		t.Fatal(err)
	}
	source := "| File |\n| --- |\n| [Schema](../../schema/API%20test.json) |\n\n[Config](config.yaml)\n![image](image.png)"
	storage, err := convert(context.Background(), []byte(source), "p.md")
	if err != nil {
		t.Fatal(err)
	}
	assertStorageXML(t, storage)
	if !strings.Contains(storage, "path=%2Fschema%2FAPI+test.json") || !strings.Contains(storage, "version="+url.QueryEscape("GBfeature/docs")) || !strings.Contains(storage, `<img src="image.png"`) || strings.Contains(storage, `ac:name="view-git-file"`) {
		t.Fatalf("URL mode output: %s", storage)
	}
}

func TestImportProfileRestoresNestedMacros(t *testing.T) {
	data, err := os.ReadFile("testdata/roundtrip-import.yaml")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := transforms.LoadImportProfile(data)
	if err != nil {
		t.Fatal(err)
	}
	convert, err := NewProfileMarkdownConverter(profile, map[string]transforms.PageRef{"other.md": {Title: "Other"}}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := convert(context.Background(), []byte("<!-- expand-start: Details -->\n\n![Other](other.md)\n\n**Body**\n\n<!-- expand-end -->"), "p.md")
	if err != nil {
		t.Fatal(err)
	}
	assertStorageXML(t, storage)
	if !strings.Contains(storage, `ac:name="expand"`) || !strings.Contains(storage, `ac:name="include"`) || !strings.Contains(storage, `<strong>Body</strong>`) {
		t.Fatalf("import profile was not applied: %s", storage)
	}
}

func TestTableInExpandedListItemKeepsFollowingSteps(t *testing.T) {
	source := `<ol start="6"><li>Check request</li><li><p>Add details:</p><ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">Details</ac:parameter><ac:rich-text-body><table><tr><th>Field</th><th>Value</th></tr><tr><td>bankId</td><td>Identifier</td></tr></table></ac:rich-text-body></ac:structured-macro></li><li>Find receiver<ul><li>Use identifier</li></ul></li><li>Send response</li></ol>`
	prepared, restore, err := converters.PrepareRoundTripExport(source, "", converters.RoundTripOptions{})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := converters.StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := transforms.LoadImportProfile([]byte("kind: import\ntransforms:\n  - type: restore_macro\n"))
	if err != nil {
		t.Fatal(err)
	}
	convert, err := NewProfileMarkdownConverter(profile, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := convert(context.Background(), []byte(restore(markdown)), "source.md")
	if err != nil {
		t.Fatal(err)
	}
	assertStorageXML(t, storage)
	doc, err := html.Parse(strings.NewReader(storage))
	if err != nil {
		t.Fatal(err)
	}
	rows, listItems, expands := 0, 0, 0
	var walk func(*html.Node, bool, bool)
	walk = func(n *html.Node, inTable, inExpand bool) {
		if n.Data == "ac:structured-macro" {
			for _, a := range n.Attr {
				if a.Key == "ac:name" && a.Val == "expand" {
					inExpand = true
					expands++
				}
			}
		}
		if n.Data == "table" {
			inTable = true
			if !inExpand {
				t.Fatalf("table escaped its Expand: %s", storage)
			}
		}
		if n.Data == "tr" {
			rows++
		}
		if n.Data == "li" {
			listItems++
		}
		if n.Type == html.TextNode && (strings.Contains(n.Data, "Find receiver") || strings.Contains(n.Data, "Send response")) && (inTable || inExpand) {
			t.Fatalf("following step swallowed by table or Expand: %s", storage)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inTable, inExpand)
		}
	}
	walk(doc, false, false)
	if rows != 2 || listItems != 5 || expands != 1 || !strings.Contains(storage, `<ol start="6">`) {
		t.Fatalf("list/table structure changed: %s", storage)
	}
}

func TestFileRewritersPreserveCodeExamples(t *testing.T) {
	r := &transforms.RewriteGitFileLinks{Mode: "url", URL: "https://tfs.test/C/P/_git/R", Branch: "main"}
	p := &transforms.RewritePlantUMLLinks{Macro: "view-git-file", Parameters: map[string]string{"path": "{path}"}}
	convert := NewMarkdownConverter(nil, p, r, nil)
	source := "`[File](config.yaml)`\n\n````md\n[Diagram](diagram.puml)\n```\n[File](config.yaml)\n````\n\n[File](config.yaml)\n[Diagram](diagram.puml)"
	storage, err := convert(context.Background(), []byte(source), "p.md")
	if err != nil {
		t.Fatal(err)
	}
	assertStorageXML(t, storage)
	if strings.Count(storage, `ac:name="view-git-file"`) != 1 || strings.Count(storage, "https://tfs.test/") != 1 || !strings.Contains(storage, "[Diagram](diagram.puml)") || !strings.Contains(storage, "<code>[File](config.yaml)</code>") {
		t.Fatalf("code examples rewritten: %s", storage)
	}
}

func TestPlantUMLRewriterLeavesExternalAndAbsoluteURLs(t *testing.T) {
	p := &transforms.RewritePlantUMLLinks{Macro: "view-git-file", Parameters: map[string]string{"path": "{path}"}}
	convert := NewMarkdownConverter(nil, p, nil, nil)
	storage, err := convert(context.Background(), []byte("[External](https://example.com/a.puml)\n\n[Absolute](/download/a.puml)\n\n[Local](diagram.puml)"), "p.md")
	if err != nil {
		t.Fatal(err)
	}
	assertStorageXML(t, storage)
	if strings.Count(storage, `ac:name="view-git-file"`) != 1 || !strings.Contains(storage, `href="https://example.com/a.puml"`) || !strings.Contains(storage, `href="/download/a.puml"`) {
		t.Fatalf("external PlantUML reference corrupted: %s", storage)
	}
}
