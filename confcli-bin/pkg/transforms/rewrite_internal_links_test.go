package transforms

import "testing"

func TestRewriteInternalLinksViewPage(t *testing.T) {
	r := NewRewriteInternalLinks(
		map[int]string{12345: "engineering/setup.md"},
		"confluence.example.com",
		"engineering",
	)

	ctx := &TransformContext{
		PostContent: `See [Setup](https://confluence.example.com/pages/viewpage.action?pageId=12345)`,
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `See [Setup](setup.md)`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestRewriteInternalLinksSpacesFormat(t *testing.T) {
	r := NewRewriteInternalLinks(
		map[int]string{99: "docs/page.md"},
		"confluence.example.com",
		"",
	)

	ctx := &TransformContext{
		PostContent: `[page](https://confluence.example.com/spaces/~99)`,
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `[page](docs/page.md)`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestRewriteInternalLinksPageNotInExport(t *testing.T) {
	r := NewRewriteInternalLinks(
		map[int]string{}, // empty map
		"confluence.example.com",
		"",
	)

	ctx := &TransformContext{
		PostContent: `[page](https://confluence.example.com/pages/viewpage.action?pageId=999)`,
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// Should keep the link but strip anchor
	expected := `[page](https://confluence.example.com/pages/viewpage.action?pageId=999)`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestRewriteInternalLinksPageExistsCheckerFalse(t *testing.T) {
	r := NewRewriteInternalLinks(
		map[int]string{},
		"confluence.example.com",
		"",
	)
	r.PageExistsChecker = func(pageID int) bool { return false }

	ctx := &TransformContext{
		PostContent: `[dead link](https://confluence.example.com/pages/viewpage.action?pageId=999)`,
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// Dead page — link text only
	expected := `dead link`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestRewriteInternalLinksUnrecognizedConfluenceURL(t *testing.T) {
	r := NewRewriteInternalLinks(
		map[int]string{},
		"confluence.example.com",
		"",
	)

	ctx := &TransformContext{
		PostContent: `[unknown](https://confluence.example.com/some/random/path)`,
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// Can't extract page ID, but URL matches base → strip link
	expected := `unknown`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestRewriteInternalLinksExternalURL(t *testing.T) {
	r := NewRewriteInternalLinks(
		map[int]string{},
		"confluence.example.com",
		"",
	)

	content := `[google](https://google.com)`
	ctx := &TransformContext{PostContent: content}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PostContent != content {
		t.Errorf("external links should be unchanged")
	}
}

func TestRewriteInternalLinksNilPageMap(t *testing.T) {
	r := NewRewriteInternalLinks(nil, "confluence.example.com", "")

	content := `[page](https://confluence.example.com/pages/viewpage.action?pageId=1)`
	ctx := &TransformContext{PostContent: content}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PostContent != content {
		t.Errorf("should be noop when PageMap is nil")
	}
}

func TestRewriteInternalLinksStripAnchor(t *testing.T) {
	r := NewRewriteInternalLinks(
		map[int]string{42: "page.md"},
		"confluence.example.com",
		"",
	)

	ctx := &TransformContext{
		PostContent: `[page](https://confluence.example.com/pages/viewpage.action?pageId=42#heading)`,
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `[page](page.md)`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestRewriteInternalLinksName(t *testing.T) {
	r := NewRewriteInternalLinks(nil, "", "")
	if r.Name() != "rewrite/internal-links" {
		t.Errorf("expected 'rewrite/internal-links', got %q", r.Name())
	}
}
