package transforms

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestRewriteMarkdownLinksBasic(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{"engineering/setup.md": {PageID: 12345, Title: "Setup Guide"}},
		"engineering",
	)
	ctx := &TransformContext{PreContent: "See [Setup](setup.md)."}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	expected := `See <ac:link><ri:page ri:content-title="Setup Guide" /><ac:plain-text-link-body><![CDATA[Setup]]></ac:plain-text-link-body></ac:link>.`
	if ctx.PreContent != expected {
		t.Errorf("\nexpected: %s\n     got: %s", expected, ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksParentRelative(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{"docs/other.md": {Title: "Other"}},
		"docs/sub",
	)
	ctx := &TransformContext{PreContent: "[Other](../other.md)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.PreContent, `ri:content-title="Other"`) {
		t.Errorf("expected ri:content-title=Other, got: %s", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksAnchor(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{"page.md": {Title: "Target"}},
		"",
	)
	ctx := &TransformContext{PreContent: "[Section](page.md#install-steps)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.PreContent, `ac:anchor="install-steps"`) {
		t.Errorf("expected ac:anchor=install-steps, got: %s", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksAnchorURLEncoded(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{"p.md": {Title: "P"}},
		"",
	)
	ctx := &TransformContext{PreContent: "[X](p.md#a%20b)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.PreContent, `ac:anchor="a b"`) {
		t.Errorf("expected ac:anchor=\"a b\", got: %s", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksSpaceKey(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{"x.md": {Title: "X", SpaceKey: "DEV"}},
		"",
	)
	ctx := &TransformContext{PreContent: "[X](x.md)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.PreContent, `ri:space-key="DEV"`) {
		t.Errorf("expected ri:space-key=DEV, got: %s", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksUnresolved(t *testing.T) {
	var buf bytes.Buffer
	r := NewRewriteMarkdownLinks(map[string]PageRef{}, "")
	r.Logger = log.New(&buf, "", 0)
	ctx := &TransformContext{PreContent: "[Missing](missing.md)", PagePath: "current.md"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PreContent != "Missing" {
		t.Errorf("expected 'Missing', got %q", ctx.PreContent)
	}
	if !strings.Contains(buf.String(), "unresolved link") {
		t.Errorf("expected warning, got: %s", buf.String())
	}
}

func TestRewriteMarkdownLinksImageSkipped(t *testing.T) {
	r := NewRewriteMarkdownLinks(map[string]PageRef{"img.md": {Title: "X"}}, "")
	ctx := &TransformContext{PreContent: "![alt](img.md)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PreContent != "![alt](img.md)" {
		t.Errorf("image should not be rewritten, got %q", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksExternalSkipped(t *testing.T) {
	r := NewRewriteMarkdownLinks(map[string]PageRef{}, "")
	content := "[Google](https://google.com)"
	ctx := &TransformContext{PreContent: content}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PreContent != content {
		t.Errorf("external link should be unchanged, got %q", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksMailtoSkipped(t *testing.T) {
	r := NewRewriteMarkdownLinks(map[string]PageRef{}, "")
	content := "[Email](mailto:a@b.c)"
	ctx := &TransformContext{PreContent: content}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PreContent != content {
		t.Errorf("mailto link should be unchanged, got %q", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksPureAnchorSkipped(t *testing.T) {
	r := NewRewriteMarkdownLinks(map[string]PageRef{}, "")
	content := "[TOC](#section)"
	ctx := &TransformContext{PreContent: content}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PreContent != content {
		t.Errorf("in-page anchor should be unchanged, got %q", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksNonMDSkipped(t *testing.T) {
	r := NewRewriteMarkdownLinks(map[string]PageRef{}, "")
	content := "[code](src/main.go)"
	ctx := &TransformContext{PreContent: content}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PreContent != content {
		t.Errorf("non-MD link should be unchanged, got %q", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksNilMap(t *testing.T) {
	r := &RewriteMarkdownLinks{}
	content := "[X](x.md)"
	ctx := &TransformContext{PreContent: content}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PreContent != content {
		t.Errorf("nil map should be noop, got %q", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksURLEncoded(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{"docs/my page.md": {Title: "My Page"}},
		"",
	)
	ctx := &TransformContext{PreContent: "[X](docs/my%20page.md)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.PreContent, `ri:content-title="My Page"`) {
		t.Errorf("expected My Page title, got: %s", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksMultiple(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{"a.md": {Title: "A"}, "b.md": {Title: "B"}},
		"",
	)
	ctx := &TransformContext{PreContent: "See [A](a.md) and [B](b.md)."}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.PreContent, `ri:content-title="A"`) {
		t.Errorf("missing A: %s", ctx.PreContent)
	}
	if !strings.Contains(ctx.PreContent, `ri:content-title="B"`) {
		t.Errorf("missing B: %s", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksTitleEscape(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{"p.md": {Title: `Q&A "FAQ" <test>`}},
		"",
	)
	ctx := &TransformContext{PreContent: "[X](p.md)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.PreContent, `ri:content-title="Q&amp;A &quot;FAQ&quot; &lt;test&gt;"`) {
		t.Errorf("title not properly escaped: %s", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksEscapesRepoRoot(t *testing.T) {
	var buf bytes.Buffer
	r := NewRewriteMarkdownLinks(map[string]PageRef{}, "")
	r.Logger = log.New(&buf, "", 0)
	ctx := &TransformContext{PreContent: "[Up](../../escape.md)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.PreContent != "Up" {
		t.Errorf("escape link should be plain text, got %q", ctx.PreContent)
	}
	if !strings.Contains(buf.String(), "escapes repo root") {
		t.Errorf("expected escape warning, got: %s", buf.String())
	}
}

func TestRewriteMarkdownLinksMixedScenario(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{
			"docs/a.md": {Title: "A"},
			"docs/b.md": {Title: "B", SpaceKey: "DEV"},
		},
		"docs",
	)
	ctx := &TransformContext{
		PreContent: "[A](a.md), [B at section](b.md#sec), [external](https://x.com), [TOC](#toc), [missing](c.md), ![img](img.md)",
	}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	got := ctx.PreContent
	if !strings.Contains(got, `ri:content-title="A"`) {
		t.Errorf("A not rewritten: %s", got)
	}
	if !strings.Contains(got, `ri:space-key="DEV"`) || !strings.Contains(got, `ac:anchor="sec"`) {
		t.Errorf("B with anchor/space not rewritten: %s", got)
	}
	if !strings.Contains(got, `[external](https://x.com)`) {
		t.Errorf("external link should remain: %s", got)
	}
	if !strings.Contains(got, `[TOC](#toc)`) {
		t.Errorf("in-page anchor should remain: %s", got)
	}
	if strings.Contains(got, `[missing]`) {
		t.Errorf("unresolved link should be reduced to text: %s", got)
	}
	if !strings.Contains(got, `![img](img.md)`) {
		t.Errorf("image should remain: %s", got)
	}
}

func TestRewriteMarkdownLinksCDATAGuard(t *testing.T) {
	r := NewRewriteMarkdownLinks(
		map[string]PageRef{"p.md": {Title: "P"}},
		"",
	)
	ctx := &TransformContext{PreContent: "[end]]>guard](p.md)"}
	if err := r.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ctx.PreContent, "]]><") && !strings.Contains(ctx.PreContent, "]]]]><![CDATA[>") {
		t.Errorf("CDATA guard missing: %s", ctx.PreContent)
	}
}

func TestRewriteMarkdownLinksName(t *testing.T) {
	r := NewRewriteMarkdownLinks(nil, "")
	if r.Name() != "rewrite/md-links" {
		t.Errorf("expected rewrite/md-links, got %s", r.Name())
	}
}

func TestRewriteMarkdownLinksRegistry(t *testing.T) {
	reg := DefaultRegistry()
	spec := TransformSpec{
		Type:   "rewrite_md_links",
		Params: map[string]interface{}{"current_page_dir": "docs"},
	}
	tr, err := reg.Build(spec)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Name() != "rewrite/md-links" {
		t.Errorf("expected rewrite/md-links, got %s", tr.Name())
	}
}
