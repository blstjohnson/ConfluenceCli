package transforms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadImportProfile(t *testing.T) {
	yaml := `
kind: import
tree:
  folder_page: README.md
  skip:
    - "**/node_modules/**"
    - "_drafts/**"
  flatten:
    - "appendix/**"
transforms:
  - type: add_toc
  - type: forward_link_rewrite
    params:
      base_url: "https://example.atlassian.net/wiki"
overrides:
  - path: "docs/onboarding.md"
    page_id: 12345
  - path: "team/**"
    skip: true
`
	p, err := LoadImportProfile([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadImportProfile: %v", err)
	}

	if p.Kind != "import" {
		t.Errorf("Kind = %q, want %q", p.Kind, "import")
	}
	if p.Tree.FolderPage != "README.md" {
		t.Errorf("Tree.FolderPage = %q, want %q", p.Tree.FolderPage, "README.md")
	}
	if len(p.Tree.Skip) != 2 {
		t.Errorf("len(Tree.Skip) = %d, want 2", len(p.Tree.Skip))
	}
	if len(p.Tree.Flatten) != 1 {
		t.Errorf("len(Tree.Flatten) = %d, want 1", len(p.Tree.Flatten))
	}
	if len(p.Transforms) != 2 {
		t.Fatalf("len(Transforms) = %d, want 2", len(p.Transforms))
	}
	if p.Transforms[0].Type != "add_toc" {
		t.Errorf("Transforms[0].Type = %q, want %q", p.Transforms[0].Type, "add_toc")
	}
	if len(p.Overrides) != 2 {
		t.Fatalf("len(Overrides) = %d, want 2", len(p.Overrides))
	}
	if p.Overrides[0].PageID != 12345 {
		t.Errorf("Overrides[0].PageID = %d, want 12345", p.Overrides[0].PageID)
	}
	if !p.Overrides[1].Skip {
		t.Error("Overrides[1].Skip should be true")
	}
}

func TestLoadImportProfileRejectsWrongKind(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"missing", `tree: {}`},
		{"export", "kind: export\nfolder: {}\n"},
		{"empty", `kind: ""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadImportProfile([]byte(tc.yaml))
			if err == nil {
				t.Fatal("expected error for non-import kind, got nil")
			}
			if !strings.Contains(err.Error(), "kind: import") {
				t.Errorf("error %q should mention 'kind: import'", err.Error())
			}
		})
	}
}

func TestLoadImportProfileInvalidGlobs(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "bad skip glob",
			yaml: "kind: import\ntree:\n  skip: [\"[unterminated\"]\n",
			want: "tree.skip",
		},
		{
			name: "bad flatten glob",
			yaml: "kind: import\ntree:\n  flatten: [\"[unterminated\"]\n",
			want: "tree.flatten",
		},
		{
			name: "bad override path",
			yaml: "kind: import\noverrides:\n  - path: \"[unterminated\"\n",
			want: "overrides[0]",
		},
		{
			name: "override missing path",
			yaml: "kind: import\noverrides:\n  - page_id: 1\n",
			want: "overrides[0]: path is required",
		},
		{
			name: "override skip + page_id",
			yaml: "kind: import\noverrides:\n  - path: \"x\"\n    page_id: 1\n    skip: true\n",
			want: "mutually exclusive",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadImportProfile([]byte(tc.yaml))
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q should contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestImportProfileMatchers(t *testing.T) {
	p := &ImportProfile{
		Tree: TreeConfig{
			Skip:    []string{"**/node_modules/**", "_drafts/**"},
			Flatten: []string{"appendix/**", "**/_promote/*"},
		},
		Overrides: []PathOverride{
			{Path: "docs/onboarding.md", PageID: 100},
			{Path: "team/**", Skip: true},
			{Path: "**/*.draft.md", Skip: true},
		},
	}

	skipCases := map[string]bool{
		"src/node_modules/foo/bar.md": true,
		"node_modules/x.md":           true, // ** matches zero segments in doublestar
		"_drafts/intro.md":            true,
		"docs/intro.md":               false,
		"src/main.go":                 false,
	}
	for path, want := range skipCases {
		if got := p.MatchesSkip(path); got != want {
			t.Errorf("MatchesSkip(%q) = %v, want %v", path, got, want)
		}
	}

	flattenCases := map[string]bool{
		"appendix/glossary.md":   true,
		"docs/appendix/refs.md":  false,
		"docs/_promote/intro.md": true,
		"docs/intro.md":          false,
	}
	for path, want := range flattenCases {
		if got := p.MatchesFlatten(path); got != want {
			t.Errorf("MatchesFlatten(%q) = %v, want %v", path, got, want)
		}
	}

	overrideCases := []struct {
		path     string
		wantID   int
		wantSkip bool
		wantNil  bool
	}{
		{path: "docs/onboarding.md", wantID: 100},
		{path: "team/alpha/lead.md", wantSkip: true},
		{path: "notes/quick.draft.md", wantSkip: true},
		{path: "unmatched/path.md", wantNil: true},
	}
	for _, tc := range overrideCases {
		got := p.FindOverride(tc.path)
		if tc.wantNil {
			if got != nil {
				t.Errorf("FindOverride(%q) = %+v, want nil", tc.path, got)
			}
			continue
		}
		if got == nil {
			t.Fatalf("FindOverride(%q) = nil, want match", tc.path)
		}
		if got.PageID != tc.wantID {
			t.Errorf("FindOverride(%q).PageID = %d, want %d", tc.path, got.PageID, tc.wantID)
		}
		if got.Skip != tc.wantSkip {
			t.Errorf("FindOverride(%q).Skip = %v, want %v", tc.path, got.Skip, tc.wantSkip)
		}
	}
}

func TestFindOverrideFirstMatchWins(t *testing.T) {
	p := &ImportProfile{
		Overrides: []PathOverride{
			{Path: "docs/onboarding.md", PageID: 100},
			{Path: "docs/**", Skip: true},
		},
	}
	got := p.FindOverride("docs/onboarding.md")
	if got == nil || got.PageID != 100 {
		t.Errorf("FindOverride = %+v, want first match with PageID=100", got)
	}
}

func TestResolveImportProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upload.yaml")
	body := "kind: import\ntree:\n  folder_page: README.md\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p, err := ResolveImportProfile(path)
	if err != nil {
		t.Fatalf("ResolveImportProfile(%q): %v", path, err)
	}
	if p.Tree.FolderPage != "README.md" {
		t.Errorf("Tree.FolderPage = %q, want %q", p.Tree.FolderPage, "README.md")
	}

	if _, err := ResolveImportProfile("nonexistent-profile-xyz-987"); err == nil {
		t.Error("expected error for missing profile, got nil")
	}
}

func TestLoadProfileRejectsImportKind(t *testing.T) {
	_, err := LoadProfile([]byte("kind: import\n"))
	if err == nil {
		t.Fatal("expected LoadProfile to reject kind: import")
	}
	if !strings.Contains(err.Error(), "import") {
		t.Errorf("error %q should mention 'import'", err.Error())
	}
}

func TestLoadProfileAcceptsExplicitExportKind(t *testing.T) {
	_, err := LoadProfile([]byte("kind: export\nfolder:\n  naming: slug\n"))
	if err != nil {
		t.Errorf("LoadProfile with kind: export should succeed, got: %v", err)
	}
}

func TestImportProfileTitleFor_Default(t *testing.T) {
	p := &ImportProfile{Kind: "import"}
	cases := map[string]string{
		"intro.md":              "intro",
		"docs/getting_started.md": "getting_started",
		"docs/api/auth.md":      "auth",
		"no-extension":          "no-extension",
	}
	for in, want := range cases {
		if got := p.TitleFor(in); got != want {
			t.Errorf("TitleFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImportProfileTitleFor_UnderscoreToSpace(t *testing.T) {
	p := &ImportProfile{Kind: "import"}
	p.Tree.Title.Rewrites = []TitleRewrite{
		{Pattern: "_", Replacement: " "},
	}
	cases := map[string]string{
		"getting_started.md":    "getting started",
		"docs/api_v2_notes.md":  "api v2 notes",
		"plain.md":              "plain",
	}
	for in, want := range cases {
		if got := p.TitleFor(in); got != want {
			t.Errorf("TitleFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImportProfileTitleFor_OrderedRewritesAndTrim(t *testing.T) {
	// Rewrites apply in order: strip leading "01-" prefix, then _→ space,
	// then collapse multiple spaces. Trim cleans up the edges.
	p := &ImportProfile{Kind: "import"}
	p.Tree.Title.Rewrites = []TitleRewrite{
		{Pattern: `^\d+[-_. ]+`, Replacement: ""},
		{Pattern: "_", Replacement: " "},
		{Pattern: `\s+`, Replacement: " "},
	}
	p.Tree.Title.Trim = true
	cases := map[string]string{
		"01-getting_started.md":   "getting started",
		"03___onboarding.md":       "onboarding",
		"plain_title.md":          "plain title",
	}
	for in, want := range cases {
		if got := p.TitleFor(in); got != want {
			t.Errorf("TitleFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestImportProfileTitleFor_RegexCaptureGroups(t *testing.T) {
	// Capture-group references in the replacement work as ReplaceAllString.
	p := &ImportProfile{Kind: "import"}
	p.Tree.Title.Rewrites = []TitleRewrite{
		{Pattern: `^(\d+)-(.+)$`, Replacement: "$2 (chapter $1)"},
	}
	if got := p.TitleFor("07-intro.md"); got != "intro (chapter 07)" {
		t.Errorf("TitleFor = %q, want %q", got, "intro (chapter 07)")
	}
}

func TestImportProfileTitleFor_RewritesAppliedViaLoad(t *testing.T) {
	yaml := `
kind: import
tree:
  title:
    rewrites:
      - pattern: "_"
        replacement: " "
    trim: true
`
	p, err := LoadImportProfile([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadImportProfile: %v", err)
	}
	if got := p.TitleFor("getting_started.md"); got != "getting started" {
		t.Errorf("TitleFor = %q, want %q", got, "getting started")
	}
}

func TestLoadImportProfileRejectsBadTitleRegex(t *testing.T) {
	yaml := `
kind: import
tree:
  title:
    rewrites:
      - pattern: "[unterminated"
        replacement: ""
`
	_, err := LoadImportProfile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
	if !strings.Contains(err.Error(), "tree.title.rewrites[0]") {
		t.Errorf("error %q should mention tree.title.rewrites[0]", err.Error())
	}
}

func TestLoadImportProfileRejectsEmptyTitlePattern(t *testing.T) {
	yaml := `
kind: import
tree:
  title:
    rewrites:
      - pattern: ""
        replacement: " "
`
	_, err := LoadImportProfile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty pattern, got nil")
	}
	if !strings.Contains(err.Error(), "pattern is required") {
		t.Errorf("error %q should mention pattern is required", err.Error())
	}
}

func TestLoadImportProfileRejectsBadGitFilesMode(t *testing.T) {
	yaml := `
kind: import
git_files:
  mode: maybe
`
	_, err := LoadImportProfile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for bad mode, got nil")
	}
	if !strings.Contains(err.Error(), "git_files.mode") {
		t.Errorf("error %q should mention git_files.mode", err.Error())
	}
}

func TestLoadImportProfileRejectsBadPerExtension(t *testing.T) {
	yaml := `
kind: import
git_files:
  per_extension:
    sql: nope
`
	_, err := LoadImportProfile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for bad per_extension value, got nil")
	}
	if !strings.Contains(err.Error(), "per_extension") {
		t.Errorf("error %q should mention per_extension", err.Error())
	}
}

func TestLoadImportProfileAcceptsInlineMode(t *testing.T) {
	yaml := `
kind: import
git_files:
  mode: inline
  per_extension:
    sql: link
    yaml: inline
  inline:
    max_bytes: 4096
`
	p, err := LoadImportProfile([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadImportProfile: %v", err)
	}
	if p.GitFiles.Mode != "inline" {
		t.Errorf("Mode = %q, want inline", p.GitFiles.Mode)
	}
	if p.GitFiles.Inline.MaxBytes != 4096 {
		t.Errorf("Inline.MaxBytes = %d, want 4096", p.GitFiles.Inline.MaxBytes)
	}
	if p.GitFiles.PerExtension["sql"] != "link" || p.GitFiles.PerExtension["yaml"] != "inline" {
		t.Errorf("PerExtension = %v, want sql=link yaml=inline", p.GitFiles.PerExtension)
	}
}

func TestPeekProfileKind(t *testing.T) {
	cases := []struct {
		yaml string
		want string
	}{
		{"kind: import\n", "import"},
		{"kind: export\nfolder: {}\n", "export"},
		{"folder: {}\n", ""},
	}
	for _, tc := range cases {
		got, err := peekProfileKind([]byte(tc.yaml))
		if err != nil {
			t.Errorf("peekProfileKind(%q): %v", tc.yaml, err)
			continue
		}
		if got != tc.want {
			t.Errorf("peekProfileKind(%q) = %q, want %q", tc.yaml, got, tc.want)
		}
	}
}
