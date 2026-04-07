package converters

import (
	"strings"
	"testing"
)

func TestGenericMacro_PlainTextBody(t *testing.T) {
	input := `<ac:structured-macro ac:name="plantuml"><ac:plain-text-body>@startuml
Alice -&gt; Bob: Hello
@enduml</ac:plain-text-body></ac:structured-macro>`
	got, err := StorageToMarkdownAdvanced(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "```plantuml") {
		t.Errorf("expected fenced code block with plantuml hint, got:\n%s", got)
	}
	if !strings.Contains(got, "Bob: Hello") {
		t.Errorf("expected body content preserved, got:\n%s", got)
	}
}

func TestGenericMacro_RichTextBody(t *testing.T) {
	input := `<ac:structured-macro ac:name="htmlblock"><ac:rich-text-body><div>Hello World</div></ac:rich-text-body></ac:structured-macro>`
	got, err := StorageToMarkdownAdvanced(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "```htmlblock") {
		t.Errorf("expected fenced code block with htmlblock hint, got:\n%s", got)
	}
	if !strings.Contains(got, "Hello World") {
		t.Errorf("expected body content preserved, got:\n%s", got)
	}
}

func TestGenericMacro_URLParameter(t *testing.T) {
	input := `<ac:structured-macro ac:name="include"><ac:parameter ac:name="url">https://example.com/diagram.puml</ac:parameter></ac:structured-macro>`
	got, err := StorageToMarkdownAdvanced(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "[include](https://example.com/diagram.puml)") {
		t.Errorf("expected markdown link with macro name, got:\n%s", got)
	}
}

func TestGenericMacro_FileRefParameter(t *testing.T) {
	input := `<ac:structured-macro ac:name="drawio"><ac:parameter ac:name="filename">architecture.drawio</ac:parameter></ac:structured-macro>`
	got, err := StorageToMarkdownAdvanced(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "[drawio](architecture.drawio)") {
		t.Errorf("expected markdown link for file ref, got:\n%s", got)
	}
}

func TestGenericMacro_BodyPreferredOverURL(t *testing.T) {
	// When both body and URL parameter exist, body takes precedence
	input := `<ac:structured-macro ac:name="plantuml"><ac:parameter ac:name="url">https://example.com/diagram.puml</ac:parameter><ac:plain-text-body>@startuml
Bob -&gt; Alice
@enduml</ac:plain-text-body></ac:structured-macro>`
	got, err := StorageToMarkdownAdvanced(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "```plantuml") {
		t.Errorf("expected fenced code block (body preferred over URL), got:\n%s", got)
	}
	if strings.Contains(got, "[plantuml]") {
		t.Errorf("body should take precedence over URL link, got:\n%s", got)
	}
}

func TestGenericMacro_NoBodyNoURL(t *testing.T) {
	// Macro with only non-URL parameters should fall through
	input := `<p>Before</p><ac:structured-macro ac:name="somemacro"><ac:parameter ac:name="title">Hello</ac:parameter></ac:structured-macro><p>After</p>`
	got, err := StorageToMarkdownAdvanced(input, "")
	if err != nil {
		t.Fatal(err)
	}
	// Should preserve surrounding content
	if !strings.Contains(got, "Before") || !strings.Contains(got, "After") {
		t.Errorf("surrounding content lost, got:\n%s", got)
	}
}

func TestGenericMacro_KnownMacrosStillWork(t *testing.T) {
	// Existing code macro should still use its specific handler
	input := `<ac:structured-macro ac:name="code"><ac:parameter ac:name="language">go</ac:parameter><ac:plain-text-body>fmt.Println("hi")</ac:plain-text-body></ac:structured-macro>`
	got, err := StorageToMarkdownAdvanced(input, "")
	if err != nil {
		t.Fatal(err)
	}
	// The code handler uses the language parameter, not the macro name
	if !strings.Contains(got, "```go") {
		t.Errorf("expected code macro to use language parameter, got:\n%s", got)
	}
}

func TestLooksLikeURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://example.com/file.puml", true},
		{"http://example.com", true},
		{"/path/to/file.md", true},
		{"diagram.puml", true},
		{"assets/flow.png", true},
		{"just some text", false},
		{"Hello", false},
		{"3", false},
		{"true", false},
	}
	for _, tt := range tests {
		got := looksLikeURL(tt.input)
		if got != tt.want {
			t.Errorf("looksLikeURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
