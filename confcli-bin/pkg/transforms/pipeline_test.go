package transforms

import (
	"errors"
	"testing"
)

type mockTransform struct {
	name    string
	apply   func(ctx *TransformContext) error
	called  bool
}

func (m *mockTransform) Name() string { return m.name }
func (m *mockTransform) Apply(ctx *TransformContext) error {
	m.called = true
	if m.apply != nil {
		return m.apply(ctx)
	}
	return nil
}

func TestPipelineRunsInOrder(t *testing.T) {
	var order []string

	t1 := &mockTransform{name: "first", apply: func(ctx *TransformContext) error {
		order = append(order, "first")
		ctx.PostContent += "A"
		return nil
	}}
	t2 := &mockTransform{name: "second", apply: func(ctx *TransformContext) error {
		order = append(order, "second")
		ctx.PostContent += "B"
		return nil
	}}

	p := NewPipeline(t1, t2)
	ctx := &TransformContext{PostContent: ""}
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}

	if ctx.PostContent != "AB" {
		t.Errorf("expected PostContent=AB, got %q", ctx.PostContent)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("expected order [first second], got %v", order)
	}
}

func TestPipelineStopsOnError(t *testing.T) {
	errBoom := errors.New("boom")

	t1 := &mockTransform{name: "failing", apply: func(ctx *TransformContext) error {
		return errBoom
	}}
	t2 := &mockTransform{name: "should-not-run"}

	p := NewPipeline(t1, t2)
	err := p.Run(&TransformContext{})
	if err == nil {
		t.Fatal("expected error")
	}

	var te *TransformError
	if !errors.As(err, &te) {
		t.Fatalf("expected TransformError, got %T", err)
	}
	if te.TransformName != "failing" {
		t.Errorf("expected transform name 'failing', got %q", te.TransformName)
	}
	if !errors.Is(te.Err, errBoom) {
		t.Errorf("expected wrapped error to be errBoom")
	}
	if t2.called {
		t.Error("second transform should not have been called")
	}
}

func TestPipelineEmpty(t *testing.T) {
	p := NewPipeline()
	if err := p.Run(&TransformContext{}); err != nil {
		t.Fatal(err)
	}
	if p.Len() != 0 {
		t.Errorf("expected Len=0, got %d", p.Len())
	}
}

func TestPipelineAppend(t *testing.T) {
	p := NewPipeline()
	p.Append(&mockTransform{name: "a"})
	p.Append(&mockTransform{name: "b"}, &mockTransform{name: "c"})
	if p.Len() != 3 {
		t.Errorf("expected Len=3, got %d", p.Len())
	}
}
