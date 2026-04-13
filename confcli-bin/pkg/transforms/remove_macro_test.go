package transforms

import (
	"strings"
	"testing"
)

func TestRemoveMacroSimple(t *testing.T) {
	rm, err := NewRemoveMacro("code")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<p>Before</p><ac:structured-macro ac:name="code"><ac:parameter ac:name="language">go</ac:parameter><ac:plain-text-body>fmt.Println("hi")</ac:plain-text-body></ac:structured-macro><p>After</p>`,
	}

	if err := rm.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `<p>Before</p><p>After</p>`
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}

func TestRemoveMacroRegex(t *testing.T) {
	rm, err := NewRemoveMacro(`^(info|warning|note)$`)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<ac:structured-macro ac:name="info"><ac:rich-text-body>Info text</ac:rich-text-body></ac:structured-macro>` +
			`<ac:structured-macro ac:name="warning"><ac:rich-text-body>Warning text</ac:rich-text-body></ac:structured-macro>` +
			`<ac:structured-macro ac:name="code"><ac:plain-text-body>keep me</ac:plain-text-body></ac:structured-macro>`,
	}

	if err := rm.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// code macro should remain
	if ctx.PreContent != `<ac:structured-macro ac:name="code"><ac:plain-text-body>keep me</ac:plain-text-body></ac:structured-macro>` {
		t.Errorf("unexpected result: %q", ctx.PreContent)
	}
}

func TestRemoveMacroNested(t *testing.T) {
	rm, err := NewRemoveMacro("expand")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<ac:structured-macro ac:name="expand"><ac:rich-text-body><ac:structured-macro ac:name="code"><ac:plain-text-body>inner</ac:plain-text-body></ac:structured-macro></ac:rich-text-body></ac:structured-macro>rest`,
	}

	if err := rm.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PreContent != "rest" {
		t.Errorf("expected 'rest', got %q", ctx.PreContent)
	}
}

func TestRemoveMacroNoMatch(t *testing.T) {
	rm, err := NewRemoveMacro("nonexistent")
	if err != nil {
		t.Fatal(err)
	}

	content := `<ac:structured-macro ac:name="code"><ac:plain-text-body>keep</ac:plain-text-body></ac:structured-macro>`
	ctx := &TransformContext{PreContent: content}

	if err := rm.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PreContent != content {
		t.Errorf("content should be unchanged, got %q", ctx.PreContent)
	}
}

func TestRemoveMacroInvalidPattern(t *testing.T) {
	_, err := NewRemoveMacro("[invalid")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestRemoveMacroName(t *testing.T) {
	rm, err := NewRemoveMacro("test")
	if err != nil {
		t.Fatal(err)
	}
	if rm.Name() != "remove/macro" {
		t.Errorf("expected 'remove/macro', got %q", rm.Name())
	}
}

func TestRemoveMacroPreserveContent(t *testing.T) {
	rm, err := NewRemoveMacroWithContentPreserve("expand")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<p>Before</p><ac:structured-macro ac:name="expand"><ac:parameter ac:name="summary">Click to expand</ac:parameter><ac:rich-text-body><p>Expanded content here</p></ac:rich-text-body></ac:structured-macro><p>After</p>`,
	}

	if err := rm.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// Content should be preserved
	if !strings.Contains(ctx.PreContent, "Expanded content here") {
		t.Errorf("expected content to be preserved, got %q", ctx.PreContent)
	}
	
	// Macro wrapper should be removed
	if strings.Contains(ctx.PreContent, "ac:structured-macro") {
		t.Errorf("macro wrapper should be removed, got %q", ctx.PreContent)
	}
}

func TestRemoveMacroPreserveContentNestedMacro(t *testing.T) {
	rm, err := NewRemoveMacroWithContentPreserve("expand")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<p>Before</p><ac:structured-macro ac:name="expand"><ac:parameter ac:name="summary">Details</ac:parameter><ac:rich-text-body><p>Some text</p><ac:structured-macro ac:name="toc"><ac:parameter ac:name="maxLevel">3</ac:parameter></ac:structured-macro></ac:rich-text-body></ac:structured-macro><p>After</p>`,
	}

	if err := rm.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// The expand macro should be removed
	if strings.Contains(ctx.PreContent, `ac:name="expand"`) {
		t.Errorf("expand macro should be removed, got %q", ctx.PreContent)
	}

	// The nested toc macro should be preserved as a proper element (not escaped)
	if !strings.Contains(ctx.PreContent, `<ac:structured-macro ac:name="toc"`) {
		t.Errorf("nested toc macro should be preserved as proper element, got %q", ctx.PreContent)
	}

	// Should not contain escaped HTML entities from the nested macro
	if strings.Contains(ctx.PreContent, "&lt;ac:") {
		t.Errorf("nested macro should not be escaped, got %q", ctx.PreContent)
	}

	// Surrounding content should be preserved
	if !strings.Contains(ctx.PreContent, "<p>Before</p>") || !strings.Contains(ctx.PreContent, "<p>After</p>") {
		t.Errorf("surrounding content should be preserved, got %q", ctx.PreContent)
	}
}
