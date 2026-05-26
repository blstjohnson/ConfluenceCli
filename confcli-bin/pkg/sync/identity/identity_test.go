package identity

import (
	"strings"
	"testing"

	"confcli/pkg/models"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"forward slashes lowercased", "Docs/Sub/Page.md", "docs/sub/page.md"},
		{"backslashes become forward", `Docs\Sub\Page.md`, "docs/sub/page.md"},
		{"leading dot-slash stripped", "./docs/a.md", "docs/a.md"},
		{"already canonical", "docs/a.md", "docs/a.md"},
		{"no extension preserved", "README", "readme"},
		{"unicode NFC", "docs/café.md", "docs/café.md"}, // decomposed → composed
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePath(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizePath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildIDLabelStability(t *testing.T) {
	// Same logical path on Windows and Linux must collapse to the same id.
	winPath := `Docs\Sub\Page.md`
	nixPath := "docs/sub/page.md"
	if BuildIDLabel(winPath) != BuildIDLabel(nixPath) {
		t.Fatalf("path normalization broke cross-platform id stability")
	}
}

func TestBuildIDLabelDistinct(t *testing.T) {
	a := BuildIDLabel("docs/a.md")
	b := BuildIDLabel("docs/b.md")
	if a == b {
		t.Fatalf("different paths must produce different id labels")
	}
	if !strings.HasPrefix(a, IDLabelPrefix) {
		t.Fatalf("id label missing prefix: %q", a)
	}
}

func TestBuildHashLabel(t *testing.T) {
	h1 := BuildHashLabel("Title", "<p>body</p>")
	h2 := BuildHashLabel("Title", "<p>body</p>")
	if h1 != h2 {
		t.Fatalf("hash must be deterministic, got %q vs %q", h1, h2)
	}
	if !strings.HasPrefix(h1, HashLabelPrefix) {
		t.Fatalf("hash label missing prefix: %q", h1)
	}

	// Title change → different hash.
	if BuildHashLabel("Title", "<p>body</p>") == BuildHashLabel("Title2", "<p>body</p>") {
		t.Fatalf("title change must change hash")
	}
	// Payload change → different hash.
	if BuildHashLabel("Title", "<p>body</p>") == BuildHashLabel("Title", "<p>body2</p>") {
		t.Fatalf("payload change must change hash")
	}
}

func TestHashLabelBoundaryDisambiguation(t *testing.T) {
	// Without a NUL separator, ("foo","bar") and ("foob","ar") would collide.
	a := BuildHashLabel("foo", "bar")
	b := BuildHashLabel("foob", "ar")
	if a == b {
		t.Fatalf("title/payload boundary ambiguity: %q == %q", a, b)
	}
}

func TestExtractLabels(t *testing.T) {
	labels := []models.Label{
		{Name: "team-docs"},
		{Name: "confcli-id-abc123"},
		{Name: "confcli-hash-def456"},
		{Name: "other"},
	}
	if got := ExtractIDLabel(labels); got != "confcli-id-abc123" {
		t.Fatalf("ExtractIDLabel = %q", got)
	}
	if got := ExtractHashLabel(labels); got != "confcli-hash-def456" {
		t.Fatalf("ExtractHashLabel = %q", got)
	}
	if got := ExtractIDLabel(nil); got != "" {
		t.Fatalf("ExtractIDLabel(nil) = %q, want empty", got)
	}
	if got := ExtractHashLabel([]models.Label{{Name: "unrelated"}}); got != "" {
		t.Fatalf("ExtractHashLabel with no match = %q, want empty", got)
	}
}

func TestCQLFilter(t *testing.T) {
	got := CQLFilter("MYSPACE", "docs/a.md")
	wantSubs := []string{
		`label = "` + BuildIDLabel("docs/a.md") + `"`,
		`space = "MYSPACE"`,
		`type = "page"`,
	}
	for _, s := range wantSubs {
		if !strings.Contains(got, s) {
			t.Fatalf("CQLFilter missing %q in %q", s, got)
		}
	}
}
