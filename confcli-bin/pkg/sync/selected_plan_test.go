package sync

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"testing/fstest"

	"confcli/pkg/models"
)

func TestSelectedPlanningIgnoresUnsealedParentAndSiblingButKeepsTheirLinks(t *testing.T) {
	profile := basicProfile()
	profile.Tree.FolderPage = "{dir}.md"
	marker := base64.StdEncoding.EncodeToString([]byte(`{"macro":"include","title":"Shared","page_id":99,"sha256":"old"}`))
	bad := "<!-- confcli-transclusion:start " + marker + " -->\nEdited snapshot\n<!-- confcli-transclusion:end -->"
	fsys := fstest.MapFS{
		"folder/folder.md": {Data: []byte(bad)},
		"folder/one.md":    {Data: []byte("[Other](two.md)\n\n![Other](two.md)")},
		"folder/two.md":    {Data: []byte(bad)},
		"unrelated.md":     {Data: []byte(bad)},
	}
	refs, err := BuildPathMap(profile, fsys)
	if err != nil {
		t.Fatal(err)
	}
	convert, err := NewProfileMarkdownConverter(profile, refs, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{Profile: profile, Locator: &fakeLocator{pages: map[string]*models.Page{
		"folder/folder.md": pageWithLabels(10, "folder", 1),
		"folder/one.md":    pageWithLabels(11, "one", 1),
	}}, Convert: convert})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.BuildPlan(context.Background(), fsys, "SP", 7); err == nil || !strings.Contains(err.Error(), "unsealed") {
		t.Fatalf("fixture must fail full conversion: %v", err)
	}
	plan, err := engine.BuildSelectedPlan(context.Background(), fsys, "SP", 7, []string{"folder/one.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 || plan.Stats.Update != 1 || plan.Stats.Skip != 1 {
		t.Fatalf("unexpected scope: %+v", plan)
	}
	parent, selected := plan.Actions[0], plan.Actions[1]
	if parent.Kind != ActionSkip || parent.Storage != "" || parent.NewHashLabel != "" || len(parent.Images) != 0 {
		t.Fatalf("ancestor can mutate: %+v", parent)
	}
	if !strings.Contains(selected.Storage, `ri:content-title="two"`) || !strings.Contains(selected.Storage, `ac:name="include"`) {
		t.Fatal(selected.Storage)
	}
	if _, err := engine.BuildSelectedPlan(context.Background(), fsys, "SP", 7, []string{"folder/two.md"}); err == nil || !strings.Contains(err.Error(), "unsealed") {
		t.Fatalf("selected invalid source was ignored: %v", err)
	}
	for _, paths := range [][]string{{"missing.md"}, {"folder/one.md", "folder/one.md"}} {
		if _, err := engine.BuildSelectedPlan(context.Background(), fsys, "SP", 7, paths); err == nil {
			t.Fatalf("invalid selection accepted: %v", paths)
		}
	}
}

func TestSelectedPlanningRejectsMissingAncestor(t *testing.T) {
	profile := basicProfile()
	profile.Tree.FolderPage = "{dir}.md"
	fsys := fstest.MapFS{"folder/folder.md": {Data: []byte("parent")}, "folder/one.md": {Data: []byte("child")}}
	engine, err := New(Options{Profile: profile, Locator: &fakeLocator{}, Convert: NewMarkdownConverter(nil, nil, nil, nil)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.BuildSelectedPlan(context.Background(), fsys, "SP", 7, []string{"folder/one.md"}); err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("missing ancestor accepted: %v", err)
	}
}
