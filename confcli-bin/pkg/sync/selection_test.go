package sync

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"confcli/pkg/models"
)

func TestSelectedFileKeepsFullLinksButWritesOnlyIt(t *testing.T) {
	profile := basicProfile()
	profile.Tree.FolderPage = "{dir}.md"
	fsys := fstest.MapFS{
		"folder/folder.md": {Data: []byte("changed parent")},
		"folder/one.md":    {Data: []byte("[Other](two.md)\n\n![Included](two.md)")},
		"folder/two.md":    {Data: []byte("changed sibling")},
		"new.md":           {Data: []byte("unrelated new page")},
	}
	refs, err := BuildPathMap(profile, fsys)
	if err != nil {
		t.Fatal(err)
	}
	convert := NewMarkdownConverter(refs, nil, nil, nil)
	engine, err := New(Options{Profile: profile, Locator: &fakeLocator{pages: map[string]*models.Page{
		"folder/folder.md": pageWithLabels(10, "folder", 1),
		"folder/one.md":    pageWithLabels(11, "one", 1),
		"folder/two.md":    pageWithLabels(12, "two", 1),
	}}, Convert: convert, Lister: &fakeLister{pages: []ManagedPage{{PageID: 99, IDLabel: "orphan"}}}})
	if err != nil {
		t.Fatal(err)
	}
	full, err := engine.BuildPlan(context.Background(), fsys, "SP", 7)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectFile(full, `folder\one.md`)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Stats.Update != 1 || selected.Stats.Create != 0 || selected.Stats.Skip != 1 || selected.Stats.Orphan != 0 {
		t.Fatalf("incorrect write scope: %+v", selected.Stats)
	}
	storage := selected.Actions[1].Storage
	if !strings.Contains(storage, `ri:content-title="two"`) || !strings.Contains(storage, `ac:name="include"`) {
		t.Fatalf("unselected link/include target lost: %s", storage)
	}
	parent := selected.Actions[0]
	if parent.Kind != ActionSkip || parent.Storage != "" || len(parent.Images) != 0 || parent.NewHashLabel != "" {
		t.Fatalf("parent can mutate: %+v", parent)
	}
	fc := &execFakeClient{}
	out := newExec(t, fc).Apply(context.Background(), selected)
	if len(out.Errors) != 0 || len(fc.updateCalls) != 1 || fc.updateCalls[0].id != 11 ||
		fc.updateCalls[0].parentID == nil || *fc.updateCalls[0].parentID != 10 || len(fc.createCalls) != 0 {
		t.Fatalf("wrong API writes: %+v, %+v", fc.updateCalls, out)
	}
	for _, label := range fc.addLabel {
		if label.pageID != 11 {
			t.Fatalf("ancestor/sibling label changed: %+v", label)
		}
	}
}

func TestSelectedCreateKeepsExistingParentReadOnly(t *testing.T) {
	full := &Plan{Actions: []Action{
		{Kind: ActionUpdate, RelPath: "parent.md", PageID: 10, Storage: "new parent"},
		{Kind: ActionCreate, RelPath: "child.md", ParentRelPath: "parent.md", Title: "Child", Storage: "child", IDLabel: "child-id", NewHashLabel: "child-hash"},
	}}
	selected, err := SelectFile(full, "child.md")
	if err != nil {
		t.Fatal(err)
	}
	fc := &execFakeClient{}
	out := newExec(t, fc).Apply(context.Background(), selected)
	if len(out.Errors) != 0 || len(fc.createCalls) != 1 || len(fc.updateCalls) != 0 ||
		fc.createCalls[0].parentID == nil || *fc.createCalls[0].parentID != 10 {
		t.Fatalf("unexpected parent creation/update: %+v, %+v", fc, out)
	}
}

func TestSelectedFileRejectsMissingParentsAndUnknownSources(t *testing.T) {
	for _, full := range []*Plan{
		{Actions: []Action{{Kind: ActionCreate, RelPath: "parent.md"}, {Kind: ActionCreate, RelPath: "child.md", ParentRelPath: "parent.md"}}},
		{Actions: []Action{{Kind: ActionUpdate, RelPath: "child.md", ParentRelPath: "missing.md", PageID: 2}}},
		{Actions: []Action{{Kind: ActionSkip, RelPath: "other.md", PageID: 2}}},
	} {
		if _, err := SelectFile(full, "child.md"); err == nil {
			t.Fatal("unsafe/incomplete selection accepted")
		}
	}
	plan, err := SelectFile(&Plan{Actions: []Action{{Kind: ActionSkip, RelPath: "same.md", PageID: 2}}}, "same.md")
	if err != nil || plan.Stats.Skip != 1 || plan.Stats.Create != 0 || plan.Stats.Update != 0 {
		t.Fatalf("repeat must remain unchanged: %+v, %v", plan, err)
	}
}

func TestFileSelectionRejectsAbsoluteTraversalAndPatterns(t *testing.T) {
	for _, value := range []string{"", ".", "../one.md", "a/../one.md", "/one.md", `C:\docs\one.md`, `\\server\one.md`, "*.md", "a?.md", "one.txt", "a//one.md"} {
		if _, err := NormalizeFileSelection(value); err == nil {
			t.Errorf("invalid path accepted: %q", value)
		}
	}
	for _, value := range []string{"./one.md", `Папка\Документ.md`, "folder/one.MD"} {
		if _, err := NormalizeFileSelection(value); err != nil {
			t.Errorf("valid path rejected: %q: %v", value, err)
		}
	}
}
