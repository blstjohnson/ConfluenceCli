package sync

import (
	"context"
	"testing"

	"confcli/pkg/models"
)

type pinnedSnapshotFake struct {
	fakeClient
	pages []models.Page
	calls int
}

func (f *pinnedSnapshotFake) GetRootSnapshot(context.Context, string, int) ([]models.Page, error) {
	f.calls++
	return f.pages, nil
}

func pinnedSnapshotPage(id int, space string, root int) models.Page {
	return models.Page{ID: models.PageID{Value: id}, Space: models.Space{Key: space}, Ancestors: []models.Page{{ID: models.PageID{Value: root}}}, Labels: []models.Label{{Name: "confcli-hash-current"}}, Version: models.Version{Number: 7}}
}

func TestPinnedSnapshotReusesVerifiedPagesAndLabels(t *testing.T) {
	f := &pinnedSnapshotFake{pages: []models.Page{pinnedSnapshotPage(2, "TEST", 1), pinnedSnapshotPage(3, "TEST", 1)}}
	l := NewLocator(f, nil, 1)
	for _, id := range []int{2, 3, 2} {
		page, err := l.FindByID(context.Background(), "TEST", id)
		if err != nil || page.Version.Number != 7 || len(page.Labels) != 1 {
			t.Fatalf("snapshot page/labels: %+v %v", page, err)
		}
	}
	if f.calls != 1 || f.expandID != nil {
		t.Fatalf("redundant reads: snapshot=%d direct=%v", f.calls, f.expandID)
	}
}

func TestPinnedSnapshotRejectsWrongSpaceAndAncestry(t *testing.T) {
	for _, page := range []models.Page{pinnedSnapshotPage(2, "OTHER", 1), pinnedSnapshotPage(2, "TEST", 99)} {
		f := &pinnedSnapshotFake{pages: []models.Page{page}}
		if _, err := NewLocator(f, nil, 1).FindByID(context.Background(), "TEST", 2); err == nil {
			t.Fatal("unsafe snapshot destination accepted")
		}
	}
}

func TestPinnedSnapshotMissingPageRetainsDirectDestinationGuard(t *testing.T) {
	f := &pinnedSnapshotFake{fakeClient: fakeClient{expandPage: func() *models.Page { p := pinnedSnapshotPage(9, "TEST", 99); return &p }()}}
	if _, err := NewLocator(f, nil, 1).FindByID(context.Background(), "TEST", 9); err == nil {
		t.Fatal("outside-root direct fallback accepted")
	}
	if f.expandID == nil {
		t.Fatal("missing page was not fetched directly")
	}
}
