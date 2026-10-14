package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFindPageByTitleDistinguishesMissingFromAuthFailure(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"results":[]}`))
			}))
			defer server.Close()
			client, err := NewHTTPClient(&ClientOptions{BaseURL: server.URL, AuthType: "bearer", Token: "test"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = NewPageExtension(client).FindPageByTitle(context.Background(), &FindPageByTitleRequest{SpaceKey: "DEV", Title: "Missing"})
			var missing *PageNotFoundError
			if err == nil || errors.As(err, &missing) != (status == http.StatusOK) {
				t.Fatalf("incorrect error classification for status %d: %v", status, err)
			}
		})
	}
}
