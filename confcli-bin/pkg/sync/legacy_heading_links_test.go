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
