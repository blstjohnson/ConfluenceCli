package md

import (
	"strings"
	"testing"
)

func TestToStorage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string // substrings the output must contain (order not enforced)
		excludes []string // substrings the output must NOT contain
	}{
		{
			name:     "empty input returns empty string",
			input:    "",
			contains: nil,
		},
		{
			name:     "heading renders as h1",
			input:    "# Title",
			contains: []string{`<h1 id="title">Title</h1>`},
		},
		{
			name:     "paragraph with bold and italic",
			input:    "This is **bold** and *italic*.",
			contains: []string{"<p>", "<strong>bold</strong>", "<em>italic</em>", "</p>"},
		},
		{
			name:     "unordered list",
			input:    "- one\n- two\n- three\n",
			contains: []string{"<ul>", "<li>one</li>", "<li>two</li>", "<li>three</li>", "</ul>"},
		},
		{
			name:     "GFM table",
			input:    "| h1 | h2 |\n|----|----|\n| a  | b  |\n",
			contains: []string{"<table>", "<thead>", "<th", "h1", "h2", "<tbody>", "<td", "a", "b"},
		},
		{
			name:     "GFM strikethrough",
			input:    "~~gone~~",
			contains: []string{"<del>gone</del>"},
		},
		{
			name:     "GFM task list",
			input:    "- [x] done\n- [ ] todo\n",
			contains: []string{`type="checkbox"`, "done", "todo"},
		},
		{
			name:     "fenced code with language",
			input:    "```go\nfmt.Println(\"hi\")\n```\n",
			contains: []string{
				`<ac:structured-macro ac:name="code">`,
				`<ac:parameter ac:name="language">go</ac:parameter>`,
				`<ac:plain-text-body><![CDATA[fmt.Println("hi")`,
				`]]></ac:plain-text-body></ac:structured-macro>`,
			},
			excludes: []string{"<pre>", "<code>"},
		},
		{
			name:     "fenced code without language",
			input:    "```\nplain text\n```\n",
			contains: []string{
				`<ac:structured-macro ac:name="code">`,
				`<ac:plain-text-body><![CDATA[plain text`,
			},
			excludes: []string{`<ac:parameter ac:name="language">`},
		},
		{
			name:     "GFM alert NOTE renders as note panel",
			input:    "> [!NOTE]\n> Heads up.\n",
			contains: []string{
				`<ac:structured-macro ac:name="note">`,
				`<ac:rich-text-body>`,
				"Heads up.",
				`</ac:rich-text-body></ac:structured-macro>`,
			},
			excludes: []string{"<blockquote>", "[!NOTE]"},
		},
		{
			name:     "GFM alert WARNING renders as warning panel",
			input:    "> [!WARNING]\n> Careful.\n",
			contains: []string{`<ac:structured-macro ac:name="warning">`, "Careful."},
			excludes: []string{"[!WARNING]"},
		},
		{
			name:     "GFM alert CAUTION maps to warning panel",
			input:    "> [!CAUTION]\n> Bad idea.\n",
			contains: []string{`<ac:structured-macro ac:name="warning">`, "Bad idea."},
		},
		{
			name:     "GFM alert TIP maps to info panel",
			input:    "> [!TIP]\n> Try this.\n",
			contains: []string{`<ac:structured-macro ac:name="info">`, "Try this."},
		},
		{
			name:     "GFM alert IMPORTANT maps to note panel",
			input:    "> [!IMPORTANT]\n> Do not forget.\n",
			contains: []string{`<ac:structured-macro ac:name="note">`, "Do not forget."},
		},
		{
			name:     "plain blockquote without alert marker stays a blockquote",
			input:    "> Just a quote.\n",
			contains: []string{"<blockquote>", "Just a quote."},
			excludes: []string{"ac:structured-macro"},
		},
		{
			name:     "link passes through",
			input:    "[example](https://example.com)",
			contains: []string{`<a href="https://example.com">example</a>`},
		},
		{
			name:  "inline code passes through",
			input: "use `Println` to print",
			contains: []string{
				"<code>Println</code>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToStorage([]byte(tt.input))
			if err != nil {
				t.Fatalf("ToStorage returned error: %v", err)
			}
			if tt.input == "" {
				if got != "" {
					t.Errorf("expected empty output for empty input, got %q", got)
				}
				return
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing expected substring %q\nfull output:\n%s", want, got)
				}
			}
			for _, unwanted := range tt.excludes {
				if strings.Contains(got, unwanted) {
					t.Errorf("output contains unexpected substring %q\nfull output:\n%s", unwanted, got)
				}
			}
		})
	}
}

func TestToStorage_AlertWithMultilineBody(t *testing.T) {
	input := "> [!NOTE]\n> First line.\n> Second line.\n> Third line.\n"
	got, err := ToStorage([]byte(input))
	if err != nil {
		t.Fatalf("ToStorage returned error: %v", err)
	}
	for _, want := range []string{
		`<ac:structured-macro ac:name="note">`,
		"First line.",
		"Second line.",
		"Third line.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "[!NOTE]") {
		t.Errorf("alert marker leaked into output:\n%s", got)
	}
}

func TestToStorage_UnrecognizedAlertMarkerStaysBlockquote(t *testing.T) {
	// Non-standard markers like [!INFO] are not GFM and should not
	// trigger panel macro rendering. The blockquote stays a blockquote
	// and the marker text is preserved verbatim.
	input := "> [!INFO]\n> Body.\n"
	got, err := ToStorage([]byte(input))
	if err != nil {
		t.Fatalf("ToStorage returned error: %v", err)
	}
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("expected <blockquote> for unrecognized marker, got:\n%s", got)
	}
	if !strings.Contains(got, "[!INFO]") {
		t.Errorf("expected marker text preserved, got:\n%s", got)
	}
	if strings.Contains(got, "ac:structured-macro") {
		t.Errorf("did not expect panel macro for unrecognized marker:\n%s", got)
	}
}

func TestToStorage_FencedCodePreservesSpecialChars(t *testing.T) {
	input := "```bash\nif [ \"$x\" -lt 10 ]; then echo \"<small>\"; fi\n```\n"
	got, err := ToStorage([]byte(input))
	if err != nil {
		t.Fatalf("ToStorage returned error: %v", err)
	}
	for _, want := range []string{
		`ac:name="language">bash`,
		`if [ "$x" -lt 10 ]`,
		`echo "<small>"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
