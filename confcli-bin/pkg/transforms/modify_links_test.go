package transforms

import "testing"

func TestModifyLinksSimpleReplace(t *testing.T) {
	ml, err := NewModifyLinks(LinkRule{
		Find:    `^http://old\.example\.com`,
		Replace: "https://new.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PostContent: `See [docs](http://old.example.com/guide) and [other](http://other.com/page)`,
	}
	if err := ml.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `See [docs](https://new.example.com/guide) and [other](http://other.com/page)`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestModifyLinksMultipleRules(t *testing.T) {
	ml, err := NewModifyLinks(
		LinkRule{Find: `\.html$`, Replace: ".md"},
		LinkRule{Find: `^/docs/`, Replace: "/content/"},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PostContent: `[page](/docs/readme.html)`,
	}
	if err := ml.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `[page](/content/readme.md)`
	if ctx.PostContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PostContent)
	}
}

func TestModifyLinksNoMatch(t *testing.T) {
	ml, err := NewModifyLinks(LinkRule{Find: `^nomatch`, Replace: "x"})
	if err != nil {
		t.Fatal(err)
	}

	content := `[link](http://example.com)`
	ctx := &TransformContext{PostContent: content}
	if err := ml.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PostContent != content {
		t.Errorf("content should be unchanged")
	}
}

func TestModifyLinksInvalidPattern(t *testing.T) {
	_, err := NewModifyLinks(LinkRule{Find: "[invalid", Replace: "x"})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestModifyLinksName(t *testing.T) {
	ml, err := NewModifyLinks()
	if err != nil {
		t.Fatal(err)
	}
	if ml.Name() != "modify/links" {
		t.Errorf("expected 'modify/links', got %q", ml.Name())
	}
}
