package sync

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHeadingLinkReusesLegacyAnchorInsideTargetHeading(t *testing.T) {
	profile := basicProfile()
	source := "[Message](target.md#API%20%22Message%22)"
	target := "<a id=\"old-slug\"></a>\n\n## API \"Message\"<a id=\"message\"></a>\n\n```html\n<a id=\"message\"></a>\n```\n"
	fsys := fstest.MapFS{"source.md": {Data: []byte(source)}, "target.md": {Data: []byte(target)}}
	refs, err := BuildPathMap(profile, fsys)
	if err != nil {
		t.Fatal(err)
	}
	if refs["target.md"].HeadingAnchors[`API "Message"`] != "message" {
		t.Fatal(refs)
	}
	convert := NewMarkdownConverter(refs, nil, nil, nil)
	storage, err := convert(context.Background(), []byte(source), "source.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(storage, `<ac:link ac:anchor="message"><ri:page ri:content-title="target"`) {
		t.Fatal(storage)
	}
	storage, err = convert(context.Background(), []byte(target), "target.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storage, "confcli-heading-") || !strings.Contains(storage, `<ac:parameter ac:name="">message</ac:parameter>`) {
		t.Fatal(storage)
	}
}

func TestHeadingLinkRemainsValidAfterRemovingLegacyHTMLAnchor(t *testing.T) {
	profile := basicProfile()
	source := "[Lookup](target.md#FLOW.1.1/1.2%20Lookup%20%28PPC%29)"
	target := "#### FLOW.1.1/1.2 Lookup (PPC)\n\nLookup rules.\n"
	fsys := fstest.MapFS{"source.md": {Data: []byte(source)}, "target.md": {Data: []byte(target)}}
	refs, err := BuildPathMap(profile, fsys)
	if err != nil {
		t.Fatal(err)
	}
	convert := NewMarkdownConverter(refs, nil, nil, nil)
	storage, err := convert(context.Background(), []byte(source), "source.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(storage, `<ac:link ac:anchor="FLOW.1.1/1.2 Lookup (PPC)"><ri:page ri:content-title="target"`) {
		t.Fatalf("heading link must use the native Confluence heading: %s", storage)
	}
	storage, err = convert(context.Background(), []byte(target), "target.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storage, `ac:name="anchor"`) || strings.Contains(storage, `<a id=`) || !strings.Contains(storage, ">FLOW.1.1/1.2 Lookup (PPC)</h4>") {
		t.Fatalf("a unique heading must not need a synthetic target: %s", storage)
	}
}
