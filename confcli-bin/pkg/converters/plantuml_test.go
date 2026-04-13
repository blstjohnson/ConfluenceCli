package converters

import (
	"strings"
	"testing"
)

func TestHasPlantUMLImages(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{
			name: "img with plantuml servlet URL",
			html: `<p>Text</p><img src="https://conf.example.com/wiki/plugins/servlet/plantuml?cachedDiagram=abc123" />`,
			want: true,
		},
		{
			name: "img with plantuml in data URL",
			html: `<span><img src="/rest/plantuml/1.0/render?source=abc" alt="diagram"></span>`,
			want: true,
		},
		{
			name: "case insensitive match",
			html: `<img src="https://example.com/PlantUML/render?id=1" />`,
			want: true,
		},
		{
			name: "no plantuml images",
			html: `<p>Hello</p><img src="https://example.com/image.png" /><img src="/wiki/download/attachments/123/screenshot.jpg" />`,
			want: false,
		},
		{
			name: "empty html",
			html: "",
			want: false,
		},
		{
			name: "plantuml in text but not in img src",
			html: `<p>This page uses plantuml</p><img src="https://example.com/other.png" />`,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasPlantUMLImages(tt.html)
			if got != tt.want {
				t.Errorf("HasPlantUMLImages() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractPlantUMLBlocks(t *testing.T) {
	tests := []struct {
		name    string
		storage string
		want    []string
	}{
		{
			name: "single plantuml macro",
			storage: `<p>Before</p>
<ac:structured-macro ac:name="plantuml" ac:schema-version="1">
<ac:plain-text-body>@startuml
Alice -> Bob: Hello
@enduml</ac:plain-text-body>
</ac:structured-macro>
<p>After</p>`,
			want: []string{"@startuml\nAlice -> Bob: Hello\n@enduml"},
		},
		{
			name: "multiple plantuml macros",
			storage: `<ac:structured-macro ac:name="plantuml">
<ac:plain-text-body>@startuml
A -> B
@enduml</ac:plain-text-body>
</ac:structured-macro>
<p>Middle</p>
<ac:structured-macro ac:name="plantuml">
<ac:plain-text-body>@startuml
C -> D
@enduml</ac:plain-text-body>
</ac:structured-macro>`,
			want: []string{
				"@startuml\nA -> B\n@enduml",
				"@startuml\nC -> D\n@enduml",
			},
		},
		{
			name: "with CDATA wrapping",
			storage: `<ac:structured-macro ac:name="plantuml">
<ac:plain-text-body><![CDATA[@startuml
X -> Y: test
@enduml]]></ac:plain-text-body>
</ac:structured-macro>`,
			want: []string{"@startuml\nX -> Y: test\n@enduml"},
		},
		{
			name: "with parameters",
			storage: `<ac:structured-macro ac:name="plantuml" ac:schema-version="1" ac:macro-id="abc123">
<ac:parameter ac:name="format">svg</ac:parameter>
<ac:plain-text-body>@startuml
Bob -> Alice
@enduml</ac:plain-text-body>
</ac:structured-macro>`,
			want: []string{"@startuml\nBob -> Alice\n@enduml"},
		},
		{
			name: "no plantuml macros",
			storage: `<ac:structured-macro ac:name="code"><ac:plain-text-body>fmt.Println("hi")</ac:plain-text-body></ac:structured-macro>`,
			want: []string{},
		},
		{
			name: "empty storage",
			storage: "",
			want:    []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPlantUMLBlocks(tt.storage)
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractPlantUMLBlocks() returned %d blocks, want %d\ngot: %v", len(got), len(tt.want), got)
			}
			for i, block := range got {
				if block != tt.want[i] {
					t.Errorf("block[%d] = %q, want %q", i, block, tt.want[i])
				}
			}
		})
	}
}

func TestReplacePlantUMLImages(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		blocks   []string
		check    func(t *testing.T, result string)
	}{
		{
			name:     "single image replaced",
			markdown: "# Title\n\n![diagram](https://conf.example.com/plugins/servlet/plantuml?cached=abc)\n\nSome text after.",
			blocks:   []string{"@startuml\nAlice -> Bob\n@enduml"},
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "```plantuml\n@startuml\nAlice -> Bob\n@enduml\n```") {
					t.Errorf("expected plantuml code block, got:\n%s", result)
				}
				if strings.Contains(result, "![diagram]") {
					t.Errorf("expected image reference to be removed, got:\n%s", result)
				}
				if !strings.Contains(result, "# Title") {
					t.Errorf("expected title preserved, got:\n%s", result)
				}
				if !strings.Contains(result, "Some text after.") {
					t.Errorf("expected surrounding text preserved, got:\n%s", result)
				}
			},
		},
		{
			name: "multiple images replaced in order",
			markdown: "![](https://conf/plantuml?a=1)\n\nMiddle text\n\n![](https://conf/plantuml?a=2)\n",
			blocks:   []string{"@startuml\nA -> B\n@enduml", "@startuml\nC -> D\n@enduml"},
			check: func(t *testing.T, result string) {
				idx1 := strings.Index(result, "A -> B")
				idx2 := strings.Index(result, "C -> D")
				if idx1 < 0 || idx2 < 0 {
					t.Fatalf("expected both blocks, got:\n%s", result)
				}
				if idx1 >= idx2 {
					t.Errorf("blocks should be in order, but first=%d, second=%d", idx1, idx2)
				}
				if !strings.Contains(result, "Middle text") {
					t.Errorf("expected middle text preserved, got:\n%s", result)
				}
			},
		},
		{
			name:     "more blocks than images appends extras",
			markdown: "![](https://example.com/plantuml?x=1)\n",
			blocks:   []string{"@startuml\nFirst\n@enduml", "@startuml\nSecond\n@enduml"},
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "First") {
					t.Errorf("expected first block, got:\n%s", result)
				}
				if !strings.Contains(result, "Second") {
					t.Errorf("expected second block appended, got:\n%s", result)
				}
			},
		},
		{
			name:     "no blocks returns markdown unchanged",
			markdown: "# Title\n\n![](https://example.com/plantuml?x=1)\n",
			blocks:   []string{},
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "![](https://example.com/plantuml?x=1)") {
					t.Errorf("expected markdown unchanged, got:\n%s", result)
				}
			},
		},
		{
			name:     "non-plantuml images left intact",
			markdown: "![screenshot](https://example.com/image.png)\n\n![](https://conf/plantuml?d=1)\n",
			blocks:   []string{"@startuml\nX -> Y\n@enduml"},
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "![screenshot](https://example.com/image.png)") {
					t.Errorf("expected non-plantuml image preserved, got:\n%s", result)
				}
				if !strings.Contains(result, "```plantuml") {
					t.Errorf("expected plantuml code block, got:\n%s", result)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplacePlantUMLImages(tt.markdown, tt.blocks)
			tt.check(t, result)
		})
	}
}

func TestStripJunkImages(t *testing.T) {
	// Junk images (relative paths, emoticons) should be stripped
	junkInput := "Text before\n\n![smile](/images/icons/emoticons/smile.png)\n\nText after"
	result := StripJunkImages(junkInput)
	if strings.Contains(result, "![smile]") {
		t.Errorf("expected junk images stripped, got:\n%s", result)
	}
	if !strings.Contains(result, "Text before") || !strings.Contains(result, "Text after") {
		t.Errorf("expected surrounding text preserved, got:\n%s", result)
	}

	// External https images should be preserved
	goodInput := "Text\n\n![diagram](https://example.com/img.png)\n\nMore"
	result2 := StripJunkImages(goodInput)
	if !strings.Contains(result2, "![diagram](https://example.com/img.png)") {
		t.Errorf("expected external image preserved, got:\n%s", result2)
	}

	// Real Confluence attachments should be preserved
	attachInput := "![file](/download/attachments/12345/screenshot.png)"
	result3 := StripJunkImages(attachInput)
	if !strings.Contains(result3, "![file]") {
		t.Errorf("expected attachment image preserved, got:\n%s", result3)
	}
}

func TestExportViewToMarkdownKeepImages(t *testing.T) {
	html := `<p>Hello</p><img src="https://example.com/test.png" alt="test" /><p>World</p>`
	md, err := ExportViewToMarkdownKeepImages(html, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "![test]") {
		t.Errorf("expected image reference preserved in keep-images variant, got:\n%s", md)
	}
	if !strings.Contains(md, "Hello") || !strings.Contains(md, "World") {
		t.Errorf("expected text preserved, got:\n%s", md)
	}
}
