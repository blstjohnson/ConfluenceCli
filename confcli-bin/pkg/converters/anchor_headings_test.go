package converters

import (
	"strings"
	"testing"
)

func TestAnchorHeadingsAndRelativeLinks(t *testing.T) {
	source := `<p><ac:link ac:anchor="msgcheck"><ac:plain-text-link-body>ниже</ac:plain-text-link-body></ac:link> <a href="#msgcheck">ещё</a></p><h3><ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">msgcheck</ac:parameter></ac:structured-macro>RU.0.0.1 002 <strong>Первичная проверка входящих сообщений</strong></h3><p>Body</p><ac:link ac:anchor="remote"><ri:page ri:content-id="43"/><ac:plain-text-link-body>Remote</ac:plain-text-link-body></ac:link>`
	fetches := 0
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{
		PageID: 42, BaseURL: "https://wiki.example.com",
		FetchPage: func(id int) (RoundTripPage, error) {
			fetches++
			return RoundTripPage{Storage: `<h2><ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">remote</ac:parameter></ac:structured-macro>Remote API</h2>`}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "https://wiki.example.com")
	if err != nil {
		t.Fatal(err)
	}
	md = RewriteLinks(restore(md), &LinkRewriteConfig{ConfBaseURL: "wiki.example.com", PageMap: map[int]string{42: "Gateway.md", 43: "other/Target.md"}})
	for _, want := range []string{
		"[ниже](Gateway.md#RU.0.0.1%20002%20Первичная%20проверка%20входящих%20сообщений)",
		"[ещё](Gateway.md#RU.0.0.1%20002%20Первичная%20проверка%20входящих%20сообщений)",
		"[Remote](other/Target.md#Remote%20API)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in %s", want, md)
		}
	}
	if strings.Contains(md, "<a id=") || strings.Contains(md, "#msgcheck") || fetches != 1 {
		t.Fatalf("unexpected anchors/fetches %d: %s", fetches, md)
	}
}

func TestTableRuleLinksUseCleanTargetHeadings(t *testing.T) {
	target := `<h2>RU.01<ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">RU.01</ac:parameter></ac:structured-macro></h2><p>First rule</p><h2>RU.09<ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">RU.09</ac:parameter></ac:structured-macro></h2><p>Ninth rule</p>`
	options := RoundTripOptions{PageID: 1, BaseURL: "https://wiki.example.com", FetchPage: func(int) (RoundTripPage, error) {
		return RoundTripPage{Storage: target, Title: "Rules"}, nil
	}}
	source := `<table><tbody><tr><td><ac:link ac:anchor="RU.09"><ri:page ri:content-id="2"/><ac:plain-text-link-body>RU.09</ac:plain-text-link-body></ac:link></td><td><ac:link ac:anchor="RU.01"><ri:page ri:content-id="2"/><ac:plain-text-link-body>RU.01</ac:plain-text-link-body></ac:link></td></tr></tbody></table>`
	for _, tc := range []struct{ storage, want string }{{target, "## RU.01\n\nFirst rule\n\n## RU.09"}, {source, "[RU.09](Rules.md#RU.09)"}} {
		prepared, restore, err := PrepareRoundTripExport(tc.storage, "", options)
		if err != nil {
			t.Fatal(err)
		}
		out, err := StorageToMarkdownAdvanced(prepared, options.BaseURL)
		if err != nil {
			t.Fatal(err)
		}
		out = RewriteLinks(restore(out), &LinkRewriteConfig{ConfBaseURL: options.BaseURL, PageMap: map[int]string{2: "Rules.md"}})
		if !strings.Contains(out, tc.want) || strings.Contains(out, "<a ") {
			t.Fatal(out)
		}
		if tc.storage == source && !strings.Contains(out, "[RU.01](Rules.md#RU.01)") {
			t.Fatal(out)
		}
	}
}

func TestAnchorHeadingOwnership(t *testing.T) {
	index, err := AnchorHeadings(`<h1>First</h1><p><ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">section</ac:parameter></ac:structured-macro>Text</p><p><ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">next</ac:parameter></ac:structured-macro></p><h2>Second &amp; API</h2><ac:structured-macro ac:name="code"><ac:plain-text-body><![CDATA[<h1>Fake</h1>]]></ac:plain-text-body></ac:structured-macro>`)
	if err != nil {
		t.Fatal(err)
	}
	if index["section"] != "First" || index["next"] != "Second & API" {
		t.Fatal(index)
	}
}

func TestNamedAnchorRetainsDuplicateHeadingOccurrence(t *testing.T) {
	source := `<h2>Scenario</h2><h3>RU.0.0.3 Decryption</h3><p><a href="#msgdecrypt">below</a></p><h2>Common processes</h2><h3><ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">msgdecrypt</ac:parameter></ac:structured-macro>RU.0.0.3 Decryption</h3><p>Actual shared implementation</p>`
	view := `<h2 id="SamplePage-Scenario">Scenario</h2><h3 id="SamplePage-first">RU.0.0.3 Decryption</h3><h2 id="SamplePage-Commonprocesses">Common processes</h2><h3 id="SamplePage-second">RU.0.0.3 Decryption</h3>`
	root, err := parseStorage(source)
	if err != nil {
		t.Fatal(err)
	}
	index := pageAnchorIndex(root, view, "Sample Page")
	want := "Common processes#RU.0.0.3 Decryption"
	for _, alias := range []string{"msgdecrypt", "SamplePage-msgdecrypt", "SamplePage-msgdecryptRU.0.0.3Decryption", "SamplePage-second"} {
		if index[alias] != want {
			t.Errorf("%s points to %q, want %q", alias, index[alias], want)
		}
	}
	if index["SamplePage-first"] != "Scenario#RU.0.0.3 Decryption" {
		t.Fatal(index)
	}
	prepared, restore, err := PrepareRoundTripExport(source, view, RoundTripOptions{PageID: 42, PageTitle: "Sample Page", BaseURL: "https://wiki.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "https://wiki.example.com")
	if err != nil {
		t.Fatal(err)
	}
	md = RewriteLinks(restore(md), &LinkRewriteConfig{ConfBaseURL: "wiki.example.com", PageMap: map[int]string{42: "Gateway.md"}})
	if !strings.Contains(md, "[below](Gateway.md#Common%20processes#RU.0.0.3%20Decryption)") || strings.Contains(md, "<a id=") {
		t.Fatal(md)
	}
}

func TestHeadingFragmentsResolveExactOccurrence(t *testing.T) {
	headings := []headingLocation{{"Scenario", 2}, {"Check: message", 3}, {"Shared", 2}, {"Check message", 3}, {"Repeated", 2}, {"Repeated", 2}}
	fragments := headingFragments(headings)
	if fragments[3] != "Shared#Check message" || !strings.HasPrefix(fragments[5], "^confcli-") {
		t.Fatal(fragments)
	}
	for i, f := range fragments {
		if f != "" && !strings.HasPrefix(f, "^") && resolveHeadingPath(headings, f) != i {
			t.Errorf("heading %d: %q resolves elsewhere", i, f)
		}
	}
}

func TestHeadingAnchorPreparationIgnoresEmptyHeadingsAndCode(t *testing.T) {
	source := "#\n\n```md\n## Shared\n### Repeated ^example\n```\n\n## Scenario\n\n### Repeated\n\n## Shared\n\n### Repeated ^real-target\n"
	prepared, anchors := PrepareMarkdownHeadingAnchors([]byte(source))
	if !strings.Contains(prepared, "### Repeated ^example\n```") || anchors["^example"] != "" || anchors["^real-target"] != "real-target" || strings.Contains(prepared, "### Repeated ^real-target") {
		t.Fatal(prepared, anchors)
	}
}

func TestLegacyAnchorsRetainSpecificDuplicateHeadingOccurrence(t *testing.T) {
	source := "# First\n\n## Repeated<a id=\"first-target\"></a>\n\n# Second\n\n<a id=\"second-target\"></a>\n\n## Repeated\n"
	prepared, anchors := PrepareMarkdownHeadingAnchors([]byte(source))
	if prepared != source || anchors["First#Repeated"] != "first-target" || anchors["Second#Repeated"] != "second-target" {
		t.Fatal(prepared, anchors)
	}
	_, ambiguous := PrepareMarkdownHeadingAnchors([]byte("## A<a id=\"same\"></a>\n\n## B<a id=\"same\"></a>\n"))
	if ambiguous["A"] != "" || ambiguous["B"] != "" {
		t.Fatal(ambiguous)
	}
}

func TestNativeHeadingIDsAndUnambiguousLinkLabels(t *testing.T) {
	for _, tc := range []struct{ name, source, view, want string }{
		{"native-with-anchor", `<p><a href="#SamplePage-msgcheckAPI001Messagecheck">below</a></p><h3><ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">msgcheck</ac:parameter></ac:structured-macro>API 001 Message check</h3>`, "", "#API%20001%20Message%20check"},
		{"native-heading", `<p><a href="#SamplePage-API001Messagecheck">below</a></p><h3>API 001 Message check</h3>`, "", "#API%20001%20Message%20check"},
		{"rendered-id", `<p><a href="#SamplePage-legacy-format">below</a></p><h3>Current heading</h3>`, `<h3 id="SamplePage-legacy-format">Current heading</h3>`, "#Current%20heading"},
		{"exact-label", `<p><ac:link ac:anchor="old-missing-id"><ac:plain-text-link-body>Message structure</ac:plain-text-link-body></ac:link></p><h3>Message structure</h3>`, "", "#Message%20structure"},
		{"ambiguous-label", `<p><a href="#old-missing-id">Message structure</a></p><h3>Message structure</h3><h3>Message structure</h3>`, "", "#old-missing-id"},
		{"foreign-include", `<p><a href="#OtherPage-foreign">below</a></p><h3>Current heading</h3>`, `<h3 id="OtherPage-foreign">Current heading</h3>`, "#OtherPage-foreign"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepared, restore, err := PrepareRoundTripExport(tc.source, tc.view, RoundTripOptions{PageID: 42, PageTitle: "Sample Page", BaseURL: "https://wiki.example.com"})
			if err != nil {
				t.Fatal(err)
			}
			md, err := StorageToMarkdownAdvanced(prepared, "https://wiki.example.com")
			if err != nil {
				t.Fatal(err)
			}
			md = RewriteLinks(restore(md), &LinkRewriteConfig{ConfBaseURL: "wiki.example.com", PageMap: map[int]string{42: "actual-name.md"}})
			if !strings.Contains(md, "actual-name.md"+tc.want+")") {
				t.Fatal(md)
			}
		})
	}
}

func TestDrawioWithoutRenderedMacroIDKeepsPublishedRevision(t *testing.T) {
	source := `<p>Before</p><ac:structured-macro ac:name="drawio"><ac:parameter ac:name="diagramName">Load Balance</ac:parameter><ac:parameter ac:name="revision">3</ac:parameter></ac:structured-macro><p>After</p>`
	prepared, restore, err := PrepareRoundTripExport(source, `<img src="newest.png"/>`, RoundTripOptions{BaseURL: "https://wiki.example.com", PageID: 42})
	if err != nil {
		t.Fatal(err)
	}
	md, err := StorageToMarkdownAdvanced(prepared, "https://wiki.example.com")
	if err != nil {
		t.Fatal(err)
	}
	md = restore(md)
	if !strings.Contains(md, "![Load Balance](https://wiki.example.com/download/attachments/42/Load%20Balance.png?version=3)") || strings.Contains(md, "newest") || strings.Contains(md, "confcli-unrendered") {
		t.Fatal(md)
	}
	if !(strings.Index(md, "Before") < strings.Index(md, "![Load") && strings.Index(md, "![Load") < strings.Index(md, "After")) {
		t.Fatal(md)
	}
}
