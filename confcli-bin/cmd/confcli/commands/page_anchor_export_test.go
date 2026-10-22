package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestPageGetSelfAnchorsUseOutputFileWithoutReferenceDirectories(t *testing.T) {
	const storage = `<h2>Scenario</h2><h3>API 001 Message check</h3><h3>API 002 Decrypt</h3><p><ac:link ac:anchor="msgcheck"><ac:plain-text-link-body>Check</ac:plain-text-link-body></ac:link> <a href="#msgdecrypt">Decrypt</a></p><h2>Common processes</h2><h3><ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">msgcheck</ac:parameter></ac:structured-macro>API 001 Message check</h3><p>Check rules.</p><h3><ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">msgdecrypt</ac:parameter></ac:structured-macro>API 002 Decrypt</h3><p>Decrypt rules.</p>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rest/api/content/42" {
			t.Errorf("unexpected API call: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "42", "type": "page", "title": "Source name", "space": map[string]any{"key": "DEV"}, "version": map[string]any{"number": 1}, "body": map[string]any{"storage": map[string]string{"value": storage}, "export_view": map[string]string{"value": "<p>Rendered content must not replace storage anchors</p>"}}})
	}))
	defer server.Close()
	for _, format := range []string{"markdown", "export"} {
		for _, mode := range []string{"file", "directory", "stale-metadata"} {
			t.Run(format+"/"+mode, func(t *testing.T) {
				viper.Reset()
				t.Cleanup(viper.Reset)
				viper.Set("url", server.URL)
				viper.Set("auth_type", "bearer")
				viper.Set("token", "test-token")
				viper.Set("read_only", true)
				dir := t.TempDir()
				name := "Renamed page.md"
				args := []string{"--id", "42", "--format", format}
				if mode == "file" || mode == "stale-metadata" {
					args = append(args, "--output", filepath.Join(dir, name))
				} else {
					name = "42.md"
					args = append(args, "--output-dir", dir)
				}
				if mode == "stale-metadata" {
					refs := t.TempDir()
					if err := os.WriteFile(filepath.Join(refs, "previous.md"), []byte("Old file"), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(refs, "previous.meta.json"), []byte(`{"id":42}`), 0600); err != nil {
						t.Fatal(err)
					}
					encoded, _ := json.Marshal(refs)
					profile := filepath.Join(dir, "export.yaml")
					if err := os.WriteFile(profile, []byte("page:\n  refs_dirs: ["+string(encoded)+"]\n"), 0600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--transform", profile)
				}
				cmd := newPageGetCmd()
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				body := string(data)
				escaped := strings.ReplaceAll(name, " ", "%20")
				for _, want := range []string{"[Check](" + escaped + "#Common%20processes#API%20001%20Message%20check)", "[Decrypt](" + escaped + "#Common%20processes#API%20002%20Decrypt)"} {
					if !strings.Contains(body, want) {
						t.Errorf("missing %q in export:\n%s", want, body)
					}
				}
				if strings.Contains(body, "#msgcheck") || strings.Contains(body, "#msgdecrypt") || strings.Contains(body, "<a id=") {
					t.Fatal(body)
				}
			})
		}
	}
}
