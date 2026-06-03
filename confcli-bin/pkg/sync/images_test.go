package sync

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestRewriteImages_LocalImageBecomesAttachmentMacro(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/page.md":     {Data: []byte("![a diagram](img/diagram.png)")},
		"docs/img/diagram.png": {Data: []byte("PNGDATA")},
	}
	storage := `<p><img src="img/diagram.png" alt="a diagram" /></p>`

	out, images, err := rewriteImages(storage, "docs/page.md", fsys, nil)
	if err != nil {
		t.Fatalf("rewriteImages: %v", err)
	}
	if strings.Contains(out, "<img") {
		t.Errorf("raw <img> should be gone: %s", out)
	}
	if !strings.Contains(out, `<ri:attachment ri:filename="diagram.png" />`) {
		t.Errorf("expected ri:attachment by basename: %s", out)
	}
	if !strings.Contains(out, `<ac:image ac:alt="a diagram">`) {
		t.Errorf("expected alt carried onto ac:image: %s", out)
	}
	if len(images) != 1 || images[0].Filename != "diagram.png" || string(images[0].Data) != "PNGDATA" {
		t.Fatalf("collected images = %+v, want one diagram.png with PNGDATA", images)
	}
}

func TestRewriteImages_RemoteImageUntouched(t *testing.T) {
	fsys := fstest.MapFS{}
	storage := `<p><img src="https://example.com/x.png" alt="x" /></p>`

	out, images, err := rewriteImages(storage, "page.md", fsys, nil)
	if err != nil {
		t.Fatalf("rewriteImages: %v", err)
	}
	if out != storage {
		t.Errorf("remote image should be left untouched, got: %s", out)
	}
	if len(images) != 0 {
		t.Errorf("remote image must not be collected: %+v", images)
	}
}

func TestRewriteImages_MissingFileLeftAsIs(t *testing.T) {
	fsys := fstest.MapFS{}
	storage := `<p><img src="missing.png" alt="m" /></p>`

	out, images, err := rewriteImages(storage, "page.md", fsys, nil)
	if err != nil {
		t.Fatalf("rewriteImages: %v", err)
	}
	if !strings.Contains(out, "<img") {
		t.Errorf("unreadable image should be left as a literal <img>: %s", out)
	}
	if len(images) != 0 {
		t.Errorf("unreadable image must not be collected: %+v", images)
	}
}

func TestRewriteImages_UnsupportedExtensionLeftAsIs(t *testing.T) {
	fsys := fstest.MapFS{"weird.tiff": {Data: []byte("x")}}
	storage := `<p><img src="weird.tiff" /></p>`

	out, images, err := rewriteImages(storage, "page.md", fsys, nil)
	if err != nil {
		t.Fatalf("rewriteImages: %v", err)
	}
	if !strings.Contains(out, "<img") || len(images) != 0 {
		t.Errorf("unsupported extension should be left as-is; out=%s images=%+v", out, images)
	}
}

func TestRewriteImages_CDATAPreserved(t *testing.T) {
	fsys := fstest.MapFS{"x.png": {Data: []byte("d")}}
	// An <img> inside a code-block CDATA body is literal text, not an image.
	storage := `<ac:plain-text-body><![CDATA[<img src="x.png" />]]></ac:plain-text-body>`

	out, images, err := rewriteImages(storage, "page.md", fsys, nil)
	if err != nil {
		t.Fatalf("rewriteImages: %v", err)
	}
	if !strings.Contains(out, `<![CDATA[<img src="x.png" />]]>`) {
		t.Errorf("CDATA <img> must survive verbatim: %s", out)
	}
	if len(images) != 0 {
		t.Errorf("CDATA <img> must not be collected: %+v", images)
	}
}

func TestRewriteImages_RelativeParentPath(t *testing.T) {
	fsys := fstest.MapFS{
		"docs/sub/page.md":  {Data: []byte("x")},
		"docs/assets/p.png": {Data: []byte("P")},
	}
	storage := `<img src="../assets/p.png" />`

	out, images, err := rewriteImages(storage, "docs/sub/page.md", fsys, nil)
	if err != nil {
		t.Fatalf("rewriteImages: %v", err)
	}
	if !strings.Contains(out, `ri:filename="p.png"`) {
		t.Errorf("expected resolved basename p.png: %s", out)
	}
	if len(images) != 1 || string(images[0].Data) != "P" {
		t.Fatalf("expected p.png bytes resolved via ../: %+v", images)
	}
}

func TestRewriteImages_DedupesByBasename(t *testing.T) {
	fsys := fstest.MapFS{"a.png": {Data: []byte("A")}}
	storage := `<img src="a.png" /> and again <img src="a.png" />`

	out, images, err := rewriteImages(storage, "page.md", fsys, nil)
	if err != nil {
		t.Fatalf("rewriteImages: %v", err)
	}
	if got := strings.Count(out, "<ac:image>"); got != 2 {
		t.Errorf("both references should be rewritten, got %d ac:image", got)
	}
	if len(images) != 1 {
		t.Errorf("same basename must be collected once, got %d", len(images))
	}
}

func TestImageFingerprint_OrderIndependentAndContentSensitive(t *testing.T) {
	a := []ImageRef{{Filename: "a.png", Data: []byte("1")}, {Filename: "b.png", Data: []byte("2")}}
	b := []ImageRef{{Filename: "b.png", Data: []byte("2")}, {Filename: "a.png", Data: []byte("1")}}
	if imageFingerprint(a) != imageFingerprint(b) {
		t.Errorf("fingerprint must be order-independent")
	}

	c := []ImageRef{{Filename: "a.png", Data: []byte("CHANGED")}, {Filename: "b.png", Data: []byte("2")}}
	if imageFingerprint(a) == imageFingerprint(c) {
		t.Errorf("fingerprint must change when image bytes change")
	}

	if imageFingerprint(nil) != "" {
		t.Errorf("empty image set must yield empty fingerprint")
	}
}
