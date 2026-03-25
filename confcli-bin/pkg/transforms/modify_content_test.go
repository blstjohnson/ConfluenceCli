package transforms

import "testing"

func TestModifyContentPrePhase(t *testing.T) {
	mc, err := NewModifyContent(PhasePre, ContentRule{
		Find:    `<ac:emoticon[^>]*/>`,
		Replace: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent:  `Hello <ac:emoticon ac:name="smile"/> world`,
		PostContent: "untouched",
	}
	if err := mc.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PreContent != "Hello  world" {
		t.Errorf("expected 'Hello  world', got %q", ctx.PreContent)
	}
	if ctx.PostContent != "untouched" {
		t.Errorf("PostContent should be untouched, got %q", ctx.PostContent)
	}
}

func TestModifyContentPostPhase(t *testing.T) {
	mc, err := NewModifyContent(PhasePost, ContentRule{
		Find:    `TODO:\s*`,
		Replace: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{
		PreContent:  "untouched",
		PostContent: "TODO: fix this later",
	}
	if err := mc.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PostContent != "fix this later" {
		t.Errorf("expected 'fix this later', got %q", ctx.PostContent)
	}
	if ctx.PreContent != "untouched" {
		t.Errorf("PreContent should be untouched")
	}
}

func TestModifyContentMultipleRules(t *testing.T) {
	mc, err := NewModifyContent(PhasePost,
		ContentRule{Find: `foo`, Replace: "bar"},
		ContentRule{Find: `bar`, Replace: "baz"},
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &TransformContext{PostContent: "foo"}
	if err := mc.Apply(ctx); err != nil {
		t.Fatal(err)
	}

	// foo -> bar -> baz (rules applied sequentially)
	if ctx.PostContent != "baz" {
		t.Errorf("expected 'baz', got %q", ctx.PostContent)
	}
}

func TestModifyContentInvalidPattern(t *testing.T) {
	_, err := NewModifyContent(PhasePost, ContentRule{Find: "[invalid", Replace: "x"})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

func TestModifyContentName(t *testing.T) {
	mc, err := NewModifyContent(PhasePost)
	if err != nil {
		t.Fatal(err)
	}
	if mc.Name() != "modify/content" {
		t.Errorf("expected 'modify/content', got %q", mc.Name())
	}
}
