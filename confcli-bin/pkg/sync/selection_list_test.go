package sync

import "testing"

func TestSelectFilesRetainsOnlySelectedWritesAndReadOnlyParents(t *testing.T) {
	full := &Plan{Actions: []Action{
		{Kind: ActionUpdate, RelPath: "parent.md", PageID: 1},
		{Kind: ActionUpdate, RelPath: "a.md", ParentRelPath: "parent.md", PageID: 2},
		{Kind: ActionUpdate, RelPath: "other.md", ParentRelPath: "parent.md", PageID: 3},
		{Kind: ActionUpdate, RelPath: "b.md", ParentRelPath: "parent.md", PageID: 4},
		{Kind: ActionOrphan, PageID: 5},
	}}
	plan, err := SelectFiles(full, []string{"b.md", "a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 3 || plan.Actions[0].Kind != ActionSkip || plan.Actions[1].RelPath != "a.md" || plan.Actions[2].RelPath != "b.md" {
		t.Fatalf("unsafe selection or order: %+v", plan.Actions)
	}
}

func TestSelectFilesRejectsDuplicatePageMappingsAndUnsafePaths(t *testing.T) {
	full := &Plan{Actions: []Action{{Kind: ActionUpdate, RelPath: "a.md", PageID: 2}, {Kind: ActionUpdate, RelPath: "b.md", PageID: 2}}}
	for _, values := range [][]string{nil, {"a.md", "a.md"}, {"a.md", "b.md"}, {"../a.md"}, {"missing.md"}} {
		if _, err := SelectFiles(full, values); err == nil {
			t.Fatalf("unsafe selection accepted: %v", values)
		}
	}
}

func TestSelectFilesStillRejectsMissingAncestorBeforeWrites(t *testing.T) {
	full := &Plan{Actions: []Action{{Kind: ActionCreate, RelPath: "p.md"}, {Kind: ActionUpdate, RelPath: "a.md", ParentRelPath: "p.md", PageID: 2}}}
	if _, err := SelectFiles(full, []string{"a.md"}); err == nil {
		t.Fatal("missing parent accepted")
	}
}
