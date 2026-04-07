package transforms

import (
	"fmt"
	"testing"

	"confcli/pkg/converters"
)

func TestExpandTinyURLsBasic(t *testing.T) {
	// Encode page ID 12345 to get a known tiny URL ID
	tinyID := converters.EncodeTinyURL(12345)

	e := NewExpandTinyURLs("https://confluence.example.com", DecodingResolver())
	html := fmt.Sprintf(`<a href="/x/%s">Setup Guide</a>`, tinyID)
	ctx := &TransformContext{PreContent: html}

	if err := e.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `<a href="https://confluence.example.com/pages/viewpage.action?pageId=12345">Setup Guide</a>`
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}

func TestExpandTinyURLsAbsoluteURL(t *testing.T) {
	tinyID := converters.EncodeTinyURL(42)

	e := NewExpandTinyURLs("https://confluence.example.com", DecodingResolver())
	html := fmt.Sprintf(`<a href="https://confluence.example.com/x/%s">Page</a>`, tinyID)
	ctx := &TransformContext{PreContent: html}

	if err := e.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `<a href="https://confluence.example.com/pages/viewpage.action?pageId=42">Page</a>`
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}

func TestExpandTinyURLsWithAnchor(t *testing.T) {
	tinyID := converters.EncodeTinyURL(99)

	e := NewExpandTinyURLs("https://confluence.example.com", DecodingResolver())
	html := fmt.Sprintf(`<a href="/x/%s#section-2">Section</a>`, tinyID)
	ctx := &TransformContext{PreContent: html}

	if err := e.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `<a href="https://confluence.example.com/pages/viewpage.action?pageId=99#section-2">Section</a>`
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}

func TestExpandTinyURLsMultiple(t *testing.T) {
	tinyID1 := converters.EncodeTinyURL(100)
	tinyID2 := converters.EncodeTinyURL(200)

	e := NewExpandTinyURLs("https://conf.co", DecodingResolver())
	html := fmt.Sprintf(
		`<p>See <a href="/x/%s">Page A</a> and <a href="/x/%s">Page B</a></p>`,
		tinyID1, tinyID2,
	)
	ctx := &TransformContext{PreContent: html}

	if err := e.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `<p>See <a href="https://conf.co/pages/viewpage.action?pageId=100">Page A</a> and <a href="https://conf.co/pages/viewpage.action?pageId=200">Page B</a></p>`
	if ctx.PreContent != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, ctx.PreContent)
	}
}

func TestExpandTinyURLsNilResolver(t *testing.T) {
	e := NewExpandTinyURLs("https://confluence.example.com", nil)
	html := `<a href="/x/AAAA">Page</a>`
	ctx := &TransformContext{PreContent: html}

	if err := e.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PreContent != html {
		t.Errorf("expected no change with nil resolver, got %q", ctx.PreContent)
	}
}

func TestExpandTinyURLsUnresolvable(t *testing.T) {
	resolver := func(tinyID string) string { return "" }

	e := NewExpandTinyURLs("https://confluence.example.com", resolver)
	html := `<a href="/x/ZZZZZZZZ">Unknown</a>`
	ctx := &TransformContext{PreContent: html}

	if err := e.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PreContent != html {
		t.Errorf("expected no change for unresolvable tiny URL, got %q", ctx.PreContent)
	}
}

func TestExpandTinyURLsNoMatchNonTinyURL(t *testing.T) {
	e := NewExpandTinyURLs("https://confluence.example.com", DecodingResolver())
	html := `<a href="https://confluence.example.com/pages/viewpage.action?pageId=123">Normal</a>`
	ctx := &TransformContext{PreContent: html}

	if err := e.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PreContent != html {
		t.Errorf("should not modify non-tiny-URL links, got %q", ctx.PreContent)
	}
}

func TestExpandTinyURLsSingleQuotes(t *testing.T) {
	tinyID := converters.EncodeTinyURL(555)

	e := NewExpandTinyURLs("https://confluence.example.com", DecodingResolver())
	html := fmt.Sprintf(`<a href='/x/%s'>Page</a>`, tinyID)
	ctx := &TransformContext{PreContent: html}

	if err := e.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := fmt.Sprintf(`<a href='https://confluence.example.com/pages/viewpage.action?pageId=555'>Page</a>`)
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}

func TestExpandTinyURLsName(t *testing.T) {
	e := NewExpandTinyURLs("", nil)
	if e.Name() != "expand/tiny-urls" {
		t.Errorf("expected 'expand/tiny-urls', got %q", e.Name())
	}
}

func TestCachingResolver(t *testing.T) {
	callCount := 0
	inner := func(tinyID string) string {
		callCount++
		return fmt.Sprintf("/pages/viewpage.action?pageId=%s", tinyID)
	}

	cached := CachingResolver(inner)

	// Call twice with same ID
	result1 := cached("AbCd")
	result2 := cached("AbCd")

	if result1 != result2 {
		t.Errorf("cache inconsistency: %q vs %q", result1, result2)
	}
	if callCount != 1 {
		t.Errorf("expected inner called once, got %d", callCount)
	}

	// Different ID should call inner again
	cached("EfGh")
	if callCount != 2 {
		t.Errorf("expected inner called twice, got %d", callCount)
	}
}

func TestVerifyingResolverPageExists(t *testing.T) {
	tinyID := converters.EncodeTinyURL(42)
	resolver := VerifyingResolver(func(pageID int) bool { return pageID == 42 })

	result := resolver(tinyID)
	expected := "/pages/viewpage.action?pageId=42"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestVerifyingResolverPageMissing(t *testing.T) {
	tinyID := converters.EncodeTinyURL(42)
	resolver := VerifyingResolver(func(pageID int) bool { return false })

	result := resolver(tinyID)
	if result != "" {
		t.Errorf("expected empty for missing page, got %q", result)
	}
}

func TestExpandTinyURLsNoBaseURL(t *testing.T) {
	tinyID := converters.EncodeTinyURL(77)

	e := NewExpandTinyURLs("", DecodingResolver())
	html := fmt.Sprintf(`<a href="/x/%s">Page</a>`, tinyID)
	ctx := &TransformContext{PreContent: html}

	if err := e.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `<a href="/pages/viewpage.action?pageId=77">Page</a>`
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}
