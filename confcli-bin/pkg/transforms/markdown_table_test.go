package transforms

import (
	"strings"
	"testing"
)

func TestMarkTableLines_BasicTable(t *testing.T) {
	src := strings.Join([]string{
		"intro",                              // 0
		"",                                   // 1
		"| col a | col b |",                  // 2 header
		"| --- | --- |",                      // 3 separator
		"| [cfg](app.yaml) | row 1 |",        // 4 body
		"| row two | row 2 |",                // 5 body
		"",                                   // 6 break — ends table
		"after the table [x](after.yaml)",    // 7 normal
	}, "\n")
	got := markTableLines(strings.Split(src, "\n"))
	want := []bool{false, false, true, true, true, true, false, false}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("line %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestMarkTableLines_AlignmentMarkers(t *testing.T) {
	src := strings.Join([]string{
		"| left | center | right |",
		"|:---|:---:|---:|",
	}, "\n")
	got := markTableLines(strings.Split(src, "\n"))
	for i, v := range got {
		if !v {
			t.Errorf("line %d should be in table", i)
		}
	}
}

func TestMarkTableLines_FencedCodeIgnored(t *testing.T) {
	// A separator-looking line inside a code fence must NOT trigger
	// table detection — code blocks can contain ASCII art / dividers.
	src := strings.Join([]string{
		"```",
		"| foo | bar |",
		"|-----|-----|",
		"| a   | b   |",
		"```",
	}, "\n")
	got := markTableLines(strings.Split(src, "\n"))
	for i, v := range got {
		if v {
			t.Errorf("line %d should NOT be in table (inside fence)", i)
		}
	}
}

func TestMarkTableLines_NoTable(t *testing.T) {
	src := "just some prose\nwith no tables\n[link](a.yaml)"
	got := markTableLines(strings.Split(src, "\n"))
	for i, v := range got {
		if v {
			t.Errorf("line %d should NOT be in table", i)
		}
	}
}
