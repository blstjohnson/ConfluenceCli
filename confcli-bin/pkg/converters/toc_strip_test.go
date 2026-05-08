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

// renderedTOCFixture mirrors the shape Confluence emits in export_view HTML
// for a `<ac:structured-macro ac:name="toc">`: a `<div class="toc-macro …">`
// wrapping nested `<ul class="toc-indentation">` link lists.
const renderedTOCFixture = `<p>Before</p>` +
	`<div class='toc-macro rbtoc1778246001968'>` +
	`<ul class='toc-indentation'>` +
	`<li><span class='toc-item'><a href='#A'>A</a></span></li>` +
	`<li><span class='toc-item'><a href='#B'>B</a></span>` +
	`<ul class='toc-indentation'>` +
	`<li><a href='#B-1'>B-1</a></li>` +
	`</ul></li>` +
	`</ul></div>` +
	`<p>After</p>`

func TestStripRenderedTOC_Removed(t *testing.T) {
	got := stripRenderedTOC(renderedTOCFixture)
	if strings.Contains(got, "toc-macro") || strings.Contains(got, "toc-indentation") {
		t.Errorf("rendered TOC not stripped:\n%s", got)
	}
	if !strings.Contains(got, "Before") || !strings.Contains(got, "After") {
		t.Errorf("surrounding content lost:\n%s", got)
	}
}

func TestStripRenderedTOC_DoubleQuotedClass(t *testing.T) {
	// Same shape but with double-quoted class — also seen in some renderings.
	input := `<p>Before</p><div class="toc-macro rbtoc1"><ul class="toc-indentation"><li><a href="#x">x</a></li></ul></div><p>After</p>`
	got := stripRenderedTOC(input)
	if strings.Contains(got, "toc-macro") {
		t.Errorf("double-quoted rendered TOC not stripped:\n%s", got)
	}
	if !strings.Contains(got, "Before") || !strings.Contains(got, "After") {
		t.Errorf("surrounding content lost:\n%s", got)
	}
}

func TestStripRenderedTOC_NestedInLayout(t *testing.T) {
	// Real page 639964318 has the rendered TOC inside a layout cell.
	input := `<ac:layout><ac:layout-section ac:type="two_right_sidebar"><ac:layout-cell>` +
		`<div class='toc-macro rbtoc1'>` +
		`<ul class='toc-indentation'><li><a href='#x'>x</a></li></ul>` +
		`</div></ac:layout-cell><ac:layout-cell><p>Body</p></ac:layout-cell></ac:layout-section></ac:layout>`
	got := stripRenderedTOC(input)
	if strings.Contains(got, "toc-macro") {
		t.Errorf("rendered TOC inside layout not stripped:\n%s", got)
	}
	if !strings.Contains(got, "Body") {
		t.Errorf("non-TOC layout cell content lost:\n%s", got)
	}
}

func TestStripRenderedTOC_NoTOCUnchanged(t *testing.T) {
	// A regular div with a different class must not be touched.
	input := `<div class="content">keep me</div>`
	got := stripRenderedTOC(input)
	if !strings.Contains(got, "keep me") || !strings.Contains(got, `class="content"`) {
		t.Errorf("non-TOC div mistakenly altered:\n%s", got)
	}
}

func TestStripRenderedTOC_PartialClassNotStripped(t *testing.T) {
	// `toc-macro-helper` shares a prefix but isn't the TOC class — must be kept.
	input := `<div class="toc-macro-helper">keep</div>`
	got := stripRenderedTOC(input)
	if !strings.Contains(got, "keep") {
		t.Errorf("partial-class match incorrectly stripped non-TOC div:\n%s", got)
	}
}
