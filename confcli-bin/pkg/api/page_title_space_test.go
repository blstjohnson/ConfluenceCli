package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPageTitleLookupUsesConfluenceSpaceKey(t *testing.T) {
	// Confluence ignores an unknown "space" parameter. With an archive copy
	// having the same title, that silently resolves links into another space.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, space := 99, "ARCHIVE"
		if r.URL.Query().Get("spaceKey") == "DEV" {
			id, space = 42, "DEV"
		}
		if r.URL.Query().Get("title") != "Shared topics" || r.URL.Query().Has("space") {
			t.Errorf("incorrect title lookup: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{
			"id": id, "title": "Shared topics", "space": map[string]string{"key": space},
		}}})
	}))
	defer server.Close()
	client, err := NewHTTPClient(&ClientOptions{BaseURL: server.URL, AuthType: "bearer", Token: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("extension", func(t *testing.T) {
		result, err := NewPageExtension(client).FindPageByTitle(context.Background(), &FindPageByTitleRequest{SpaceKey: "DEV", Title: "Shared topics"})
		if err != nil {
			t.Fatal(err)
		}
		if id, _ := result.Page.ID.Int(); id != 42 {
			t.Fatalf("resolved archive page instead of requested space: %+v", result.Page)
		}
	})
	t.Run("raw", func(t *testing.T) {
		response, err := client.GetPageByTitleRaw(context.Background(), "DEV", "Shared topics")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result struct {
			Results []struct {
				ID int `json:"id"`
			} `json:"results"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if len(result.Results) != 1 || result.Results[0].ID != 42 {
			t.Fatalf("resolved archive page: %+v", result)
		}
	})
}
