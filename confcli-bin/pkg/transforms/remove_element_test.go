package transforms

import "testing"

func TestRemoveElementByTag(t *testing.T) {
	re, err := NewRemoveElement("div")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<p>keep</p><div class="foo">remove me</div><p>also keep</p>`,
	}
	if err := re.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `<p>keep</p><p>also keep</p>`
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}

func TestRemoveElementByClass(t *testing.T) {
	re, err := NewRemoveElement("div.hidden")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<div class="visible">keep</div><div class="hidden">remove</div>`,
	}
	if err := re.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `<div class="visible">keep</div>`
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}

func TestRemoveElementByID(t *testing.T) {
	re, err := NewRemoveElement("span#footer")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `<span id="header">keep</span><span id="footer">remove</span>`,
	}
	if err := re.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `<span id="header">keep</span>`
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}

func TestRemoveElementSelfClosing(t *testing.T) {
	re, err := NewRemoveElement("br")
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent: `hello<br/>world`,
	}
	if err := re.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	expected := `helloworld`
	if ctx.PreContent != expected {
		t.Errorf("expected %q, got %q", expected, ctx.PreContent)
	}
}

func TestRemoveElementNoMatch(t *testing.T) {
	re, err := NewRemoveElement("table")
	if err != nil {
		t.Fatal(err)
	}

	content := `<p>no tables here</p>`
	ctx := &TransformContext{PreContent: content}
	if err := re.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PreContent != content {
		t.Errorf("content should be unchanged")
	}
}

func TestRemoveElementName(t *testing.T) {
	re, err := NewRemoveElement("div")
	if err != nil {
		t.Fatal(err)
	}
	if re.Name() != "remove/element" {
		t.Errorf("expected 'remove/element', got %q", re.Name())
	}
}
