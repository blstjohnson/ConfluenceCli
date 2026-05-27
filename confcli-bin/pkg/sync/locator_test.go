package sync

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"confcli/pkg/api"
	"confcli/pkg/models"
	"confcli/pkg/sync/identity"
)

// fakeClient implements just the api.Client methods used by Locator. It
// embeds api.Client so the file still satisfies the interface; unused methods
// will nil-panic if accidentally called — desired for catching regressions.
type fakeClient struct {
	api.Client

	searchCQL    string
	searchResult []models.SearchResult
	searchErr    error

	expandID         interface{}
	expandExpansions []string
	expandPage       *models.Page
	expandErr        error

	getLabelsByID map[int][]models.Label
	getLabelsErr  error
}

func (f *fakeClient) Search(_ context.Context, cql string, _ int) ([]models.SearchResult, error) {
	f.searchCQL = cql
	return f.searchResult, f.searchErr
}

func (f *fakeClient) GetPageWithExpansions(_ context.Context, id interface{}, expansions []string) (*models.Page, error) {
	f.expandID = id
	f.expandExpansions = expansions
	return f.expandPage, f.expandErr
}

func (f *fakeClient) GetLabels(_ context.Context, pageID int) ([]models.Label, error) {
	if f.getLabelsErr != nil {
		return nil, f.getLabelsErr
	}
	return f.getLabelsByID[pageID], nil
}

func pageWithID(id int) models.Page {
	return models.Page{ID: models.PageID{Value: id}}
}

func TestLocator_FindByPath_NotFound(t *testing.T) {
	fc := &fakeClient{searchResult: nil}
	l := NewLocator(fc, nil)

	got, err := l.FindByPath(context.Background(), "SPACE", "docs/a.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil page, got %+v", got)
	}
	if !strings.Contains(fc.searchCQL, identity.BuildIDLabel("docs/a.md")) {
		t.Fatalf("CQL missing id-label: %q", fc.searchCQL)
	}
}

func TestLocator_FindByPath_OneMatch(t *testing.T) {
	idLabel := identity.BuildIDLabel("docs/a.md")
	fc := &fakeClient{
		searchResult: []models.SearchResult{{Content: pageWithID(42)}},
		expandPage: &models.Page{
			ID:    models.PageID{Value: 42},
			Title: "Hello",
		},
		// Labels come from the dedicated label endpoint, not the page
		// expansion — see locator.go comment for why.
		getLabelsByID: map[int][]models.Label{
			42: {{Name: idLabel}},
		},
	}
	l := NewLocator(fc, nil)

	got, err := l.FindByPath(context.Background(), "SPACE", "docs/a.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Title != "Hello" {
		t.Fatalf("expected page Hello, got %+v", got)
	}
	// version must be expanded so the engine can pass it to UpdatePage.
	versionAsked := false
	for _, e := range fc.expandExpansions {
		if e == "version" {
			versionAsked = true
		}
	}
	if !versionAsked {
		t.Fatalf("version expansion not requested; got %v", fc.expandExpansions)
	}
	// Labels must be populated via GetLabels.
	if identity.ExtractIDLabel(got.Labels) != idLabel {
		t.Fatalf("returned page missing id label; got labels %+v", got.Labels)
	}
}

func TestLocator_FindByPath_MultipleMatches_Warns(t *testing.T) {
	fc := &fakeClient{
		searchResult: []models.SearchResult{
			{Content: pageWithID(1)},
			{Content: pageWithID(2)},
		},
		expandPage: &models.Page{ID: models.PageID{Value: 1}},
	}
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	l := NewLocator(fc, logger)

	_, err := l.FindByPath(context.Background(), "SPACE", "docs/a.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "multiple pages") {
		t.Fatalf("expected warning about multiple pages, got: %q", buf.String())
	}
}

func TestLocator_FindByPath_SearchError(t *testing.T) {
	fc := &fakeClient{searchErr: errors.New("boom")}
	l := NewLocator(fc, nil)
	_, err := l.FindByPath(context.Background(), "SPACE", "docs/a.md")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error wrapping 'boom', got %v", err)
	}
}
