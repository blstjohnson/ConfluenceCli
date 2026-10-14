package clients

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"confcli/pkg/api"
)

func TestRootSnapshotPaginatesFiltersAndCachesLabels(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/api/content/7" {
			fmt.Fprint(w, `{"id":"7","title":"Root","space":{"key":"PERSONAL"}}`)
			return
		}
		if r.URL.Query().Get("start") == "0" {
			fmt.Fprint(w, `{"results":[{"id":"42","title":"Owned","space":{"key":"PERSONAL"},"ancestors":[{"id":"7"}],"version":{"number":2},"metadata":{"labels":{"results":[{"name":"confcli-id-a"}],"size":1}}}],"_links":{"next":"/rest/api/content?start=1"}}`)
		} else {
			fmt.Fprint(w, `{"results":[{"id":"99","title":"Outside","space":{"key":"PERSONAL"},"ancestors":[{"id":"8"}],"metadata":{"labels":{"results":[],"size":0}}},{"id":"43","title":"Unmanaged","space":{"key":"PERSONAL"},"ancestors":[{"id":"7"}],"metadata":{"labels":{"results":[],"size":0}}}],"_links":{}}`)
		}
	}))
	defer server.Close()
	wrapped, err := NewClient(&api.ClientOptions{BaseURL: server.URL, AuthType: "bearer", Token: "test", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	c := wrapped.(*client)
	for i := 0; i < 2; i++ {
		pages, err := c.GetRootSnapshot(context.Background(), "PERSONAL", 7)
		if err != nil || len(pages) != 2 || pages[0].Version.Number != 2 || len(pages[0].Labels) != 1 || len(pages[1].Labels) != 0 {
			t.Fatalf("incorrect snapshot: %+v, %v", pages, err)
		}
	}
	if requests != 3 {
		t.Fatalf("snapshot was not paged/cached: %d requests", requests)
	}
}

func TestRootSnapshotRejectsOtherSpace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"id":"7","space":{"key":"OTHER"}}`) }))
	defer server.Close()
	wrapped, err := NewClient(&api.ClientOptions{BaseURL: server.URL, AuthType: "bearer", Token: "test", ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wrapped.(*client).GetRootSnapshot(context.Background(), "PERSONAL", 7); err == nil {
		t.Fatal("wrong space accepted")
	}
}
