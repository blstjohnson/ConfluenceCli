package transforms

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfile(t *testing.T) {
	yaml := `
folder:
  naming: slug
  length_limit: 50
  flat_leaves: true
page:
  format: markdown
  strip_toc: true
  save_metadata: false
  transforms:
    - type: remove_macro
      params:
        macro_names: ["toc", "info"]
    - type: modify_content
      params:
        phase: post
        rules:
          - find: "foo"
            replace: "bar"
pages:
  - id: 12345
    format: html
    skip_transforms: true
  - path: "docs/*.md"
    transforms:
      - type: remove_element
        params:
          selectors: ["div.sidebar"]
  - id: [100, 200, 300]
    skip: true
`
	p, err := LoadProfile([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	// Folder config
	if p.Folder.Naming != "slug" {
		t.Errorf("Folder.Naming = %q, want %q", p.Folder.Naming, "slug")
	}
	if p.Folder.LengthLimit != 50 {
		t.Errorf("Folder.LengthLimit = %d, want 50", p.Folder.LengthLimit)
	}
	if !p.Folder.FlatLeaves {
		t.Error("Folder.FlatLeaves = false, want true")
	}

	// Page defaults
	if p.Page.Format != "markdown" {
		t.Errorf("Page.Format = %q, want %q", p.Page.Format, "markdown")
	}
	if !p.Page.StripTOC {
		t.Error("Page.StripTOC = false, want true")
	}
	if len(p.Page.Transforms) != 2 {
		t.Fatalf("len(Page.Transforms) = %d, want 2", len(p.Page.Transforms))
	}
	if p.Page.Transforms[0].Type != "remove_macro" {
		t.Errorf("Transforms[0].Type = %q, want %q", p.Page.Transforms[0].Type, "remove_macro")
	}

	// Page overrides
	if len(p.Pages) != 3 {
		t.Fatalf("len(Pages) = %d, want 3", len(p.Pages))
	}
	if p.Pages[0].Format != "html" {
		t.Errorf("Pages[0].Format = %q, want %q", p.Pages[0].Format, "html")
	}
	if !p.Pages[0].SkipTransforms {
		t.Error("Pages[0].SkipTransforms = false, want true")
	}
	if !p.Pages[2].Skip {
		t.Error("Pages[2].Skip = false, want true")
	}
}

func TestPageOverrideMatchesPage(t *testing.T) {
	tests := []struct {
		name     string
		override PageOverride
		pageID   int
		pagePath string
		want     bool
	}{
		{
			name:     "match by int id",
			override: PageOverride{ID: 123},
			pageID:   123,
			want:     true,
		},
		{
			name:     "no match by int id",
			override: PageOverride{ID: 123},
			pageID:   456,
			want:     false,
		},
		{
			name:     "match by float64 id (yaml decode)",
			override: PageOverride{ID: float64(123)},
			pageID:   123,
			want:     true,
		},
		{
			name:     "match by id list",
			override: PageOverride{ID: []interface{}{float64(100), float64(200)}},
			pageID:   200,
			want:     true,
		},
		{
			name:     "no match by id list",
			override: PageOverride{ID: []interface{}{float64(100), float64(200)}},
			pageID:   300,
			want:     false,
		},
		{
			name:     "match by path glob",
			override: PageOverride{Path: "docs/*.md"},
			pagePath: "docs/intro.md",
			want:     true,
		},
		{
			name:     "no match by path glob",
			override: PageOverride{Path: "docs/*.md"},
			pagePath: "src/main.go",
			want:     false,
		},
		{
			name:     "match by both id and path",
			override: PageOverride{ID: float64(42), Path: "docs/*.md"},
			pageID:   42,
			pagePath: "docs/intro.md",
			want:     true,
		},
		{
			name:     "id matches but path doesn't (AND logic)",
			override: PageOverride{ID: float64(42), Path: "docs/*.md"},
			pageID:   42,
			pagePath: "src/main.go",
			want:     false,
		},
		{
			name:     "no id or path set",
			override: PageOverride{},
			pageID:   42,
			pagePath: "docs/intro.md",
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.override.MatchesPage(tc.pageID, tc.pagePath)
			if got != tc.want {
				t.Errorf("MatchesPage(%d, %q) = %v, want %v", tc.pageID, tc.pagePath, got, tc.want)
			}
		})
	}
}

func TestResolvePageConfig(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	profile := &TransformProfile{
		Page: PageConfig{
			Format:   "markdown",
			StripTOC: false,
			Transforms: []TransformSpec{
				{Type: "remove_macro", Params: map[string]interface{}{"macro_names": []interface{}{"toc"}}},
			},
		},
		Pages: []PageOverride{
			{
				ID:     float64(42),
				Format: "html",
			},
			{
				Path:           "api/*",
				SkipTransforms: true,
			},
			{
				ID:       float64(99),
				StripTOC: boolPtr(true),
				Transforms: []TransformSpec{
					{Type: "remove_element", Params: map[string]interface{}{"selectors": []interface{}{"div.nav"}}},
				},
			},
		},
	}

	// Default (no override matches)
	cfg, skip, skipContent := profile.ResolvePageConfig(1, "unmatched/path")
	if skip {
		t.Error("default: unexpected skip")
	}
	if skipContent {
		t.Error("default: unexpected skipContent")
	}
	if cfg.Format != "markdown" {
		t.Errorf("default: Format = %q, want %q", cfg.Format, "markdown")
	}
	if len(cfg.Transforms) != 1 {
		t.Errorf("default: len(Transforms) = %d, want 1", len(cfg.Transforms))
	}

	// Override format
	cfg, skip, _ = profile.ResolvePageConfig(42, "")
	if skip {
		t.Error("id=42: unexpected skip")
	}
	if cfg.Format != "html" {
		t.Errorf("id=42: Format = %q, want %q", cfg.Format, "html")
	}

	// Skip transforms
	cfg, _, _ = profile.ResolvePageConfig(1, "api/users")
	if len(cfg.Transforms) != 0 {
		t.Errorf("api/*: len(Transforms) = %d, want 0", len(cfg.Transforms))
	}

	// Append transforms
	cfg, _, _ = profile.ResolvePageConfig(99, "")
	if !cfg.StripTOC {
		t.Error("id=99: StripTOC = false, want true")
	}
	if len(cfg.Transforms) != 2 {
		t.Errorf("id=99: len(Transforms) = %d, want 2", len(cfg.Transforms))
	}
}

func TestResolvePageConfigSkipContent(t *testing.T) {
	profile := &TransformProfile{
		Page: PageConfig{
			Format: "markdown",
		},
		Pages: []PageOverride{
			{
				ID:          float64(10),
				Skip:        true,
			},
			{
				ID:          float64(20),
				SkipContent: true,
			},
		},
	}

	// skip:true → skip=true, skipContent=false
	_, skip, skipContent := profile.ResolvePageConfig(10, "")
	if !skip {
		t.Error("id=10: expected skip=true")
	}
	if skipContent {
		t.Error("id=10: expected skipContent=false when skip=true")
	}

	// skip_content:true → skip=false, skipContent=true
	_, skip, skipContent = profile.ResolvePageConfig(20, "")
	if skip {
		t.Error("id=20: unexpected skip")
	}
	if !skipContent {
		t.Error("id=20: expected skipContent=true")
	}

	// No match → both false
	_, skip, skipContent = profile.ResolvePageConfig(99, "")
	if skip {
		t.Error("id=99: unexpected skip")
	}
	if skipContent {
		t.Error("id=99: unexpected skipContent")
	}
}

func TestResolveFlatten(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	profile := &TransformProfile{
		Pages: []PageOverride{
			{
				Path:    "*/API Docs/*",
				Flatten: boolPtr(true),
			},
			{
				Path:    "*/Guides/*",
				Flatten: boolPtr(false),
			},
			{
				ID:      float64(555),
				Flatten: boolPtr(true),
			},
		},
	}

	tests := []struct {
		name            string
		pageID          int
		pagePath        string
		globalFlatLeaves bool
		want            bool
	}{
		{
			name:            "no override, global false",
			pageID:          1,
			pagePath:        "unmatched/path",
			globalFlatLeaves: false,
			want:            false,
		},
		{
			name:            "no override, global true",
			pageID:          1,
			pagePath:        "unmatched/path",
			globalFlatLeaves: true,
			want:            true,
		},
		{
			name:            "path override enables flatten",
			pageID:          1,
			pagePath:        "space/API Docs/endpoints",
			globalFlatLeaves: false,
			want:            true,
		},
		{
			name:            "path override disables flatten",
			pageID:          1,
			pagePath:        "space/Guides/intro",
			globalFlatLeaves: true,
			want:            false,
		},
		{
			name:            "id override enables flatten",
			pageID:          555,
			pagePath:        "",
			globalFlatLeaves: false,
			want:            true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := profile.ResolveFlatten(tc.pageID, tc.pagePath, tc.globalFlatLeaves)
			if got != tc.want {
				t.Errorf("ResolveFlatten(%d, %q, %v) = %v, want %v",
					tc.pageID, tc.pagePath, tc.globalFlatLeaves, got, tc.want)
			}
		})
	}
}

func TestResolveFlattenDecision(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	// Anchor page 100 has per-page flatten:true; page 200 has flatten:false (explicit opt-out).
	profile := &TransformProfile{
		Pages: []PageOverride{
			{ID: float64(100), Flatten: boolPtr(true)},
			{ID: float64(200), Flatten: boolPtr(false)},
		},
	}

	tests := []struct {
		name                string
		profile             *TransformProfile
		pageID              int
		isLeaf              bool
		globalFlatLeaves    bool
		inheritedFlatten    bool
		wantFlattenThisPage bool
		wantChildInherit    bool
	}{
		// --- global flat_leaves is leaf-only and non-cascading ---
		{
			name:                "flat_leaves=true, non-leaf parent: keep folder, do not cascade",
			profile:             profile,
			pageID:              1,
			isLeaf:              false,
			globalFlatLeaves:    true,
			inheritedFlatten:    false,
			wantFlattenThisPage: false,
			wantChildInherit:    false,
		},
		{
			name:                "flat_leaves=true, leaf: flatten this page",
			profile:             profile,
			pageID:              1,
			isLeaf:              true,
			globalFlatLeaves:    true,
			inheritedFlatten:    false,
			wantFlattenThisPage: true,
			wantChildInherit:    false,
		},
		{
			name:                "flat_leaves=false, leaf: do not flatten",
			profile:             profile,
			pageID:              1,
			isLeaf:              true,
			globalFlatLeaves:    false,
			inheritedFlatten:    false,
			wantFlattenThisPage: false,
			wantChildInherit:    false,
		},

		// --- per-page flatten:true is a deep-flatten anchor ---
		{
			name:                "per-page flatten anchor (non-leaf): keep own folder, cascade to descendants",
			profile:             profile,
			pageID:              100,
			isLeaf:              false,
			globalFlatLeaves:    false,
			inheritedFlatten:    false,
			wantFlattenThisPage: false, // anchor keeps its own folder
			wantChildInherit:    true,  // but descendants land flat in it
		},
		{
			name:                "per-page flatten anchor (leaf): flatten this page",
			profile:             profile,
			pageID:              100,
			isLeaf:              true,
			globalFlatLeaves:    false,
			inheritedFlatten:    false,
			wantFlattenThisPage: true,
			wantChildInherit:    true,
		},
		{
			name:                "inside anchor's subtree: flatten this page, propagate",
			profile:             profile,
			pageID:              1,
			isLeaf:              false,
			globalFlatLeaves:    false,
			inheritedFlatten:    true,
			wantFlattenThisPage: true,
			wantChildInherit:    true,
		},
		{
			name:                "explicit flatten:false override blocks global flat_leaves on leaf",
			profile:             profile,
			pageID:              200,
			isLeaf:              true,
			globalFlatLeaves:    true,
			inheritedFlatten:    false,
			wantFlattenThisPage: true, // still leaf-flattened by global flat_leaves
			wantChildInherit:    false,
		},

		// --- nil receiver ---
		{
			name:                "nil profile, flat_leaves=true, non-leaf: do not flatten",
			profile:             nil,
			pageID:              1,
			isLeaf:              false,
			globalFlatLeaves:    true,
			inheritedFlatten:    false,
			wantFlattenThisPage: false,
			wantChildInherit:    false,
		},
		{
			name:                "nil profile, flat_leaves=true, leaf: flatten",
			profile:             nil,
			pageID:              1,
			isLeaf:              true,
			globalFlatLeaves:    true,
			inheritedFlatten:    false,
			wantFlattenThisPage: true,
			wantChildInherit:    false,
		},
		{
			name:                "nil profile, inheritedFlatten=true: flatten and propagate",
			profile:             nil,
			pageID:              1,
			isLeaf:              false,
			globalFlatLeaves:    false,
			inheritedFlatten:    true,
			wantFlattenThisPage: true,
			wantChildInherit:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotFlatten, gotChild := tc.profile.ResolveFlattenDecision(tc.pageID, "", tc.isLeaf, tc.globalFlatLeaves, tc.inheritedFlatten)
			if gotFlatten != tc.wantFlattenThisPage {
				t.Errorf("flattenThisPage = %v, want %v", gotFlatten, tc.wantFlattenThisPage)
			}
			if gotChild != tc.wantChildInherit {
				t.Errorf("childInheritedFlatten = %v, want %v", gotChild, tc.wantChildInherit)
			}
		})
	}
}

func TestResolveSkipRoot(t *testing.T) {
	profile := &TransformProfile{
		Pages: []PageOverride{
			{ID: float64(42), SkipRoot: true},
			{Path: "*/Containers/*", SkipRoot: true},
			{ID: float64(99)}, // unrelated override, must not match
		},
	}

	tests := []struct {
		name     string
		pageID   int
		pagePath string
		want     bool
	}{
		{name: "no override", pageID: 1, pagePath: "unmatched", want: false},
		{name: "id override sets skip_root", pageID: 42, want: true},
		{name: "path override sets skip_root", pageID: 1, pagePath: "space/Containers/foo", want: true},
		{name: "matching override without skip_root", pageID: 99, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := profile.ResolveSkipRoot(tc.pageID, tc.pagePath)
			if got != tc.want {
				t.Errorf("ResolveSkipRoot(%d, %q) = %v, want %v",
					tc.pageID, tc.pagePath, got, tc.want)
			}
		})
	}
}

func TestLoadProfileWithSkipRoot(t *testing.T) {
	yaml := `
pages:
  - id: 12345
    skip_root: true
  - id: 67890
    skip_root: false
  - id: 11111
`
	p, err := LoadProfile([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if len(p.Pages) != 3 {
		t.Fatalf("len(Pages) = %d, want 3", len(p.Pages))
	}
	if !p.Pages[0].SkipRoot {
		t.Error("Pages[0].SkipRoot should be true")
	}
	if p.Pages[1].SkipRoot {
		t.Error("Pages[1].SkipRoot should be false")
	}
	if p.Pages[2].SkipRoot {
		t.Error("Pages[2].SkipRoot should be false (default)")
	}
}

func TestLoadProfileWithFlatten(t *testing.T) {
	yaml := `
pages:
  - path: "*/API Docs/*"
    flatten: true
  - path: "*/Guides/*"
    flatten: false
  - id: 42
    format: html
`
	p, err := LoadProfile([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if len(p.Pages) != 3 {
		t.Fatalf("len(Pages) = %d, want 3", len(p.Pages))
	}
	if p.Pages[0].Flatten == nil || *p.Pages[0].Flatten != true {
		t.Error("Pages[0].Flatten should be true")
	}
	if p.Pages[1].Flatten == nil || *p.Pages[1].Flatten != false {
		t.Error("Pages[1].Flatten should be false")
	}
	if p.Pages[2].Flatten != nil {
		t.Error("Pages[2].Flatten should be nil (not specified)")
	}
}

func TestResolveProfile(t *testing.T) {
	// Test loading from file path
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "test.yaml")
	err := os.WriteFile(profilePath, []byte(`
page:
  format: html
`), 0644)
	if err != nil {
		t.Fatalf("write test file: %v", err)
	}

	p, err := ResolveProfile(profilePath)
	if err != nil {
		t.Fatalf("ResolveProfile(%q): %v", profilePath, err)
	}
	if p.Page.Format != "html" {
		t.Errorf("Format = %q, want %q", p.Page.Format, "html")
	}

	// Test not found
	_, err = ResolveProfile("nonexistent-profile-xyz")
	if err == nil {
		t.Error("expected error for nonexistent profile, got nil")
	}
}

func TestApplySetOverrides(t *testing.T) {
	p := &TransformProfile{}

	overrides := map[string]string{
		"folder.naming":       "slug",
		"folder.length_limit": "100",
		"folder.flat_leaves":  "true",
		"page.format":         "storage",
		"page.strip_toc":      "1",
		"page.save_metadata":  "false",
	}

	if err := ApplySetOverrides(p, overrides); err != nil {
		t.Fatalf("ApplySetOverrides: %v", err)
	}

	if p.Folder.Naming != "slug" {
		t.Errorf("Folder.Naming = %q, want %q", p.Folder.Naming, "slug")
	}
	if p.Folder.LengthLimit != 100 {
		t.Errorf("Folder.LengthLimit = %d, want 100", p.Folder.LengthLimit)
	}
	if !p.Folder.FlatLeaves {
		t.Error("Folder.FlatLeaves = false, want true")
	}
	if p.Page.Format != "storage" {
		t.Errorf("Page.Format = %q, want %q", p.Page.Format, "storage")
	}
	if !p.Page.StripTOC {
		t.Error("Page.StripTOC = false, want true")
	}
	if p.Page.SaveMetadata {
		t.Error("Page.SaveMetadata = true, want false")
	}
}

func TestApplySetOverridesErrors(t *testing.T) {
	p := &TransformProfile{}

	// Unknown key
	err := ApplySetOverrides(p, map[string]string{"bogus.key": "val"})
	if err == nil {
		t.Error("expected error for unknown key")
	}

	// Invalid int
	err = ApplySetOverrides(p, map[string]string{"folder.length_limit": "abc"})
	if err == nil {
		t.Error("expected error for invalid int")
	}

	// Invalid bool
	err = ApplySetOverrides(p, map[string]string{"folder.flat_leaves": "maybe"})
	if err == nil {
		t.Error("expected error for invalid bool")
	}
}

func TestBuildPipeline(t *testing.T) {
	reg := DefaultRegistry()

	specs := []TransformSpec{
		{
			Type: "remove_macro",
			Params: map[string]interface{}{
				"macro_names": []interface{}{"toc", "info"},
			},
		},
		{
			Type: "remove_element",
			Params: map[string]interface{}{
				"selectors": []interface{}{"div.sidebar"},
			},
		},
	}

	pipeline, err := BuildPipeline(specs, reg)
	if err != nil {
		t.Fatalf("BuildPipeline: %v", err)
	}
	if pipeline.Len() != 2 {
		t.Errorf("pipeline.Len() = %d, want 2", pipeline.Len())
	}
}

func TestBuildPipelineUnknownType(t *testing.T) {
	reg := DefaultRegistry()
	specs := []TransformSpec{{Type: "bogus_transform"}}
	_, err := BuildPipeline(specs, reg)
	if err == nil {
		t.Error("expected error for unknown transform type")
	}
}

func TestRegistryBuildRemoveMacroMissingParam(t *testing.T) {
	reg := DefaultRegistry()
	_, err := reg.Build(TransformSpec{Type: "remove_macro", Params: map[string]interface{}{}})
	if err == nil {
		t.Error("expected error for remove_macro with no macro_names")
	}
}
