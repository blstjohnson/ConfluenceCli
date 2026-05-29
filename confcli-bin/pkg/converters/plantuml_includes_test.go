package converters

import (
	"fmt"
	"reflect"
	"testing"
)

func TestExtractIncludeTargets(t *testing.T) {
	tests := []struct {
		name    string
		storage string
		want    []IncludeTarget
	}{
		{
			name: "include without space-key",
			storage: `<p><ac:structured-macro ac:name="include" ac:schema-version="1">
<ac:parameter ac:name=""><ac:link><ri:page ri:content-title="Target Page" /></ac:link></ac:parameter>
</ac:structured-macro></p>`,
			want: []IncludeTarget{{Title: "Target Page"}},
		},
		{
			name: "include with space-key",
			storage: `<ac:structured-macro ac:name="include">
<ac:parameter ac:name=""><ac:link><ri:page ri:space-key="DOCS" ri:content-title="Other" /></ac:link></ac:parameter>
</ac:structured-macro>`,
			want: []IncludeTarget{{Title: "Other", SpaceKey: "DOCS"}},
		},
		{
			name: "title with HTML entities is unescaped",
			storage: `<ac:structured-macro ac:name="include">
<ac:parameter ac:name=""><ac:link><ri:page ri:content-title="RU.1 &quot;LookUpPush&quot;" /></ac:link></ac:parameter>
</ac:structured-macro>`,
			want: []IncludeTarget{{Title: `RU.1 "LookUpPush"`}},
		},
		{
			name: "excerpt-include is recognised",
			storage: `<ac:structured-macro ac:name="excerpt-include">
<ac:parameter ac:name=""><ac:link><ri:page ri:content-title="Snippet Source" /></ac:link></ac:parameter>
</ac:structured-macro>`,
			want: []IncludeTarget{{Title: "Snippet Source"}},
		},
		{
			name: "plantuml macros are ignored",
			storage: `<ac:structured-macro ac:name="plantuml"><ac:plain-text-body>@startuml
A->B
@enduml</ac:plain-text-body></ac:structured-macro>`,
			want: nil,
		},
		{
			name: "multiple includes preserve order",
			storage: `<ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-title="First" /></ac:link></ac:parameter></ac:structured-macro>
<p>middle</p>
<ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:space-key="X" ri:content-title="Second" /></ac:link></ac:parameter></ac:structured-macro>`,
			want: []IncludeTarget{
				{Title: "First"},
				{Title: "Second", SpaceKey: "X"},
			},
		},
		{
			name:    "empty storage",
			storage: "",
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractIncludeTargets(tt.storage)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractIncludeTargets() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// fakeFetcher returns canned storage per (space|title) key.
type fakeFetcher struct {
	pages map[string]string // key: "space|title" — space "" means any
	calls int
}

func (f *fakeFetcher) fetch(t IncludeTarget, defaultSpace string) (string, string, error) {
	f.calls++
	space := t.SpaceKey
	if space == "" {
		space = defaultSpace
	}
	if s, ok := f.pages[space+"|"+t.Title]; ok {
		return s, space, nil
	}
	if s, ok := f.pages["|"+t.Title]; ok {
		return s, space, nil
	}
	return "", space, fmt.Errorf("page not found: space=%q title=%q", space, t.Title)
}

func plantumlMacro(body string) string {
	return `<ac:structured-macro ac:name="plantuml"><ac:plain-text-body>` + body + `</ac:plain-text-body></ac:structured-macro>`
}
func includeMacro(title string) string {
	return `<ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-title="` + title + `" /></ac:link></ac:parameter></ac:structured-macro>`
}

func TestExtractPlantUMLBlocksWithIncludes_OrderPreserved(t *testing.T) {
	// Parent has: inline plantuml #1, include(Sub), inline plantuml #4.
	// Sub has:    plantuml #2, plantuml #3.
	// Expected flatten order: 1, 2, 3, 4.
	parent := plantumlMacro("@startuml\n1\n@enduml") +
		`<p>middle</p>` + includeMacro("Sub") +
		`<p>tail</p>` + plantumlMacro("@startuml\n4\n@enduml")
	sub := plantumlMacro("@startuml\n2\n@enduml") +
		plantumlMacro("@startuml\n3\n@enduml")
	f := &fakeFetcher{pages: map[string]string{"SPACE|Sub": sub}}

	got := ExtractPlantUMLBlocksWithIncludes(parent, "SPACE", f.fetch, DefaultIncludeMaxDepth, nil)
	want := []string{
		"@startuml\n1\n@enduml",
		"@startuml\n2\n@enduml",
		"@startuml\n3\n@enduml",
		"@startuml\n4\n@enduml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order wrong:\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestExtractPlantUMLBlocksWithIncludes_NestedIncludes(t *testing.T) {
	// Parent -> A -> B (B has a plantuml block).
	parent := includeMacro("A")
	a := plantumlMacro("@startuml\nfromA\n@enduml") + includeMacro("B")
	b := plantumlMacro("@startuml\nfromB\n@enduml")
	f := &fakeFetcher{pages: map[string]string{"S|A": a, "S|B": b}}

	got := ExtractPlantUMLBlocksWithIncludes(parent, "S", f.fetch, DefaultIncludeMaxDepth, nil)
	want := []string{
		"@startuml\nfromA\n@enduml",
		"@startuml\nfromB\n@enduml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nested include flatten wrong:\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestExtractPlantUMLBlocksWithIncludes_CycleTerminates(t *testing.T) {
	// A includes B, B includes A. Each carries one plantuml block — we should
	// emit each block exactly once, never loop forever.
	a := plantumlMacro("@startuml\nA\n@enduml") + includeMacro("B")
	b := plantumlMacro("@startuml\nB\n@enduml") + includeMacro("A")
	parent := includeMacro("A")
	f := &fakeFetcher{pages: map[string]string{"S|A": a, "S|B": b}}

	got := ExtractPlantUMLBlocksWithIncludes(parent, "S", f.fetch, DefaultIncludeMaxDepth, nil)
	want := []string{
		"@startuml\nA\n@enduml",
		"@startuml\nB\n@enduml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cycle handling wrong:\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestExtractPlantUMLBlocksWithIncludes_DepthCap(t *testing.T) {
	// Chain longer than the cap: parent -> A -> B -> C (each has a block).
	// With maxDepth=1, only blocks reachable through one fetch (A's own block,
	// not B's or C's) should appear.
	a := plantumlMacro("@startuml\nA\n@enduml") + includeMacro("B")
	b := plantumlMacro("@startuml\nB\n@enduml") + includeMacro("C")
	c := plantumlMacro("@startuml\nC\n@enduml")
	parent := includeMacro("A")
	f := &fakeFetcher{pages: map[string]string{"S|A": a, "S|B": b, "S|C": c}}

	got := ExtractPlantUMLBlocksWithIncludes(parent, "S", f.fetch, 1, nil)
	want := []string{"@startuml\nA\n@enduml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("depth cap wrong:\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestExtractPlantUMLBlocksWithIncludes_NilFetcher(t *testing.T) {
	// With nil fetch, include macros become no-ops; only own-page plantuml
	// blocks are returned.
	storage := plantumlMacro("@startuml\nown\n@enduml") + includeMacro("Other")
	got := ExtractPlantUMLBlocksWithIncludes(storage, "S", nil, DefaultIncludeMaxDepth, nil)
	want := []string{"@startuml\nown\n@enduml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nil-fetcher behaviour wrong:\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestCountPlantUMLImages(t *testing.T) {
	html := `<p>x</p><img src="/download/export/plantuml1.png" />
<img src="/download/export/plantuml2.png" />
<img src="/wiki/download/attachments/12/screenshot.png" />`
	got := CountPlantUMLImages(html)
	if got != 2 {
		t.Errorf("CountPlantUMLImages() = %d, want 2", got)
	}
}
