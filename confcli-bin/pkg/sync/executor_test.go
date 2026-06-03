package sync

import (
	"context"
	"errors"
	"strings"
	"testing"

	"confcli/pkg/api"
	"confcli/pkg/models"
)

// execFakeClient is a minimal api.Client double covering only the methods
// Executor calls. Unused methods inherit nil-panic behavior from the
// embedded interface so accidental dependencies are loud, not silent.
type execFakeClient struct {
	api.Client

	createCalls []createCall
	updateCalls []updateCall
	addLabel    []labelCall
	removeLabel []labelCall
	uploads     []uploadCall

	createErr error
	updateErr error
	labelErr  error
	uploadErr error

	nextPageID int
}

type uploadCall struct {
	pageID   int
	filename string
	data     []byte
	mime     string
}

type createCall struct {
	spaceKey string
	parentID *int
	title    string
	content  string
	format   string
}

type updateCall struct {
	id       int
	content  string
	comment  string
	format   string
	parentID *int
}

type labelCall struct {
	pageID int
	name   string
}

func (f *execFakeClient) CreatePage(_ context.Context, spaceKey string, parentID *int, title, content, format string) (*models.Page, error) {
	f.createCalls = append(f.createCalls, createCall{spaceKey, parentID, title, content, format})
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.nextPageID++
	return &models.Page{ID: models.PageID{Value: f.nextPageID + 1000}, Title: title}, nil
}

func (f *execFakeClient) UpdatePage(_ context.Context, id int, content, comment, format string, parentID *int) (*models.Page, error) {
	f.updateCalls = append(f.updateCalls, updateCall{id, content, comment, format, parentID})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &models.Page{ID: models.PageID{Value: id}}, nil
}

func (f *execFakeClient) UploadAttachment(_ context.Context, pageID int, filename string, data []byte, mime string) error {
	f.uploads = append(f.uploads, uploadCall{pageID, filename, data, mime})
	return f.uploadErr
}

func (f *execFakeClient) AddLabel(_ context.Context, pageID int, name string) error {
	f.addLabel = append(f.addLabel, labelCall{pageID, name})
	return f.labelErr
}

func (f *execFakeClient) RemoveLabel(_ context.Context, pageID int, name string) error {
	f.removeLabel = append(f.removeLabel, labelCall{pageID, name})
	return f.labelErr
}

func newExec(t *testing.T, fc api.Client) *Executor {
	t.Helper()
	x, err := NewExecutor(ExecutorOptions{
		Client:     fc,
		SpaceKey:   "SP",
		RootPageID: 7,
	})
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	return x
}

func TestExecutor_ApplyCreateUsesRootForEmptyParent(t *testing.T) {
	fc := &execFakeClient{}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{
			Kind:         ActionCreate,
			RelPath:      "a.md",
			Title:        "a",
			IDLabel:      "confcli-id-aaa",
			NewHashLabel: "confcli-hash-bbb",
			Storage:      "<p>a</p>",
		},
	}}
	out := x.Apply(context.Background(), plan)
	if out.Created != 1 || len(out.Errors) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if len(fc.createCalls) != 1 {
		t.Fatalf("create calls = %d, want 1", len(fc.createCalls))
	}
	c := fc.createCalls[0]
	if c.parentID == nil || *c.parentID != 7 {
		t.Fatalf("create parent = %v, want pointer to 7 (root)", c.parentID)
	}
	if c.format != "storage" {
		t.Fatalf("create format = %q, want storage", c.format)
	}
	// Both labels added; no removes since this is a fresh create.
	if len(fc.addLabel) != 2 || len(fc.removeLabel) != 0 {
		t.Fatalf("labels: add=%v remove=%v", fc.addLabel, fc.removeLabel)
	}
}

func TestExecutor_ApplyCreateThenChildResolvesParent(t *testing.T) {
	fc := &execFakeClient{}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{Kind: ActionCreate, RelPath: "docs/README.md", Title: "docs",
			IDLabel: "id-1", NewHashLabel: "hash-1"},
		{Kind: ActionCreate, RelPath: "docs/intro.md", ParentRelPath: "docs/README.md",
			Title: "intro", IDLabel: "id-2", NewHashLabel: "hash-2"},
	}}
	out := x.Apply(context.Background(), plan)
	if out.Created != 2 || len(out.Errors) != 0 {
		t.Fatalf("outcome = %+v", out)
	}

	// First create: parent = root (7). Returned page id = 1001.
	// Second create's parent must be the first create's returned id.
	if *fc.createCalls[0].parentID != 7 {
		t.Fatalf("first create parent = %d, want 7", *fc.createCalls[0].parentID)
	}
	if *fc.createCalls[1].parentID != 1001 {
		t.Fatalf("second create parent = %d, want 1001 (first child's id)", *fc.createCalls[1].parentID)
	}
}

func TestExecutor_ChildSkippedWhenParentCreateFails(t *testing.T) {
	fc := &execFakeClient{createErr: errors.New("boom")}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{Kind: ActionCreate, RelPath: "docs/README.md", Title: "docs", IDLabel: "id-1"},
		{Kind: ActionCreate, RelPath: "docs/intro.md", ParentRelPath: "docs/README.md", Title: "intro", IDLabel: "id-2"},
	}}
	out := x.Apply(context.Background(), plan)

	if out.Created != 0 {
		t.Fatalf("expected zero successful creates, got %d", out.Created)
	}
	if len(out.Errors) != 2 {
		t.Fatalf("expected two errors (parent create + orphaned child), got %d: %+v", len(out.Errors), out.Errors)
	}
	if !strings.Contains(out.Errors[1].Err.Error(), "parent") {
		t.Errorf("second error should mention parent: %v", out.Errors[1].Err)
	}
}

func TestExecutor_UpdateRemovesOldHashOnlyIfDifferent(t *testing.T) {
	fc := &execFakeClient{}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{
			Kind:         ActionUpdate,
			RelPath:      "a.md",
			Title:        "a",
			PageID:       42,
			Version:      3,
			IDLabel:      "id-1",
			OldHashLabel: "hash-old",
			NewHashLabel: "hash-new",
			Storage:      "<p>v2</p>",
		},
	}}
	out := x.Apply(context.Background(), plan)
	if out.Updated != 1 || len(out.Errors) != 0 {
		t.Fatalf("outcome = %+v", out)
	}

	// Identity label is unchanged → only re-added (idempotent), not removed.
	for _, r := range fc.removeLabel {
		if r.name == "id-1" {
			t.Errorf("identity label should not be removed when unchanged; got %+v", fc.removeLabel)
		}
	}
	// Old hash must be removed since it differs from new.
	found := false
	for _, r := range fc.removeLabel {
		if r.name == "hash-old" {
			found = true
		}
	}
	if !found {
		t.Errorf("old hash label should be removed; got removes %+v", fc.removeLabel)
	}
}

func TestExecutor_UpdateReparentsTopLevelUnderRoot(t *testing.T) {
	// Regression for the "--root ignored" bug: an existing top-level page is
	// reparented under the sync root on update, so a page that predates --root
	// (or drifted to the space root) is pulled back under the tree.
	fc := &execFakeClient{}
	x := newExec(t, fc) // root = 7

	plan := &Plan{Actions: []Action{
		{
			Kind: ActionUpdate, RelPath: "a.md", Title: "a", PageID: 42, Version: 2,
			IDLabel: "id", OldHashLabel: "old", NewHashLabel: "new", Storage: "<p>x</p>",
		},
	}}
	out := x.Apply(context.Background(), plan)
	if out.Updated != 1 || len(out.Errors) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if len(fc.updateCalls) != 1 {
		t.Fatalf("update calls = %d, want 1", len(fc.updateCalls))
	}
	if u := fc.updateCalls[0]; u.parentID == nil || *u.parentID != 7 {
		t.Errorf("update parent = %v, want pointer to 7 (root)", u.parentID)
	}
}

func TestExecutor_UpdateReparentsUnderResolvedParent(t *testing.T) {
	// A nested page is reparented under its (already-existing) folder-marker
	// parent, resolved from the plan's pre-populated id map.
	fc := &execFakeClient{}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{Kind: ActionSkip, RelPath: "docs/README.md", PageID: 555, Title: "docs"},
		{
			Kind: ActionUpdate, RelPath: "docs/intro.md", ParentRelPath: "docs/README.md",
			PageID: 600, Version: 1, IDLabel: "id2", OldHashLabel: "o", NewHashLabel: "n",
			Storage: "<p>i</p>",
		},
	}}
	out := x.Apply(context.Background(), plan)
	if out.Updated != 1 || out.Skipped != 1 || len(out.Errors) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if u := fc.updateCalls[0]; u.id != 600 || u.parentID == nil || *u.parentID != 555 {
		t.Errorf("update = %+v, want id=600 parent=555", u)
	}
}

func TestExecutor_UpdateWithUnresolvedParentUpdatesContentOnly(t *testing.T) {
	// When the parent's create fails earlier in the run, the child's content
	// update must still happen (parent omitted) rather than being blocked.
	fc := &execFakeClient{createErr: errors.New("boom")}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{Kind: ActionCreate, RelPath: "docs/README.md", Title: "docs", IDLabel: "id1", NewHashLabel: "h1"},
		{
			Kind: ActionUpdate, RelPath: "docs/intro.md", ParentRelPath: "docs/README.md",
			PageID: 600, Version: 1, IDLabel: "id2", OldHashLabel: "o", NewHashLabel: "n",
			Storage: "<p>i</p>",
		},
	}}
	out := x.Apply(context.Background(), plan)
	// Parent create failed (1 error); child content still updated.
	if out.Updated != 1 {
		t.Errorf("Updated = %d, want 1", out.Updated)
	}
	if len(out.Errors) != 1 {
		t.Fatalf("Errors = %d, want 1 (the failed parent create)", len(out.Errors))
	}
	if u := fc.updateCalls[0]; u.id != 600 || u.parentID != nil {
		t.Errorf("update = %+v, want id=600 with nil parent (no reparent)", u)
	}
}

func TestExecutor_SkipParentResolvesChildOnCreate(t *testing.T) {
	// Regression: when a parent action is a skip (page already exists,
	// hash unchanged), a child create action must still be able to
	// resolve the parent's page id. The executor pre-populates the
	// parent map from the plan, not just from successful creates.
	fc := &execFakeClient{}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{Kind: ActionSkip, RelPath: "docs/docs.md", PageID: 555, Title: "docs"},
		{Kind: ActionCreate, RelPath: "docs/new.md",
			ParentRelPath: "docs/docs.md",
			Title:         "new", IDLabel: "id-new", NewHashLabel: "hash-new"},
	}}
	out := x.Apply(context.Background(), plan)
	if out.Skipped != 1 || out.Created != 1 || len(out.Errors) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if len(fc.createCalls) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(fc.createCalls))
	}
	if fc.createCalls[0].parentID == nil || *fc.createCalls[0].parentID != 555 {
		t.Fatalf("child parent = %v, want pointer to 555 (skipped parent's id)", fc.createCalls[0].parentID)
	}
}

func TestExecutor_CreateUploadsImagesToNewPage(t *testing.T) {
	fc := &execFakeClient{}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{
			Kind:         ActionCreate,
			RelPath:      "a.md",
			Title:        "a",
			IDLabel:      "id",
			NewHashLabel: "hash",
			Storage:      `<ac:image><ri:attachment ri:filename="d.png" /></ac:image>`,
			Images:       []ImageRef{{Filename: "d.png", Data: []byte("PNG")}},
		},
	}}
	out := x.Apply(context.Background(), plan)
	if out.Created != 1 || out.ImagesUploaded != 1 || len(out.Errors) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if len(fc.uploads) != 1 {
		t.Fatalf("upload calls = %d, want 1", len(fc.uploads))
	}
	u := fc.uploads[0]
	// The image must be uploaded to the freshly-created page id (1001), not root.
	if u.pageID != 1001 || u.filename != "d.png" || string(u.data) != "PNG" {
		t.Errorf("upload = %+v, want pageID=1001 d.png/PNG", u)
	}
	if u.mime != "image/png" {
		t.Errorf("mime = %q, want image/png", u.mime)
	}
}

func TestExecutor_ImageUploadFailureDoesNotAbortPage(t *testing.T) {
	fc := &execFakeClient{uploadErr: errors.New("attach boom")}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{
			Kind: ActionUpdate, RelPath: "a.md", Title: "a", PageID: 42, Version: 1,
			IDLabel: "id", OldHashLabel: "old", NewHashLabel: "new",
			Storage: "<p>x</p>",
			Images:  []ImageRef{{Filename: "d.png", Data: []byte("PNG")}},
		},
	}}
	out := x.Apply(context.Background(), plan)
	// Page update still counts as success; the image error is surfaced but
	// does not roll back the page.
	if out.Updated != 1 {
		t.Errorf("Updated = %d, want 1", out.Updated)
	}
	if out.ImagesUploaded != 0 {
		t.Errorf("ImagesUploaded = %d, want 0", out.ImagesUploaded)
	}
	if len(out.Errors) != 1 || !strings.Contains(out.Errors[0].Err.Error(), "upload image") {
		t.Errorf("expected one upload-image error, got %+v", out.Errors)
	}
}

func TestExecutor_SkipAndOrphanMakeNoAPICalls(t *testing.T) {
	fc := &execFakeClient{}
	x := newExec(t, fc)

	plan := &Plan{Actions: []Action{
		{Kind: ActionSkip, RelPath: "a.md", PageID: 1},
		{Kind: ActionOrphan, PageID: 99, Title: "ghost"},
	}}
	out := x.Apply(context.Background(), plan)

	if out.Skipped != 1 || out.Orphaned != 1 || len(out.Errors) != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if len(fc.createCalls)+len(fc.updateCalls)+len(fc.addLabel)+len(fc.removeLabel) != 0 {
		t.Errorf("skip/orphan must be no-op; saw create=%d update=%d add=%d remove=%d",
			len(fc.createCalls), len(fc.updateCalls), len(fc.addLabel), len(fc.removeLabel))
	}
}

func TestExecutor_CreateErrorCountedOnceNotAsCreated(t *testing.T) {
	fc := &execFakeClient{createErr: errors.New("permission denied")}
	x := newExec(t, fc)
	plan := &Plan{Actions: []Action{
		{Kind: ActionCreate, RelPath: "a.md", Title: "a", IDLabel: "id"},
	}}
	out := x.Apply(context.Background(), plan)
	if out.Created != 0 {
		t.Errorf("Created = %d, want 0", out.Created)
	}
	if len(out.Errors) != 1 {
		t.Fatalf("Errors = %d, want 1", len(out.Errors))
	}
}

func TestNewExecutor_RequiredFields(t *testing.T) {
	good := ExecutorOptions{Client: &execFakeClient{}, SpaceKey: "X", RootPageID: 1}

	missing := []struct {
		name string
		opts ExecutorOptions
	}{
		{"client", ExecutorOptions{SpaceKey: good.SpaceKey, RootPageID: good.RootPageID}},
		{"space", ExecutorOptions{Client: good.Client, RootPageID: good.RootPageID}},
		{"root", ExecutorOptions{Client: good.Client, SpaceKey: good.SpaceKey}},
	}
	for _, tc := range missing {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewExecutor(tc.opts); err == nil {
				t.Fatalf("expected error when %s is missing", tc.name)
			}
		})
	}
}
