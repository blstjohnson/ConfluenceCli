package converters

import (
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeInsideListsAndTablesDoesNotSwallowFollowingContent(t *testing.T) {
	source := `<ol><li><p>Example</p><ac:structured-macro ac:name="code"><ac:plain-text-body><![CDATA[first
second]]></ac:plain-text-body></ac:structured-macro><p>After list code</p></li></ol><table><tbody><tr><th>Example</th><th>Value</th></tr><tr><td>JSON</td><td><ac:structured-macro ac:name="code"><ac:parameter ac:name="language">json</ac:parameter><ac:plain-text-body><![CDATA[{"x": [1,
2], "pipe": "|"}]]></ac:plain-text-body></ac:structured-macro></td></tr></tbody></table><p>After table code</p>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	md = restore(md)
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader([]byte(md)))
	fences, tables := 0, 0
	ast.Walk(doc, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if enter {
			switch x := n.(type) {
			case *ast.FencedCodeBlock:
				fences++
				if string(x.Lines().Value([]byte(md))) != "first\nsecond\n" {
					t.Fatalf("list code changed: %q", x.Lines().Value([]byte(md)))
				}
			case *extast.Table:
				tables++
				if x.ChildCount() != 2 {
					t.Fatalf("table code broke rows: %s", md)
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if fences != 1 || tables != 1 || !strings.Contains(md, "&#10;") || !strings.Contains(md, "&#124;") || !strings.Contains(md, "After table code") {
		t.Fatalf("code/table lost: %s", md)
	}
}

func TestHeaderListsNestedTablesAndLiteralPipesKeepTheTable(t *testing.T) {
	source := `<table><tbody><tr><th>Name</th><th><ol><li>First</li><li>Second</li></ol></th></tr><tr><td>A|B</td><td><table><tbody><tr><td>Retry</td><td>3</td></tr><tr><td>Delay</td><td>10</td></tr></tbody></table></td></tr></tbody></table>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	md = restore(md)
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader([]byte(md)))
	tables := 0
	ast.Walk(doc, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if x, ok := n.(*extast.Table); enter && ok {
			tables++
			if len(x.Alignments) != 2 || x.ChildCount() != 2 {
				t.Fatalf("nested/header table corrupted: %s", md)
			}
		}
		return ast.WalkContinue, nil
	})
	if tables != 1 || !strings.Contains(md, `A\|B`) || !strings.Contains(md, "Retry — 3") || !strings.Contains(md, "Delay — 10") {
		t.Fatalf("table contents missing: %s", md)
	}
}

func TestRowspanDoesNotDropTrailingCellsAndHTMLPreKeepsBreaks(t *testing.T) {
	source := `<table><tbody><tr><th rowspan="2">Group</th><th>One</th><th>Two</th></tr><tr><td>A</td><td>B</td><td>Tail cell</td></tr></tbody></table><table><tr><th>Code</th></tr><tr><td><pre>first<br/>second</pre></td></tr></table>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	md = restore(md)
	if !strings.Contains(md, "Tail cell") || !strings.Contains(md, "first&#10;second") {
		t.Fatalf("table cell dropped or pre lines merged: %s", md)
	}
}

func TestRichLinkLabelsDoNotMergeAdjacentWords(t *testing.T) {
	source := `<p>Prepare <ac:link><ri:page ri:content-title="Setup"/><ac:link-body><p>required</p></ac:link-body></ac:link>infrastructure.</p>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{BaseURL: "https://wiki.test", ResolvePage: func(string, string) (int, error) { return 1, nil }})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "https://wiki.test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restore(md), ") infrastructure") {
		t.Fatalf("link word boundary lost: %s", restore(md))
	}
}

func TestPageIDReferenceUsesCollectedTitleWhenLabelIsEmpty(t *testing.T) {
	source := `<p><ac:link><ri:page ri:content-id="42"/><ac:link-body/></ac:link></p>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{BaseURL: "https://wiki.test", TitleForPage: func(id string) string {
		if id == "42" {
			return "Collected title"
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "https://wiki.test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restore(md), "[Collected title](https://wiki.test/pages/viewpage.action?pageId=42)") {
		t.Fatalf("title lost: %s", restore(md))
	}
}

func TestExcerptAndTooltipTargetsKeepTheirContentsAndAnchors(t *testing.T) {
	source := `<p>Outside</p><ac:structured-macro ac:name="excerpt"><ac:rich-text-body><p>Shared</p><ac:structured-macro ac:name="handy-tip"><ac:parameter ac:name="id">123</ac:parameter><ac:rich-text-body><p>Tip</p></ac:rich-text-body></ac:structured-macro></ac:rich-text-body></ac:structured-macro>`
	body, ok, err := StorageExcerptBody(source)
	if err != nil || !ok || strings.Contains(body, "Outside") {
		t.Fatalf("excerpt: %s, %v", body, err)
	}
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	got := restore(md)
	for _, want := range []string{`id="confcli-excerpt"`, `id="handytip-123"`, "Shared", "Tip"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing excerpt/tooltip: %s", got)
		}
	}
}

func TestRoundTripUnresolvedLinksKeepTextAndUserReferences(t *testing.T) {
	source := `<p>See <ac:link><ac:plain-text-link-body><![CDATA[AF.1.2]]></ac:plain-text-link-body></ac:link> and <ac:link><ri:user ri:username="Editor"/></ac:link>.</p>`
	var warnings []string
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{BaseURL: "https://wiki.test", Warn: func(s string) { warnings = append(warnings, s) }})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "https://wiki.test")
	if err != nil {
		t.Fatal(err)
	}
	if got := restore(md); !strings.Contains(got, "AF.1.2") || !strings.Contains(got, "[Editor](https://wiki.test/display/~Editor)") || len(warnings) != 1 {
		t.Fatalf("lost unresolved/user link: %s; %v", got, warnings)
	}
}

func TestRoundTripContainersAndRawMarkdownDoNotLeakSettings(t *testing.T) {
	source := `<ac:structured-macro ac:name="note"><ac:parameter ac:name="title">Caution</ac:parameter><ac:rich-text-body><p>Read this</p></ac:rich-text-body></ac:structured-macro><ac:structured-macro ac:name="table-filter"><ac:parameter ac:name="width">99999</ac:parameter><ac:rich-text-body><p>Table body</p></ac:rich-text-body></ac:structured-macro><ac:structured-macro ac:name="markdown"><ac:plain-text-body><![CDATA[**Original** [link](file.md)]]></ac:plain-text-body></ac:structured-macro>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	got := restore(md)
	for _, want := range []string{"<!-- note-start: Caution -->", "Read this", "<!-- note-end -->", "Table body", "**Original** [link](file.md)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lost macro body: %s", got)
		}
	}
	if strings.Contains(got, "99999") {
		t.Fatalf("settings leaked: %s", got)
	}
}

func TestRoundTripRenderedMacroAndCrossSpace(t *testing.T) {
	storage := `<p>Before</p><ac:structured-macro ac:name="drawio" ac:macro-id="diagram-1"><ac:parameter ac:name="filename">a.drawio</ac:parameter></ac:structured-macro><ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:space-key="OTHER" ri:content-title="Shared" /></ac:link></ac:parameter></ac:structured-macro><p>After</p>`
	prepared, restore, err := PrepareRoundTripExport(storage, `<div data-macro-id="diagram-1"><p>Rendered diagram caption</p></div>`, RoundTripOptions{BaseURL: "https://wiki.test", SpaceKey: "DEV", ResolvePage: func(space, title string) (int, error) {
		if space != "OTHER" || title != "Shared" {
			t.Fatalf("unexpected ref %s/%s", space, title)
		}
		return 5, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "https://wiki.test")
	if err != nil {
		t.Fatal(err)
	}
	md = restore(md)
	if !strings.Contains(md, "Rendered diagram caption") || !strings.Contains(md, "![Shared](https://wiki.test/pages/viewpage.action?pageId=5)") {
		t.Fatalf("hybrid output: %s", md)
	}
}

func TestRoundTripRejectsUnresolvedAndMalformedStorage(t *testing.T) {
	for _, storage := range []string{`<p>bad</div>`, `<ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-title="Missing"/></ac:link></ac:parameter></ac:structured-macro>`} {
		_, _, err := PrepareRoundTripExport(storage, "", RoundTripOptions{ResolvePage: func(string, string) (int, error) { return 0, fmt.Errorf("not found") }})
		if err == nil {
			t.Fatalf("expected preservation failure: %s", storage)
		}
	}
}

func TestRoundTripKeepsMissingPageLinksButRejectsMissingIncludes(t *testing.T) {
	var warnings []string
	opts := RoundTripOptions{
		BaseURL: "https://wiki.test", SpaceKey: "DEV",
		ResolvePage: func(string, string) (int, error) { return 0, nil },
		Warn:        func(s string) { warnings = append(warnings, s) },
	}
	storage := `<p>Before <ac:link ac:anchor="api"><ri:page ri:content-title="Missing page"/><ac:plain-text-link-body><![CDATA[API]]></ac:plain-text-link-body></ac:link> after</p><p><a href="/display/DEV/Missing+page#api">Original</a></p>`
	prepared, restore, err := PrepareRoundTripExport(storage, "", opts)
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := StorageToMarkdownAdvanced(prepared, opts.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	markdown = restore(markdown)
	for _, label := range []string{"API", "Original"} {
		if !strings.Contains(markdown, "["+label+"](https://wiki.test/display/DEV/Missing+page#api)") {
			t.Fatalf("missing title link or fragment: %s", markdown)
		}
	}
	if len(warnings) != 2 || !strings.Contains(markdown, "Before") || !strings.Contains(markdown, "after") {
		t.Fatalf("missing content or warnings: %s, %v", markdown, warnings)
	}
	_, _, err = PrepareRoundTripExport(`<ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-title="Missing"/></ac:link></ac:parameter></ac:structured-macro>`, "", opts)
	if err == nil {
		t.Fatal("missing include must fail rather than silently lose its content")
	}
}

func TestRewriteLinksKeepsFragments(t *testing.T) {
	cfg := &LinkRewriteConfig{PageMap: map[int]string{42: "docs/Other page.md"}, CurrentPageDir: "docs/sub", ConfBaseURL: "wiki.test"}
	for _, source := range []string{"[API](https://wiki.test/pages/viewpage.action?pageId=42#api)", "[API](https://wiki.test/pages/viewpage.action?pageId=42%23api)"} {
		if got := RewriteLinks(source, cfg); got != "[API](../Other%20page.md#api)" {
			t.Errorf("rewritten fragment: %s", got)
		}
	}
	if got := RewriteLinks("[External](https://wiki.test/pages/viewpage.action?pageId=99#api)", cfg); !strings.Contains(got, "pageId=99#api") {
		t.Errorf("external fragment lost: %s", got)
	}
}

func TestRoundTripGitFilePathsRelativeToExportedPage(t *testing.T) {
	repo := t.TempDir()
	storage := `<ac:structured-macro ac:name="view-git-file"><ac:parameter ac:name="path">docs/diagrams/a.puml</ac:parameter><ac:parameter ac:name="renderpuml">true</ac:parameter></ac:structured-macro>`
	prepared, restore, err := PrepareRoundTripExport(storage, "", RoundTripOptions{LocalRepoPath: repo, CurrentPageDir: filepath.Join(repo, "docs", "requirements", "API")})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := restore(md); !strings.Contains(got, "[docs/diagrams/a.puml](../../diagrams/a.puml)") {
		t.Fatalf("repository path not relative to page: %s", got)
	}
}

func TestUnrenderedMacroParametersAreRetainedWithoutLeakingIntoText(t *testing.T) {
	var warnings []string
	source := `<ac:structured-macro ac:name="scroll-ignore"><ac:parameter ac:name="scroll-pdf">true</ac:parameter><ac:rich-text-body><p>Body</p></ac:rich-text-body></ac:structured-macro><ac:structured-macro ac:name="drawio"><ac:parameter ac:name="diagramName">scheme_ha</ac:parameter><ac:parameter ac:name="border">true</ac:parameter></ac:structured-macro>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{Warn: func(s string) { warnings = append(warnings, s) }})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	markdown = restore(markdown)
	if len(warnings) != 1 || !strings.Contains(markdown, `<!-- confcli-unrendered-macro: {"name":"drawio"`) || !strings.Contains(markdown, `"diagramName":"scheme_ha"`) || !strings.HasPrefix(markdown, "Body") || strings.Contains(markdown, "truetrue") {
		t.Fatalf("macro settings leaked or were silently discarded: %s, warnings=%v", markdown, warnings)
	}
}

func TestRoundTripAttachmentImagesAndDownloadLinks(t *testing.T) {
	source := `<p><ac:image ac:alt="Diagram"><ri:attachment ri:filename="image test.png" /></ac:image></p><p><ac:link><ri:attachment ri:filename="file.svg"><ri:page ri:content-title="Owner" /></ri:attachment><ac:plain-text-link-body><![CDATA[Download]]></ac:plain-text-link-body></ac:link></p>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{BaseURL: "https://wiki.test", PageID: 10, ResolvePage: func(string, string) (int, error) { return 20, nil }})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := StorageToMarkdownAdvanced(prepared, "https://wiki.test")
	if err != nil {
		t.Fatal(err)
	}
	markdown = restore(markdown)
	for _, want := range []string{"![Diagram](https://wiki.test/download/attachments/10/image%20test.png)", "[Download](https://wiki.test/download/attachments/20/file.svg)"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("attachment reference lost: %s", markdown)
		}
	}
}

func TestRoundTripGitFileWithRelativeExportDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	source := `<ac:structured-macro ac:name="view-git-file"><ac:parameter ac:name="path">docs/a.puml</ac:parameter></ac:structured-macro>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{LocalRepoPath: filepath.Join(cwd, "repo"), CurrentPageDir: filepath.Join("out", "page")})
	if err != nil {
		t.Fatal(err)
	}
	markdown, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restore(markdown), "../../repo/docs/a.puml") {
		t.Fatalf("relative output path not resolved: %s", restore(markdown))
	}
}

func TestExportLinkRewriteKeepsAttachmentsUnknownLinksAndCodeExamples(t *testing.T) {
	cfg := &LinkRewriteConfig{PageMap: map[int]string{42: "Other.md"}, ConfBaseURL: "wiki.test"}
	code := "`[Code](https://wiki.test/pages/viewpage.action?pageId=42)`\n\n````md\n[Code](https://wiki.test/pages/viewpage.action?pageId=42)\n```\n````"
	source := "![Image](https://wiki.test/download/attachments/7/check.png)\n\n[Title](https://wiki.test/display/DEV/Missing#api)\n\n" + code + "\n\n[Real](https://wiki.test/pages/viewpage.action?pageId=42)"
	got := RewriteLinks(source, cfg)
	for _, want := range []string{"![Image](https://wiki.test/download/attachments/7/check.png)", "[Title](https://wiki.test/display/DEV/Missing#api)", code, "[Real](Other.md)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("link or code reference discarded: %s", got)
		}
	}
}
