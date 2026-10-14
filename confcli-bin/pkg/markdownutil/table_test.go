package markdownutil

import (
	"strings"
	"testing"
)

func TestSeparateTableLists(t *testing.T) {
	table := "| Field | Value |\n| :--- | ---: |\n| id | value |"
	for _, tc := range []struct {
		name, source, want string
	}{
		{"resumed ordered list", table + "\n12. Continue\n    - Detail", table + "\n\n12. Continue\n    - Detail"},
		{"correctly nested table", "11. Details:\n\n    " + strings.ReplaceAll(table, "\n", "\n    ") + "\n12. Continue", "11. Details:\n\n    " + strings.ReplaceAll(table, "\n", "\n    ") + "\n12. Continue"},
		{"unordered list", table + "\n- Continue", table + "\n\n- Continue"},
		{"already separated", table + "\n\n12. Continue", table + "\n\n12. Continue"},
		{"pipe in list text", table + "\n12. Use a | b", table + "\n\n12. Use a | b"},
		{"cell text retained", table + "\n| 12. Cell text | - Cell text |", table + "\n| 12. Cell text | - Cell text |"},
		{"not a table", "| Text |\n| not a delimiter |\n12. Continue", "| Text |\n| not a delimiter |\n12. Continue"},
		{"fenced example", "````text\n" + table + "\n12. Continue\n```\n````", "````text\n" + table + "\n12. Continue\n```\n````"},
		{"raw HTML example", "<pre>\n" + table + "\n12. Continue\n</pre>", "<pre>\n" + table + "\n12. Continue\n</pre>"},
		{"indented example", "    " + strings.ReplaceAll(table, "\n", "\n    ") + "\n    12. Continue", "    " + strings.ReplaceAll(table, "\n", "\n    ") + "\n    12. Continue"},
		{"CRLF", strings.ReplaceAll(table+"\n12. Continue\n", "\n", "\r\n"), strings.ReplaceAll(table+"\n\n12. Continue\n", "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SeparateTableLists(tc.source)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if SeparateTableLists(got) != got {
				t.Fatal("repair must be idempotent")
			}
		})
	}
}

func TestNormalizeTableLists(t *testing.T) {
	table := "| Field | Value |\n| --- | --- |\n| id | value |"
	indented := "   " + strings.ReplaceAll(table, "\n", "\n   ")
	for _, tc := range []struct{ name, source, want string }{
		{"continue one list", "1. Add details:\n\n" + table + "\n2. Continue", "1. Add details:\n\n" + indented + "\n2. Continue"},
		{"already separated", "1. Add details:\n\n" + table + "\n\n2. Continue", "1. Add details:\n\n" + indented + "\n\n2. Continue"},
		{"already nested", "1. Add details:\n\n" + indented + "\n2. Continue", "1. Add details:\n\n" + indented + "\n2. Continue"},
		{"nested bullet condition", "1. Details:\n    - If available:\n\n" + table + "\n3. Continue", "1. Details:\n    - If available:\n\n      " + strings.ReplaceAll(table, "\n", "\n      ") + "\n3. Continue"},
		{"bullet continuation in ordered item", "1. Details:\n   - If available:\n\n" + table + "\n\n   - Continue condition\n2. Finish", "1. Details:\n   - If available:\n\n     " + strings.ReplaceAll(table, "\n", "\n     ") + "\n\n   - Continue condition\n2. Finish"},
		{"deeper condition before sibling", "1. Details:\n   - Check:\n     - If available:\n\n" + table + "\n\n   - Continue condition\n2. Finish", "1. Details:\n   - Check:\n     - If available:\n\n       " + strings.ReplaceAll(table, "\n", "\n       ") + "\n\n   - Continue condition\n2. Finish"},
		{"new list", "2. First list ends.\n\n" + table + "\n\n1. New list", "2. First list ends.\n\n" + table + "\n\n1. New list"},
		{"independent paragraph", "1. First list ends.\n\nOther paragraph\n\n" + table + "\n\n2. New list", "1. First list ends.\n\nOther paragraph\n\n" + table + "\n\n2. New list"},
		{"code retained", "````md\n1. Add details:\n\n" + table + "\n2. Continue\n````", "````md\n1. Add details:\n\n" + table + "\n2. Continue\n````"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeTableLists(tc.source)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if NormalizeTableLists(got) != got {
				t.Fatal("normalization must be idempotent")
			}
		})
	}
}
