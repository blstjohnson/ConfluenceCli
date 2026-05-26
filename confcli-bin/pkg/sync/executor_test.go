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

	createErr error
	updateErr error
	labelErr  error

	nextPageID int
}

type createCall struct {
	spaceKey string
	parentID *int
	title    string
	content  string
	format   string
}

type updateCall struct {
	id      int
	content string
	comment string
	format  string
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

func (f *execFakeClient) UpdatePage(_ context.Context, id int, content, comment, format string) (*models.Page, error) {
	f.updateCalls = append(f.updateCalls, updateCall{id, content, comment, format})
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &models.Page{ID: models.PageID{Value: id}}, nil
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
