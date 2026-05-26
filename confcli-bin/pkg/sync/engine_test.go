package sync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"confcli/pkg/models"
	"confcli/pkg/sync/identity"
	"confcli/pkg/transforms"
)

// passthroughConvert returns the markdown unchanged. Tests don't care about
// the storage format, only that the hash is computed consistently.
func passthroughConvert(_ context.Context, md []byte, _ string) (string, error) {
	return string(md), nil
}

// fakeLocator returns pre-seeded pages keyed by relative path. A nil entry
// means "no page exists for this path" (returned as (nil, nil) — the
// not-found signal the real Locator uses).
type fakeLocator struct {
	pages map[string]*models.Page
	err   error
}

func (f *fakeLocator) FindByPath(_ context.Context, _ string, relPath string) (*models.Page, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.pages[relPath], nil
}

// fakeLister returns a fixed managed-page list.
type fakeLister struct {
	pages []ManagedPage
	err   error
}

func (f *fakeLister) ListManaged(_ context.Context, _ string, _ int) ([]ManagedPage, error) {
	return f.pages, f.err
}

func newEngine(t *testing.T, profile *transforms.ImportProfile, loc PageLocator, lister ManagedPageLister) *Engine {
	t.Helper()
	e, err := New(Options{
		Profile: profile,
		Locator: loc,
		Lister:  lister,
		Convert: passthroughConvert,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

func basicProfile() *transforms.ImportProfile {
	return &transforms.ImportProfile{Kind: "import"}
}

func pageWithLabels(id int, title string, version int, labels ...string) *models.Page {
	p := &models.Page{
		ID:      models.PageID{Value: id},
		Title:   title,
		Version: models.Version{Number: version},
	}
	for _, l := range labels {
		p.Labels = append(p.Labels, models.Label{Name: l})
	}
	return p
}

func TestBuildPlan_CreateUpdateSkip(t *testing.T) {
	fsys := fstest.MapFS{
		"new.md":      {Data: []byte("# new")},
		"changed.md":  {Data: []byte("# changed v2")},
		"same.md":     {Data: []byte("# unchanged")},
		"not-md.txt":  {Data: []byte("ignored")},
	}

	// Seed locator: changed.md and same.md exist on the server already.
	oldChangedHash := identity.BuildHashLabel("changed", "# changed v1")
	sameStorage := "# unchanged"
	sameHash := identity.BuildHashLabel("same", sameStorage)

	loc := &fakeLocator{pages: map[string]*models.Page{
		"changed.md": pageWithLabels(101, "changed", 3,
			identity.BuildIDLabel("changed.md"), oldChangedHash),
		"same.md": pageWithLabels(102, "same", 5,
			identity.BuildIDLabel("same.md"), sameHash),
	}}

	e := newEngine(t, basicProfile(), loc, nil)
	plan, err := e.BuildPlan(context.Background(), fsys, "SPACE", 0)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	if got := plan.Stats; got.Create != 1 || got.Update != 1 || got.Skip != 1 || got.Orphan != 0 {
		t.Fatalf("stats = %+v, want create=1 update=1 skip=1 orphan=0", got)
	}

	byPath := map[string]Action{}
	for _, a := range plan.Actions {
		byPath[a.RelPath] = a
	}

	if a := byPath["new.md"]; a.Kind != ActionCreate || a.NewHashLabel == "" || a.Storage != "# new" {
		t.Errorf("new.md: wrong action %+v", a)
	}
	if a := byPath["changed.md"]; a.Kind != ActionUpdate ||
		a.PageID != 101 ||
		a.Version != 3 ||
		a.OldHashLabel != oldChangedHash ||
		a.NewHashLabel == oldChangedHash {
		t.Errorf("changed.md: wrong action %+v", a)
	}
	if a := byPath["same.md"]; a.Kind != ActionSkip ||
		a.PageID != 102 ||
		a.OldHashLabel != sameHash ||
		a.Storage != "" ||
		a.NewHashLabel != "" {
		t.Errorf("same.md: wrong action %+v (storage/newhash should be empty for skip)", a)
	}
}

func TestBuildPlan_OrphanDetection(t *testing.T) {
	fsys := fstest.MapFS{
		"keep.md": {Data: []byte("# keep")},
	}

	keepLabel := identity.BuildIDLabel("keep.md")
	loc := &fakeLocator{pages: map[string]*models.Page{
		"keep.md": pageWithLabels(1, "keep", 1, keepLabel,
			identity.BuildHashLabel("keep", "# keep")),
	}}
	lister := &fakeLister{pages: []ManagedPage{
		{PageID: 1, Title: "keep", IDLabel: keepLabel},
		{PageID: 99, Title: "ghost", IDLabel: "confcli-id-deadbeef"},
	}}

	e := newEngine(t, basicProfile(), loc, lister)
	plan, err := e.BuildPlan(context.Background(), fsys, "SPACE", 42)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	if plan.Stats.Orphan != 1 {
		t.Fatalf("orphan count = %d, want 1", plan.Stats.Orphan)
	}

	var orphan *Action
	for i := range plan.Actions {
		if plan.Actions[i].Kind == ActionOrphan {
			orphan = &plan.Actions[i]
		}
	}
	if orphan == nil {
		t.Fatal("orphan action missing from plan")
	}
	if orphan.PageID != 99 || orphan.IDLabel != "confcli-id-deadbeef" {
		t.Errorf("orphan = %+v", orphan)
	}

	// Orphan must come after the create/skip actions (executor order).
	if plan.Actions[len(plan.Actions)-1].Kind != ActionOrphan {
		t.Errorf("orphans should be appended at the end; got order %v", kinds(plan.Actions))
	}
}

func TestBuildPlan_FolderMarkerParenting(t *testing.T) {
	// docs/README.md -> "docs" page (child of root)
	// docs/intro.md  -> "intro" page (child of docs)
	// docs/api/auth.md -> "auth" (child of root, since api/ has no marker)
	fsys := fstest.MapFS{
		"docs/README.md":   {Data: []byte("# docs root")},
		"docs/intro.md":    {Data: []byte("# intro")},
		"docs/api/auth.md": {Data: []byte("# auth")},
	}

	profile := &transforms.ImportProfile{Kind: "import"}
	profile.Tree.FolderPage = "README.md"

	loc := &fakeLocator{pages: nil} // everything is a create

	e := newEngine(t, profile, loc, nil)
	plan, err := e.BuildPlan(context.Background(), fsys, "SPACE", 0)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	if plan.Stats.Create != 3 {
		t.Fatalf("create count = %d, want 3", plan.Stats.Create)
	}

	parents := map[string]string{}
	order := []string{}
	for _, a := range plan.Actions {
		parents[a.RelPath] = a.ParentRelPath
		order = append(order, a.RelPath)
	}

	if parents["docs/README.md"] != "" {
		t.Errorf("docs/README.md parent = %q, want \"\" (root)", parents["docs/README.md"])
	}
	if parents["docs/intro.md"] != "docs/README.md" {
		t.Errorf("docs/intro.md parent = %q, want docs/README.md", parents["docs/intro.md"])
	}
	if parents["docs/api/auth.md"] != "docs/README.md" {
		t.Errorf("docs/api/auth.md parent = %q, want docs/README.md (no api marker, falls back to docs)", parents["docs/api/auth.md"])
	}

	// Parent must appear before child.
	if indexOf(order, "docs/README.md") > indexOf(order, "docs/intro.md") {
		t.Errorf("docs/README.md must appear before docs/intro.md; got %v", order)
	}
	if indexOf(order, "docs/README.md") > indexOf(order, "docs/api/auth.md") {
		t.Errorf("docs/README.md must appear before docs/api/auth.md; got %v", order)
	}
}

func TestBuildPlan_SkipGlobsExcludeFilesAndFolders(t *testing.T) {
	fsys := fstest.MapFS{
		"keep.md":            {Data: []byte("keep")},
		"drafts/secret.md":   {Data: []byte("secret")},
		"docs/_internal.md":  {Data: []byte("internal")},
		"docs/public.md":     {Data: []byte("public")},
	}

	profile := &transforms.ImportProfile{Kind: "import"}
	profile.Tree.Skip = []string{"drafts", "**/_*.md"}

	loc := &fakeLocator{pages: nil}
	e := newEngine(t, profile, loc, nil)
	plan, err := e.BuildPlan(context.Background(), fsys, "SPACE", 0)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	got := map[string]bool{}
	for _, a := range plan.Actions {
		got[a.RelPath] = true
	}

	wantPresent := []string{"keep.md", "docs/public.md"}
	wantAbsent := []string{"drafts/secret.md", "docs/_internal.md"}

	for _, p := range wantPresent {
		if !got[p] {
			t.Errorf("expected %q in plan, got %v", p, got)
		}
	}
	for _, p := range wantAbsent {
		if got[p] {
			t.Errorf("%q should be skipped, but appeared in plan", p)
		}
	}
}

func TestBuildPlan_FlattenFolder(t *testing.T) {
	// archive/ is flattened: its files attach to root instead of becoming
	// children of a "archive" page.
	fsys := fstest.MapFS{
		"top.md":           {Data: []byte("top")},
		"archive/old.md":   {Data: []byte("old")},
	}

	profile := &transforms.ImportProfile{Kind: "import"}
	profile.Tree.FolderPage = "README.md" // no README in archive
	profile.Tree.Flatten = []string{"archive"}

	loc := &fakeLocator{pages: nil}
	e := newEngine(t, profile, loc, nil)
	plan, err := e.BuildPlan(context.Background(), fsys, "SPACE", 0)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	parents := map[string]string{}
	for _, a := range plan.Actions {
		parents[a.RelPath] = a.ParentRelPath
	}
	if parents["archive/old.md"] != "" {
		t.Errorf("archive/old.md parent = %q, want \"\" (flatten promotes to root)", parents["archive/old.md"])
	}
}

func TestBuildPlan_PerPathSkipOverride(t *testing.T) {
	fsys := fstest.MapFS{
		"keep.md": {Data: []byte("keep")},
		"draft.md": {Data: []byte("draft")},
	}

	profile := &transforms.ImportProfile{Kind: "import"}
	profile.Overrides = []transforms.PathOverride{{Path: "draft.md", Skip: true}}

	loc := &fakeLocator{pages: nil}
	e := newEngine(t, profile, loc, nil)
	plan, err := e.BuildPlan(context.Background(), fsys, "SPACE", 0)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	for _, a := range plan.Actions {
		if a.RelPath == "draft.md" {
			t.Fatalf("draft.md should be skipped via override, but appeared")
		}
	}
	if plan.Stats.Create != 1 {
		t.Errorf("create = %d, want 1", plan.Stats.Create)
	}
}

func TestBuildPlan_LocatorErrorBubbles(t *testing.T) {
	fsys := fstest.MapFS{"a.md": {Data: []byte("a")}}
	loc := &fakeLocator{err: errors.New("network down")}

	e := newEngine(t, basicProfile(), loc, nil)
	_, err := e.BuildPlan(context.Background(), fsys, "SPACE", 0)
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestBuildPlan_ConvertErrorBubbles(t *testing.T) {
	fsys := fstest.MapFS{"a.md": {Data: []byte("bad")}}
	loc := &fakeLocator{pages: nil}

	bad := func(_ context.Context, _ []byte, _ string) (string, error) {
		return "", errors.New("convert boom")
	}
	e, err := New(Options{
		Profile: basicProfile(),
		Locator: loc,
		Convert: bad,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = e.BuildPlan(context.Background(), fsys, "SPACE", 0)
	if err == nil || !strings.Contains(err.Error(), "convert boom") {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestNew_RequiredFields(t *testing.T) {
	good := Options{Profile: basicProfile(), Locator: &fakeLocator{}, Convert: passthroughConvert}

	missing := []struct {
		name string
		opts Options
	}{
		{"profile", Options{Locator: good.Locator, Convert: good.Convert}},
		{"locator", Options{Profile: good.Profile, Convert: good.Convert}},
		{"convert", Options{Profile: good.Profile, Locator: good.Locator}},
	}
	for _, tc := range missing {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts); err == nil {
				t.Fatalf("expected error when %s is nil", tc.name)
			}
		})
	}
}

func kinds(as []Action) []ActionKind {
	out := make([]ActionKind, len(as))
	for i, a := range as {
		out[i] = a.Kind
	}
	return out
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
