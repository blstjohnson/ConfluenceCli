package converters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPageMapFromDirs_FilenamePrefix(t *testing.T) {
	dir := t.TempDir()

	// Default export naming: "<id>_<title>.<ext>".
	content := filepath.Join(dir, "744208425_my-page.md")
	if err := os.WriteFile(content, []byte("# hi"), 0644); err != nil {
		t.Fatal(err)
	}
	// A nested page in a subfolder.
	sub := filepath.Join(dir, "child")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(sub, "100_child.md")
	if err := os.WriteFile(child, []byte("# child"), 0644); err != nil {
		t.Fatal(err)
	}
	// A non-content file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := BuildPageMapFromDirs([]string{dir})
	if err != nil {
		t.Fatalf("BuildPageMapFromDirs: %v", err)
	}

	wantRoot, _ := filepath.Abs(content)
	if got := m[744208425]; got != wantRoot {
		t.Errorf("map[744208425] = %q, want %q", got, wantRoot)
	}
	wantChild, _ := filepath.Abs(child)
	if got := m[100]; got != wantChild {
		t.Errorf("map[100] = %q, want %q", got, wantChild)
	}
	if len(m) != 2 {
		t.Errorf("len(map) = %d, want 2", len(m))
	}
}

func TestBuildPageMapFromDirs_MetaSidecarCleanNames(t *testing.T) {
	dir := t.TempDir()

	// --clean-names export: content file has no numeric prefix, but a meta.json
	// sidecar carries the authoritative id.
	content := filepath.Join(dir, "My Page.md")
	if err := os.WriteFile(content, []byte("# hi"), 0644); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(dir, "My Page.meta.json")
	if err := os.WriteFile(meta, []byte(`{"id": 555, "title": "My Page"}`), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := BuildPageMapFromDirs([]string{dir})
	if err != nil {
		t.Fatalf("BuildPageMapFromDirs: %v", err)
	}

	want, _ := filepath.Abs(content)
	if got := m[555]; got != want {
		t.Errorf("map[555] = %q, want %q", got, want)
	}
}

func TestBuildPageMapFromDirs_MetaWinsOverPrefix(t *testing.T) {
	dir := t.TempDir()

	// Content file name says 111, meta sidecar says 222 (authoritative).
	content := filepath.Join(dir, "111_renamed.md")
	if err := os.WriteFile(content, []byte("# hi"), 0644); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(dir, "111_renamed.meta.json")
	if err := os.WriteFile(meta, []byte(`{"id": 222}`), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := BuildPageMapFromDirs([]string{dir})
	if err != nil {
		t.Fatalf("BuildPageMapFromDirs: %v", err)
	}

	want, _ := filepath.Abs(content)
	if got := m[222]; got != want {
		t.Errorf("map[222] = %q, want %q", got, want)
	}
}

func TestBuildPageMapFromDirs_EmptyAndMissing(t *testing.T) {
	m, err := BuildPageMapFromDirs([]string{"", "   "})
	if err != nil {
		t.Fatalf("BuildPageMapFromDirs(empty): %v", err)
	}
	if len(m) != 0 {
		t.Errorf("len(map) = %d, want 0", len(m))
	}
}
