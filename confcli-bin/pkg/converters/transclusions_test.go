package converters

import (
	"strings"
	"testing"

	"confcli/pkg/converters/md"
)

func TestWholePageTransclusionsAndOwnerContext(t *testing.T) {
	pages := map[int]RoundTripPage{
		2: {PageID: 2, SpaceKey: "API", Title: "External", Storage: `<p>Full external text</p><ac:image><ri:attachment ri:filename="picture.png"/></ac:image><ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-title="Nested"/></ac:link></ac:parameter></ac:structured-macro>`},
		3: {PageID: 3, SpaceKey: "API", Title: "Nested", Storage: "<p>Nested text</p>"},
		4: {PageID: 4, SpaceKey: "DOCS", Title: "Excerpt source", Storage: `<p>Before excerpt</p><ac:structured-macro ac:name="excerpt"><ac:rich-text-body><p>Only excerpt text</p></ac:rich-text-body></ac:structured-macro><p>After excerpt</p>`},
	}
	opts := RoundTripOptions{BaseURL: "https://wiki.test", SpaceKey: "DOCS", PageID: 1, InlineTransclusions: true,
		ResolvePage: func(space, title string) (int, error) {
			for id, p := range pages {
				if p.SpaceKey == space && p.Title == title {
					return id, nil
				}
			}
			t.Fatalf("wrong context: %s/%s", space, title)
			return 0, nil
		},
		FetchPage: func(id int) (RoundTripPage, error) { return pages[id], nil },
	}
	storage := `<p>Page beginning</p><ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:space-key="API" ri:content-title="External"/></ac:link></ac:parameter></ac:structured-macro><p>Between</p><ac:structured-macro ac:name="excerpt-include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-title="Excerpt source"/></ac:link></ac:parameter><ac:parameter ac:name="nopanel">true</ac:parameter></ac:structured-macro><p>Page ending</p>`
	prepared, restore, err := PrepareRoundTripExport(storage, "", opts)
	if err != nil {
		t.Fatal(err)
	}
	output, err := StorageToMarkdownAdvanced(prepared, opts.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	output, err = SealTransclusions(restore(output))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Page beginning", "Full external text", "Nested text", "Only excerpt text", "Page ending", "/download/attachments/2/picture.png"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q: %s", want, output)
		}
	}
	for _, unwanted := range []string{"Before excerpt", "After excerpt", "_excerpts/", "_includes/"} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("unwanted %q", unwanted)
		}
	}
	sealedAgain, err := SealTransclusions(output)
	if err != nil || sealedAgain != output {
		t.Fatal("sealing is not idempotent", err)
	}
	markdown, restoreMacros, err := RestoreTransclusions(strings.ReplaceAll(output, "\n", "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	imported, err := md.ToStorage([]byte(markdown))
	if err != nil {
		t.Fatal(err)
	}
	imported = restoreMacros(imported)
	for _, want := range []string{`ac:name="include"`, `ri:space-key="API"`, `ri:content-title="External"`, `ac:name="excerpt-include"`, `ac:name="nopanel">true`, "Page beginning", "Page ending"} {
		if !strings.Contains(imported, want) {
			t.Fatalf("missing %q: %s", want, imported)
		}
	}
	if strings.Contains(imported, "Full external text") {
		t.Fatal("snapshot was materialized instead of restoring the live reference")
	}
	edited := strings.Replace(output, "Full external text", "Changed external text", 1)
	if _, _, err := RestoreTransclusions(edited); err == nil {
		t.Fatal("edited snapshot silently discarded")
	}
}

func TestOwnExcerptRemainsEditable(t *testing.T) {
	source := `<p>Before</p><ac:structured-macro ac:name="excerpt"><ac:parameter ac:name="hidden">false</ac:parameter><ac:rich-text-body><p>Editable excerpt</p></ac:rich-text-body></ac:structured-macro><p>After</p>`
	prepared, restore, err := PrepareRoundTripExport(source, "", RoundTripOptions{InlineTransclusions: true})
	if err != nil {
		t.Fatal(err)
	}
	output, err := StorageToMarkdownAdvanced(prepared, "")
	if err != nil {
		t.Fatal(err)
	}
	output, err = SealTransclusions(restore(output))
	if err != nil {
		t.Fatal(err)
	}
	output = strings.ReplaceAll(output, "Editable excerpt", "Updated excerpt")
	prepared, restore, err = RestoreTransclusions(output)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := md.ToStorage([]byte(prepared))
	if err != nil {
		t.Fatal(err)
	}
	imported = restore(imported)
	for _, want := range []string{`ac:name="excerpt"`, `ac:name="hidden">false`, "<p>Updated excerpt</p>", "Before", "After"} {
		if !strings.Contains(imported, want) {
			t.Fatalf("missing %q: %s", want, imported)
		}
	}
}

func TestTransclusionFailuresAndCodeExamples(t *testing.T) {
	opts := RoundTripOptions{SpaceKey: "DOCS", PageID: 1, InlineTransclusions: true,
		ResolvePage: func(space, title string) (int, error) { return 1, nil },
		FetchPage: func(id int) (RoundTripPage, error) {
			t.Fatal("cycle should be detected before fetching")
			return RoundTripPage{}, nil
		},
	}
	source := `<ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-title="Self"/></ac:link></ac:parameter></ac:structured-macro>`
	if _, _, err := PrepareRoundTripExport(source, "", opts); err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatal("cycle not rejected", err)
	}
	for _, source := range []string{
		"<!-- confcli-transclusion:end -->",
		transclusionStart("transclusion", transclusionMetadata{Macro: "include", Title: "Source"}) + "\nbody",
		"<!-- confcli-transclusion:start BAD -->\nbody\n<!-- confcli-transclusion:end -->",
	} {
		if _, _, err := RestoreTransclusions(source); err == nil {
			t.Fatal("malformed markers accepted", source)
		}
	}
	example := "\x60\x60\x60text\n<!-- confcli-transclusion:end -->\n\x60\x60\x60"
	out, restore, err := RestoreTransclusions(example)
	if err != nil || out != example || restore(out) != example {
		t.Fatal("code example was treated as metadata", err)
	}
}
