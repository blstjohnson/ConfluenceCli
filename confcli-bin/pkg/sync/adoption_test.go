package sync

import (
	"confcli/pkg/transforms"
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"confcli/pkg/models"
	"confcli/pkg/sync/identity"
)

type copyLocator struct {
	*fakeLocator
	pinned      *models.Page
	requestedID int
}

func (l *copyLocator) FindByID(_ context.Context, _ string, id int) (*models.Page, error) {
	l.requestedID = id
	return l.pinned, nil
}

func TestCopyAdoptionPreservesTitlesInPlanAndLinks(t *testing.T) {
	profile := basicProfile()
	profile.Overrides = []transforms.PathOverride{{Path: "ТЗ_Сервис.md", PageID: 42, Title: "ТЗ. Сервис"}}
	fsys := fstest.MapFS{"ТЗ_Сервис.md": {Data: []byte("body")}}
	loc := &copyLocator{fakeLocator: &fakeLocator{}, pinned: pageWithLabels(42, "ТЗ. Сервис", 1)}
	plan, err := newEngine(t, profile, loc, nil).BuildPlan(context.Background(), fsys, "SPACE", 1)
	if err != nil {
		t.Fatal(err)
	}
	if loc.requestedID != 42 || plan.Stats.Update != 1 || plan.Stats.Create != 0 || plan.Actions[0].Title != "ТЗ. Сервис" {
		t.Fatalf("copy not adopted: %+v", plan)
	}
	refs, err := BuildPathMap(profile, fsys)
	if err != nil || refs["ТЗ_Сервис.md"].Title != "ТЗ. Сервис" {
		t.Fatalf("incorrect link target: %+v, %v", refs, err)
	}
}

func TestPinnedLocatorRejectsWrongSpaceAndOutsideRoot(t *testing.T) {
	for _, test := range []struct {
		space  string
		parent int
		valid  bool
	}{
		{"PERSONAL", 7, true}, {"OTHER", 7, false}, {"PERSONAL", 8, false},
	} {
		fc := &fakeClient{expandPage: &models.Page{ID: models.PageID{Value: 42}, Space: models.Space{Key: test.space}, Ancestors: []models.Page{pageWithID(test.parent)}}}
		_, err := NewLocator(fc, nil, 7).FindByID(context.Background(), "PERSONAL", 42)
		if (err == nil) != test.valid {
			t.Fatalf("destination accepted incorrectly: %+v: %v", test, err)
		}
	}
}

func TestSelectedDocumentAttachmentPreservesBytesAndRejectsMissingFile(t *testing.T) {
	fsys := fstest.MapFS{"assets/file.xlsx": {Data: []byte("original spreadsheet bytes")}}
	storage := `<p><a href="../assets/file.xlsx">Download</a></p>`
	out, files, err := rewriteImages(storage, "pages/page.md", fsys, nil, "", nil, []string{"assets/**"})
	if err != nil || len(files) != 1 || string(files[0].Data) != "original spreadsheet bytes" || !strings.Contains(out, `ri:filename="file.xlsx"`) {
		t.Fatalf("attachment not uploaded: %s, %+v, %v", out, files, err)
	}
	_, _, err = rewriteImages(strings.ReplaceAll(storage, "file.xlsx", "missing.xlsx"), "pages/page.md", fsys, nil, "", nil, []string{"assets/**"})
	if err == nil {
		t.Fatal("missing selected attachment must fail during planning")
	}
}

func TestSelectedMarkdownAttachmentSurvivesPageAndGitRewriters(t *testing.T) {
	profile := basicProfile()
	profile.Attachments.Paths = []string{"archive/**"}
	gitFiles := &transforms.RewriteGitFileLinks{Mode: "url", URL: "https://tfs.example/repo", AttachmentPaths: profile.Attachments.Paths}
	convert, err := NewProfileMarkdownConverter(profile, map[string]transforms.PageRef{}, nil, gitFiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := convert(context.Background(), []byte("[old](../archive/old.md#section)"), "pages/page.md")
	if err != nil {
		t.Fatal(err)
	}
	out, files, err := rewriteImages(storage, "pages/page.md", fstest.MapFS{"archive/old.md": {Data: []byte("# Old document")}}, nil, "", nil, profile.Attachments.Paths)
	if err != nil || len(files) != 1 || !strings.Contains(out, `ri:filename="old.md"`) || strings.Contains(out, "tfs.example") {
		t.Fatalf("old document link lost: %s, %+v, %v", out, files, err)
	}
}

func TestLiteralCodePlaceholderStaysVisibleAndValidCodeRemainsFormatted(t *testing.T) {
	convert := NewMarkdownConverter(nil, nil, nil, nil)
	storage, err := convert(context.Background(), []byte("Ошибка <code> : <detail>. URL /<template identifier>. `real code`"), "page.md")
	if err != nil || !strings.Contains(storage, "&lt;code&gt;") || !strings.Contains(storage, "&lt;template identifier&gt;") || !strings.Contains(storage, "<code>real code</code>") {
		t.Fatalf("literal placeholder not preserved: %s, %v", storage, err)
	}
}

func TestUnchangedChildMovesUnderRecreatedParent(t *testing.T) {
	profile := basicProfile()
	profile.Tree.FolderPage = "{dir}.md"
	fsys := fstest.MapFS{"folder/folder.md": {Data: []byte("parent")}, "folder/child.md": {Data: []byte("child")}}
	child := pageWithLabels(42, "child", 1, identity.BuildHashLabel("child", "child", ""))
	child.Ancestors = []models.Page{pageWithID(99)}
	loc := &fakeLocator{pages: map[string]*models.Page{"folder/child.md": child}}
	plan, err := newEngine(t, profile, loc, nil).BuildPlan(context.Background(), fsys, "SPACE", 7)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Stats.Create != 1 || plan.Stats.Update != 1 || plan.Actions[1].ParentRelPath != "folder/folder.md" {
		t.Fatalf("unchanged child was not moved: %+v", plan)
	}
}

func TestParagraphStartingWithPageLinkRetainsParagraphAndFormatting(t *testing.T) {
	convert := NewMarkdownConverter(map[string]transforms.PageRef{"target.md": {Title: "Target"}}, nil, nil, nil)
	storage, err := convert(context.Background(), []byte("[Target](target.md) **description**.\n\n[Target](target.md) another paragraph."), "page.md")
	if err != nil || strings.Count(storage, "<p>") != 2 || !strings.Contains(storage, "<strong>description</strong>") || strings.Contains(storage, "data-confcli-xml") {
		t.Fatalf("link ate paragraph formatting: %s, %v", storage, err)
	}
}
