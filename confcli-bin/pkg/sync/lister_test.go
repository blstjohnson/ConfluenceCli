package sync

import (
	"context"
	"errors"
	"testing"

	"confcli/pkg/api"
	"confcli/pkg/models"
	"confcli/pkg/sync/identity"
)

// listerFakeClient is a minimal api.Client for testing APILister. Its
// GetDescendants returns a fixed slice; GetLabels returns the labels
// recorded in labelsByID.
type listerFakeClient struct {
	api.Client

	descendants    []models.Page
	descendantsErr error

	labelsByID    map[int][]models.Label
	getLabelsErr  error
	getLabelsHits int
}

func (f *listerFakeClient) GetDescendants(_ context.Context, _ int, _ int) ([]models.Page, error) {
	return f.descendants, f.descendantsErr
}

func (f *listerFakeClient) GetLabels(_ context.Context, pageID int) ([]models.Label, error) {
	f.getLabelsHits++
	if f.getLabelsErr != nil {
		return nil, f.getLabelsErr
	}
	return f.labelsByID[pageID], nil
}

func TestAPILister_ListManaged_FiltersAndExtractsIDLabel(t *testing.T) {
	idA := identity.BuildIDLabel("a.md")
	idB := identity.BuildIDLabel("b.md")

	fc := &listerFakeClient{
		descendants: []models.Page{
			{ID: models.PageID{Value: 1}, Title: "a"},
			{ID: models.PageID{Value: 2}, Title: "b"},
			{ID: models.PageID{Value: 3}, Title: "unmanaged"},
		},
		labelsByID: map[int][]models.Label{
			1: {{Name: idA}, {Name: "team-docs"}},
			2: {{Name: idB}},
			3: {{Name: "team-docs"}}, // no confcli-id → not managed
		},
	}

	l := NewAPILister(fc, nil)
	got, err := l.ListManaged(context.Background(), "SP", 99)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2; got %+v", len(got), got)
	}
	byID := map[int]ManagedPage{}
	for _, m := range got {
		byID[m.PageID] = m
	}
	if byID[1].IDLabel != idA {
		t.Errorf("page 1 id label = %q, want %q", byID[1].IDLabel, idA)
	}
	if byID[2].IDLabel != idB {
		t.Errorf("page 2 id label = %q, want %q", byID[2].IDLabel, idB)
	}
}

func TestAPILister_ListManaged_SkipsPagesWithLabelFetchError(t *testing.T) {
	fc := &listerFakeClient{
		descendants:  []models.Page{{ID: models.PageID{Value: 1}, Title: "a"}},
		getLabelsErr: errors.New("network blip"),
	}
	l := NewAPILister(fc, nil)
	got, err := l.ListManaged(context.Background(), "SP", 99)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected zero managed pages on fetch error, got %+v", got)
	}
}

func TestAPILister_ListManaged_UsesEmbeddedLabelsWhenPresent(t *testing.T) {
	idA := identity.BuildIDLabel("a.md")
	fc := &listerFakeClient{
		descendants: []models.Page{{
			ID:     models.PageID{Value: 1},
			Title:  "a",
			Labels: []models.Label{{Name: idA}},
		}},
	}
	l := NewAPILister(fc, nil)
	got, err := l.ListManaged(context.Background(), "SP", 99)
	if err != nil {
		t.Fatalf("ListManaged: %v", err)
	}
	if len(got) != 1 || got[0].IDLabel != idA {
		t.Fatalf("got = %+v", got)
	}
	if fc.getLabelsHits != 0 {
		t.Errorf("must not re-fetch labels when already present; getLabelsHits = %d", fc.getLabelsHits)
	}
}

func TestAPILister_ListManaged_DescendantsErrorBubbles(t *testing.T) {
	fc := &listerFakeClient{descendantsErr: errors.New("api down")}
	l := NewAPILister(fc, nil)
	_, err := l.ListManaged(context.Background(), "SP", 99)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAPILister_ListManaged_RequiresRoot(t *testing.T) {
	l := NewAPILister(&listerFakeClient{}, nil)
	if _, err := l.ListManaged(context.Background(), "SP", 0); err == nil {
		t.Fatal("expected error when rootPageID == 0")
	}
}
