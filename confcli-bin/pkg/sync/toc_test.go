package sync

import (
	"context"
	"strings"
	"testing"

	"confcli/pkg/transforms"
)

func TestTableOfContentsThresholdAndExamples(t *testing.T) {
	for _, tc := range []struct {
		name, source, storage string
		threshold             int
		added                 bool
	}{
		{"disabled", "# A\n## B", "<h1>A</h1>", 0, false},
		{"below", "# A", "<h1>A</h1>", 2, false},
		{"at", "# A\n\nB\n---\n", "<h1>A</h1><h2>B</h2>", 2, true},
		{"code", "# A\n\n```md\n# Example\n```\n\n    # Indented\n", "<h1>A</h1>", 2, false},
		{"existing", "# A", `<ac:structured-macro ac:name="toc"/><h1>A</h1>`, 1, true},
		{"literal", "# A", `<ac:structured-macro ac:name="code"><ac:plain-text-body><![CDATA[<ac:structured-macro ac:name="toc"/>]]></ac:plain-text-body></ac:structured-macro>`, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := addTableOfContents([]byte(tc.source), tc.storage, tc.threshold)
			if err != nil {
				t.Fatal(err)
			}
			if (out != tc.storage) != tc.added {
				t.Fatalf("unexpected TOC: %s", out)
			}
			if tc.added && (!strings.HasPrefix(out, `<ac:structured-macro ac:name="expand"`) || !strings.Contains(out, `<ac:parameter ac:name="title">Оглавление</ac:parameter><ac:rich-text-body><ac:structured-macro ac:name="toc"`)) {
				t.Fatal(out)
			}
			if again, err := addTableOfContents([]byte(tc.source), out, tc.threshold); err != nil || again != out {
				t.Fatalf("not idempotent: %s", again)
			}
		})
	}
}

func TestProfileTOCAndIncludePosition(t *testing.T) {
	profile, err := transforms.LoadImportProfile([]byte("kind: import\ntoc:\n  min_headings: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	convert, err := NewProfileMarkdownConverter(profile, map[string]transforms.PageRef{"nested/Other_(old).md": {Title: "Other (old)"}}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := convert(context.Background(), []byte("# Before\n\n![Other](nested/Other_(old).md)\n\n## After\n"), "source.md")
	if err != nil {
		t.Fatal(err)
	}
	assertStorageXML(t, out)
	if !strings.HasPrefix(out, `<ac:structured-macro ac:name="expand"`) || strings.Count(out, `ac:name="include"`) != 1 || !strings.Contains(out, `ri:content-title="Other (old)"`) {
		t.Fatal(out)
	}
	if !(strings.Index(out, "Before") < strings.Index(out, `ac:name="include"`) && strings.Index(out, `ac:name="include"`) < strings.Index(out, "After")) {
		t.Fatal(out)
	}
	if _, err := transforms.LoadImportProfile([]byte("kind: import\ntoc: {min_headings: -1}")); err == nil {
		t.Fatal("negative threshold accepted")
	}
}

func TestTOCFiveHeadingPolicyReplacesExistingNavigation(t *testing.T) {
	old := `<ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">Оглавление</ac:parameter><ac:rich-text-body><ac:structured-macro ac:name="toc"/></ac:rich-text-body></ac:structured-macro>`
	for _, headings := range []int{2, 3, 4, 5, 6} {
		source := strings.Repeat("## Section\n\n", headings)
		content := strings.Repeat("<h2>Section</h2>", headings)
		out, err := addTableOfContents([]byte(source), content+old, 5)
		if err != nil {
			t.Fatal(err)
		}
		if headings < 5 {
			if out != content {
				t.Fatalf("%d headings must have no navigation: %s", headings, out)
			}
		} else if !strings.HasPrefix(out, `<ac:structured-macro ac:name="expand"`) || strings.Count(out, `ac:name="toc"`) != 1 || !strings.HasSuffix(out, content) {
			t.Fatalf("%d headings must have one leading TOC: %s", headings, out)
		}
	}
	if out, err := addTableOfContents([]byte("## Short"), old, 0); err != nil || out != old {
		t.Fatalf("disabled policy changed an authored TOC: %s, %v", out, err)
	}
}
