package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"confcli/pkg/api"
	"confcli/pkg/converters"
	"confcli/pkg/models"
	"confcli/pkg/transforms"
)

type roundTripExportClient struct {
	api.Client
	pages        map[int]*models.Page
	contentCalls []string
}

func (c *roundTripExportClient) GetPage(_ context.Context, id int) (*models.Page, error) {
	if p := c.pages[id]; p != nil {
		return p, nil
	}
	return nil, fmt.Errorf("not found: %d", id)
}
func (c *roundTripExportClient) GetDescendants(context.Context, int, int) ([]models.Page, error) {
	return []models.Page{*c.pages[42]}, nil
}
func (c *roundTripExportClient) GetPageVersions(context.Context, int) ([]models.Version, error) {
	return []models.Version{{Number: 7, UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}, {Number: 8, UpdatedAt: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)}}, nil
}
func (c *roundTripExportClient) GetPageByTitle(_ context.Context, space, title string) (*models.Page, error) {
	return nil, fmt.Errorf("unexpected API title lookup: %s/%s", space, title)
}
func (c *roundTripExportClient) GetPageContent(_ context.Context, id interface{}, format string, version int) (string, error) {
	c.contentCalls = append(c.contentCalls, fmt.Sprintf("%v/%s/%d", id, format, version))
	if format == "export_view" {
		return `<p>Rendered body</p>`, nil
	}
	if fmt.Sprint(id) == "42" {
		return `<p>Other content</p>`, nil
	}
	return `<p>Before</p><ac:structured-macro ac:name="expand"><ac:parameter ac:name="title">Details</ac:parameter><ac:rich-text-body><ac:structured-macro ac:name="status"><ac:parameter ac:name="title">Must disappear</ac:parameter></ac:structured-macro><ac:structured-macro ac:name="include"><ac:parameter ac:name=""><ac:link><ri:page ri:content-title="Other"/></ac:link></ac:parameter></ac:structured-macro><ac:link ac:anchor="api"><ri:page ri:content-title="Other"/><ac:plain-text-link-body><![CDATA[Link]]></ac:plain-text-link-body></ac:link></ac:rich-text-body></ac:structured-macro><p>After</p>`, nil
}

func TestHierarchyPreservesMacrosWithHistoricalExportView(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("url", "https://wiki.test")
	client := &roundTripExportClient{pages: map[int]*models.Page{
		1:  {ID: models.PageID{Value: 1}, Title: "Source", Space: models.Space{Key: "DEV"}},
		42: {ID: models.PageID{Value: 42}, Title: "Other", Space: models.Space{Key: "DEV"}, Ancestors: []models.Page{{ID: models.PageID{Value: 1}}}},
	}}
	profile, err := transforms.LoadProfile([]byte("page:\n  preserve_macros: true\n  transforms:\n    - type: remove_macro\n      params:\n        macro_names: [status]\n        preserve_content: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	err = exportSpaceToDirectoryIterative(client, "DEV", 1, dir, "export", 0, false, 25, true, false, false, true, true, false, true, false, &converters.LinkRewriteConfig{ConfBaseURL: "https://wiki.test"}, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "Source", "Source.md"))
	if err != nil {
		t.Fatal(err)
	}
	markdown := string(data)
	for _, want := range []string{"Before", "<!-- expand-start: Details -->", "![Other](Other.md)", "[Link](Other.md#api)", "<!-- expand-end -->", "After"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("missing %q in exported file:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "Must disappear") {
		t.Errorf("storage pre-transform was not applied: %s", markdown)
	}
	for _, call := range []string{"1/export_view/7", "1/storage/7", "42/export_view/7", "42/storage/7"} {
		if !strings.Contains(strings.Join(client.contentCalls, ","), call) {
			t.Errorf("missing historical content request %s: %v", call, client.contentCalls)
		}
	}
}
