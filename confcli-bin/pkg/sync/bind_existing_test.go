package sync

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"confcli/pkg/models"
	"confcli/pkg/sync/identity"
	"confcli/pkg/transforms"
)

func existingTestPage(id, parent int, title string, labels ...string) models.Page {
	p := pinnedSnapshotPage(id, "TEST", 1)
	p.Title, p.Labels = title, nil
	if parent != 1 {
		p.Ancestors = append(p.Ancestors, pageWithID(parent))
	}
	for _, name := range labels {
		p.Labels = append(p.Labels, models.Label{Name: name})
	}
	return p
}

func existingTestLocator(pages ...models.Page) *Locator {
	return NewLocator(&pinnedSnapshotFake{fakeClient: fakeClient{expandPage: &models.Page{ID: models.PageID{Value: 1}, Space: models.Space{Key: "TEST"}}}, pages: pages}, nil, 1)
}

func existingTestPlan(t *testing.T, p *transforms.ImportProfile, tree fstest.MapFS, l PageLocator, selected ...string) *Plan {
	t.Helper()
	e, err := New(Options{Profile: p, Locator: l, Convert: passthroughConvert})
	if err != nil {
		t.Fatal(err)
	}
	var plan *Plan
	if len(selected) == 0 {
		plan, err = e.BuildPlan(context.Background(), tree, "TEST", 1)
	} else {
		plan, err = e.BuildSelectedPlan(context.Background(), tree, "TEST", 1, selected)
	}
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestBindExistingPreservesExportedTitlesHierarchyAndLinkMap(t *testing.T) {
	tree := fstest.MapFS{
		"Handbook/Handbook.md":    {Data: []byte("parent")},
		"Handbook/Child_Guide.md": {Data: []byte("child")},
	}
	p := &transforms.ImportProfile{Tree: transforms.TreeConfig{FolderPage: "{dir}.md"}}
	l := existingTestLocator(existingTestPage(2, 1, "Handbook"), existingTestPage(3, 2, "Child. Guide"), existingTestPage(4, 1, "Child-Guide"))
	// Without the flag, unbound pages retain the old create behavior.
	before := existingTestPlan(t, p, tree, l)
	if before.Stats.Create != 2 {
		t.Fatalf("without flag: %+v", before.Stats)
	}
	bound, err := BindExisting(context.Background(), p, tree, l, "TEST", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Overrides) != 0 {
		t.Fatal("source profile was mutated")
	}
	if bound.TitleFor("Handbook/Child_Guide.md") != "Child. Guide" {
		t.Fatal("original title not retained for forward links")
	}
	pathMap, err := BuildPathMap(bound, tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(pathMap) != 2 {
		t.Fatalf("incomplete forward link map: %+v", pathMap)
	}
	if pathMap["Handbook/Child_Guide.md"].Title != "Child. Guide" {
		t.Fatal("forward links lost the destination's actual title")
	}
	plan := existingTestPlan(t, bound, tree, l)
	if plan.Stats.Update != 2 || plan.Stats.Create != 0 || plan.Actions[1].PageID != 3 || plan.Actions[1].ParentRelPath != "Handbook/Handbook.md" {
		t.Fatalf("incorrect adoption: %+v", plan)
	}
	if !strings.Contains(plan.Actions[1].Reason, "bind") {
		t.Fatal("dry-run must explain adoption")
	}
	// Model the result of Executor.Apply, then verify an identical repeat skips.
	var pages []models.Page
	for _, a := range plan.Actions {
		parent := 1
		if a.PageID == 3 {
			parent = 2
		}
		pages = append(pages, existingTestPage(a.PageID, parent, a.Title, a.IDLabel, a.NewHashLabel))
	}
	l = existingTestLocator(pages...)
	bound, err = BindExisting(context.Background(), p, tree, l, "TEST", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if repeat := existingTestPlan(t, bound, tree, l); repeat.Stats.Skip != 2 || repeat.Stats.Update != 0 {
		t.Fatalf("repeat not idempotent: %+v", repeat.Stats)
	}
}

func TestBindExistingNeverAdoptsFromAnotherParentOrMissingBranch(t *testing.T) {
	tree := fstest.MapFS{"Section/Section.md": {}, "Section/Child.md": {}}
	p := &transforms.ImportProfile{Tree: transforms.TreeConfig{FolderPage: "{dir}.md"}}
	l := existingTestLocator(existingTestPage(3, 1, "Child"))
	bound, err := BindExisting(context.Background(), p, tree, l, "TEST", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan := existingTestPlan(t, bound, tree, l); plan.Stats.Create != 2 || plan.Stats.Update != 0 {
		t.Fatalf("page stolen from another branch: %+v", plan)
	}
}

func TestBindExistingRejectsAmbiguityAndOtherBindings(t *testing.T) {
	tree := fstest.MapFS{"Child_Guide.md": {}}
	for name, pages := range map[string][]models.Page{
		"collision":       {existingTestPage(2, 1, "Child. Guide"), existingTestPage(3, 1, "Child-Guide")},
		"bound elsewhere": {existingTestPage(2, 1, "Child Guide", identity.BuildIDLabel("other.md"))},
		"two identities":  {existingTestPage(2, 1, "Child Guide", identity.BuildIDLabel("Child_Guide.md"), identity.BuildIDLabel("other.md"))},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BindExisting(context.Background(), &transforms.ImportProfile{}, tree, existingTestLocator(pages...), "TEST", 1, nil); err == nil {
				t.Fatal("unsafe name match accepted")
			}
		})
	}
}

func TestBindExistingSelectionLeavesUnboundParentsAndSiblingsReadOnly(t *testing.T) {
	tree := fstest.MapFS{"Section/Section.md": {}, "Section/Child_Guide.md": {}, "Section/Other_Guide.md": {}}
	p := &transforms.ImportProfile{Tree: transforms.TreeConfig{FolderPage: "{dir}.md"}}
	l := existingTestLocator(existingTestPage(2, 1, "Section"), existingTestPage(3, 2, "Child. Guide"), existingTestPage(4, 2, "Other Guide"), existingTestPage(5, 2, "Other-Guide"))
	selected := []string{"Section/Child_Guide.md"}
	bound, err := BindExisting(context.Background(), p, tree, l, "TEST", 1, selected)
	if err != nil {
		t.Fatal(err)
	}
	plan := existingTestPlan(t, bound, tree, l, selected...)
	if len(plan.Actions) != 2 || plan.Actions[0].Kind != ActionSkip || plan.Actions[0].PageID != 2 || plan.Actions[1].Kind != ActionUpdate || plan.Actions[1].PageID != 3 {
		t.Fatalf("selection writes outside its scope: %+v", plan)
	}
}

func TestBindExistingRespectsExplicitTitleAndIDAndEscapesGlobPaths(t *testing.T) {
	tree := fstest.MapFS{"Child_[Guide].md": {}}
	p := &transforms.ImportProfile{Overrides: []transforms.PathOverride{{Path: "**", Title: "Custom title", PageID: 2}}}
	l := existingTestLocator(existingTestPage(2, 1, "Custom title"), existingTestPage(3, 1, "Child [Guide]"))
	bound, err := BindExisting(context.Background(), p, tree, l, "TEST", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o := bound.FindOverride("Child_[Guide].md"); o == nil || o.PageID != 2 || o.Title != "Custom title" {
		t.Fatalf("explicit mapping lost: %+v", o)
	}
	if o := bound.FindOverride("Child_G.md"); o == nil || o.Path != "**" {
		t.Fatal("literal brackets became a glob")
	}
	p = &transforms.ImportProfile{Overrides: []transforms.PathOverride{{Path: "**", Title: "Child_[Guide]"}}}
	bound, err = BindExisting(context.Background(), p, tree, l, "TEST", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.Overrides) != 1 {
		t.Fatal("explicit title unexpectedly matched a sanitized title")
	}
}

func TestBindExistingRejectsTwoSourcesClaimingOnePage(t *testing.T) {
	tree := fstest.MapFS{"Child_Guide.md": {}, "Child-Guide.md": {}}
	_, err := BindExisting(context.Background(), &transforms.ImportProfile{}, tree, existingTestLocator(existingTestPage(2, 1, "Child Guide")), "TEST", 1, nil)
	if err == nil || !strings.Contains(err.Error(), "both resolve") {
		t.Fatalf("duplicate destination not detected: %v", err)
	}
}

func TestBindExistingHashWithoutIdentityStillBinds(t *testing.T) {
	tree := fstest.MapFS{"Child.md": {Data: []byte("same")}}
	l := existingTestLocator(existingTestPage(2, 1, "Child", identity.BuildHashLabel("Child", "same", "")))
	p, err := BindExisting(context.Background(), &transforms.ImportProfile{}, tree, l, "TEST", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan := existingTestPlan(t, p, tree, l); plan.Stats.Update != 1 {
		t.Fatal("hash-only page did not receive its identity")
	}
}

func TestBindExistingSelectionListHonorsFlattenAndSkip(t *testing.T) {
	tree := fstest.MapFS{
		"Section/Section.md": {}, "Section/Flat/First.md": {}, "Section/Flat/Second.md": {},
		"Section/Flat/Flat.md": {}, "Section/Ignored.md": {},
	}
	p := &transforms.ImportProfile{Tree: transforms.TreeConfig{
		FolderPage: "{dir}.md", Flatten: []string{"Section/Flat"}, Skip: []string{"**/Ignored.md"},
	}}
	l := existingTestLocator(existingTestPage(2, 1, "Section"), existingTestPage(3, 2, "First"), existingTestPage(4, 2, "Second"))
	selected := []string{"Section/Flat/First.md", "Section/Flat/Second.md"}
	bound, err := BindExisting(context.Background(), p, tree, l, "TEST", 1, selected)
	if err != nil {
		t.Fatal(err)
	}
	plan := existingTestPlan(t, bound, tree, l, selected...)
	if len(plan.Actions) != 3 || plan.Stats.Update != 2 || plan.Stats.Skip != 1 {
		t.Fatalf("wrong selected projection: %+v", plan)
	}
	for _, a := range plan.Actions[1:] {
		if a.ParentRelPath != "Section/Section.md" {
			t.Fatalf("flattened parent lost: %+v", a)
		}
	}
	if _, err := BindExisting(context.Background(), p, tree, l, "TEST", 1, []string{"Section/Ignored.md"}); err == nil {
		t.Fatal("excluded source was selected")
	}
}

func TestFindExistingChildValidatesSpaceAndRoot(t *testing.T) {
	for _, page := range []models.Page{pinnedSnapshotPage(2, "OTHER", 1), pinnedSnapshotPage(2, "TEST", 99)} {
		if _, err := existingTestLocator(page).FindExistingChild(context.Background(), "TEST", 1, "Child", false); err == nil {
			t.Fatal("unsafe destination accepted")
		}
	}
}

func TestExistingTitleMatchesCleanExportRules(t *testing.T) {
	for _, tc := range []struct{ actual, planned string }{
		{"ТЗ. Подсистема / данных", "ТЗ_Подсистема_данных"},
		{"A - B", "a_b"},
		{strings.Repeat("A", 90), strings.Repeat("A", 80)},
		{"Cafe\u0301", "Café"},
	} {
		if !existingTitleMatches(tc.actual, tc.planned, false) {
			t.Errorf("export mismatch: %+v", tc)
		}
	}
}
