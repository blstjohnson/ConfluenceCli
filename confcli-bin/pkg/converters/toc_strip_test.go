package converters

import (
	"strings"
	"testing"
)

func TestStripTOCMacro_TopLevel(t *testing.T) {
	input := `<p>Before</p><ac:structured-macro ac:name="toc"><ac:parameter ac:name="maxLevel">3</ac:parameter></ac:structured-macro><p>After</p>`
	got := stripTOCMacro(input)
	if strings.Contains(got, "toc") {
		t.Errorf("TOC macro not stripped: %s", got)
	}
	if !strings.Contains(got, "Before") || !strings.Contains(got, "After") {
		t.Errorf("surrounding content lost: %s", got)
	}
}

func TestStripTOCMacro_NestedInLayout(t *testing.T) {
	input := `<ac:layout><ac:layout-section ac:type="single"><ac:layout-cell>
<ac:structured-macro ac:name="toc">
  <ac:parameter ac:name="maxLevel">2</ac:parameter>
</ac:structured-macro>
</ac:layout-cell></ac:layout-section></ac:layout>
<p>Content</p>`
	got := stripTOCMacro(input)
	if strings.Contains(got, `ac:name="toc"`) {
		t.Errorf("nested TOC macro not stripped: %s", got)
	}
	if !strings.Contains(got, "Content") {
		t.Errorf("content after layout lost: %s", got)
	}
}

func TestStripTOCMacro_NestedInPanel(t *testing.T) {
	input := `<ac:structured-macro ac:name="info"><ac:rich-text-body>
<ac:structured-macro ac:name="toc"></ac:structured-macro>
<p>Panel text</p>
</ac:rich-text-body></ac:structured-macro>`
	got := stripTOCMacro(input)
	if strings.Contains(got, `ac:name="toc"`) {
		t.Errorf("TOC inside panel not stripped: %s", got)
	}
	if !strings.Contains(got, "Panel text") {
		t.Errorf("panel content lost: %s", got)
	}
	// The outer info macro should remain
	if !strings.Contains(got, `ac:name="info"`) {
		t.Errorf("outer info macro should be preserved: %s", got)
	}
}

func TestStripTOCMacro_NestedInExpand(t *testing.T) {
	input := `<ac:structured-macro ac:name="expand"><ac:rich-text-body>
<ac:structured-macro ac:name="toc"><ac:parameter ac:name="style">circle</ac:parameter></ac:structured-macro>
</ac:rich-text-body></ac:structured-macro>`
	got := stripTOCMacro(input)
	if strings.Contains(got, `ac:name="toc"`) {
		t.Errorf("TOC inside expand not stripped: %s", got)
	}
	if !strings.Contains(got, `ac:name="expand"`) {
		t.Errorf("outer expand macro should be preserved: %s", got)
	}
}

func TestStripTOCMacro_NoTOC(t *testing.T) {
	input := `<p>No TOC here</p><ac:structured-macro ac:name="code"><ac:plain-text-body>x</ac:plain-text-body></ac:structured-macro>`
	got := stripTOCMacro(input)
	if got != input {
		t.Errorf("content without TOC should be unchanged\ngot:  %s\nwant: %s", got, input)
	}
}

func TestStripTOCMacro_MultipleTOCs(t *testing.T) {
	input := `<ac:structured-macro ac:name="toc"></ac:structured-macro><p>Middle</p><ac:structured-macro ac:name="toc"><ac:parameter ac:name="maxLevel">1</ac:parameter></ac:structured-macro>`
	got := stripTOCMacro(input)
	if strings.Contains(got, "toc") {
		t.Errorf("not all TOC macros stripped: %s", got)
	}
	if !strings.Contains(got, "Middle") {
		t.Errorf("middle content lost: %s", got)
	}
}

func TestStripTOCMacro_RegexNotGreedy(t *testing.T) {
	// Ensure non-greedy matching: TOC macro followed by another macro
	input := `<ac:structured-macro ac:name="toc"></ac:structured-macro><ac:structured-macro ac:name="info"><ac:rich-text-body>Keep</ac:rich-text-body></ac:structured-macro>`
	got := stripTOCMacro(input)
	if !strings.Contains(got, `ac:name="info"`) {
		t.Errorf("non-greedy match failed, info macro eaten: %s", got)
	}
	if !strings.Contains(got, "Keep") {
		t.Errorf("info macro content lost: %s", got)
	}
}
