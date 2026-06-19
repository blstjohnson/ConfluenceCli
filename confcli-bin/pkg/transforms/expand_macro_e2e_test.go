package transforms

import (
	"strings"
	"testing"

	"confcli/pkg/converters"
)

// TestExpandVsClearMacroThroughConverter verifies the pre-conversion ordering
// used by `page get`: a remove_macro transform edits the storage XHTML, and the
// result is then converted to markdown. "expand" (preserve_content) keeps the
// inner text; "clear" drops it. This guards against regressing back to running
// macro transforms after conversion (where they would be no-ops).
func TestExpandVsClearMacroThroughConverter(t *testing.T) {
	const storage = `<p>Intro.</p>` +
		`<ac:structured-macro ac:name="expand">` +
		`<ac:parameter ac:name="title">Details</ac:parameter>` +
		`<ac:rich-text-body><p>Hidden treasure</p></ac:rich-text-body>` +
		`</ac:structured-macro>` +
		`<p>Outro.</p>`

	// expand: unwrap the macro, keep its body.
	expand, err := NewRemoveMacroWithContentPreserve("expand")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &TransformContext{PreContent: storage, Format: "markdown"}
	if err := expand.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	md, err := converters.StorageToMarkdownAdvanced(ctx.PreContent, "")
	if err != nil {
		t.Fatalf("convert (expand): %v", err)
	}
	if !strings.Contains(md, "Hidden treasure") {
		t.Errorf("expand: expected body preserved, got:\n%s", md)
	}

	// clear: drop the macro and its body entirely.
	clear, err := NewRemoveMacro("expand")
	if err != nil {
		t.Fatal(err)
	}
	ctx = &TransformContext{PreContent: storage, Format: "markdown"}
	if err := clear.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	md, err = converters.StorageToMarkdownAdvanced(ctx.PreContent, "")
	if err != nil {
		t.Fatalf("convert (clear): %v", err)
	}
	if strings.Contains(md, "Hidden treasure") {
		t.Errorf("clear: expected body removed, got:\n%s", md)
	}
	if !strings.Contains(md, "Intro.") || !strings.Contains(md, "Outro.") {
		t.Errorf("clear: surrounding content should survive, got:\n%s", md)
	}
}
