package transforms

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// ImportProfile is the top-level YAML structure for an import profile
// (local files → Confluence). It is parallel to TransformProfile but covers
// the reverse direction: folder→page-tree mapping, the transformation
// pipeline applied to source files on upload, and explicit path→pageId
// overrides that bypass identity-label resolution at sync time.
//
// Import profiles share the ~/.confcli/transformations/ directory with
// export profiles; the "kind: import" discriminator distinguishes them.
type ImportProfile struct {
	Kind       string          `yaml:"kind"`
	Tree       TreeConfig      `yaml:"tree"`
	Transforms []TransformSpec `yaml:"transforms"`
	Overrides  []PathOverride  `yaml:"overrides"`
}

// TreeConfig controls how a folder tree is projected onto a Confluence page
// tree.
type TreeConfig struct {
	// FolderPage names a file (case-insensitive match) whose contents become
	// the parent page for its containing folder. Empty = no marker file; the
	// sync engine creates an empty stub parent page per folder.
	FolderPage string `yaml:"folder_page"`

	// Skip is a list of doublestar globs. Files or folders matching any of
	// them are excluded from the sync entirely.
	Skip []string `yaml:"skip"`

	// Flatten is a list of doublestar globs. Folders matching any of them
	// are not created as their own pages — their child pages are promoted
	// into the parent folder's page.
	Flatten []string `yaml:"flatten"`
}

// PathOverride applies per-path settings during sync. The Path is a
// doublestar glob matched against the page path (repo-relative file path).
type PathOverride struct {
	Path   string `yaml:"path"`
	PageID int    `yaml:"page_id,omitempty"`
	Skip   bool   `yaml:"skip,omitempty"`
}

// LoadImportProfile parses an ImportProfile from YAML bytes and validates it.
func LoadImportProfile(data []byte) (*ImportProfile, error) {
	var p ImportProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse import profile: %w", err)
	}
	if p.Kind != "import" {
		return nil, fmt.Errorf("import profile must have kind: import (got %q)", p.Kind)
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// LoadImportProfileFromFile loads and validates an ImportProfile from disk.
func LoadImportProfileFromFile(path string) (*ImportProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read import profile %q: %w", path, err)
	}
	return LoadImportProfile(data)
}

// ResolveImportProfile resolves a --profile flag value to an ImportProfile.
// Resolution order mirrors ResolveProfile:
//  1. If value is an existing file path, load from that file.
//  2. Look up ~/.confcli/transformations/<name>.{yaml,yml} or <name>.
//  3. Return a not-found error.
func ResolveImportProfile(value string) (*ImportProfile, error) {
	if _, err := os.Stat(value); err == nil {
		return LoadImportProfileFromFile(value)
	}

	configDir, err := transformationsDir()
	if err == nil {
		candidates := []string{
			filepath.Join(configDir, value+".yaml"),
			filepath.Join(configDir, value+".yml"),
			filepath.Join(configDir, value),
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				return LoadImportProfileFromFile(path)
			}
		}
	}

	return nil, fmt.Errorf(
		"import profile %q not found: checked as file path and in ~/.confcli/transformations/",
		value,
	)
}

func (p *ImportProfile) validate() error {
	for i, pat := range p.Tree.Skip {
		if !doublestar.ValidatePattern(pat) {
			return fmt.Errorf("tree.skip[%d]: invalid glob %q", i, pat)
		}
	}
	for i, pat := range p.Tree.Flatten {
		if !doublestar.ValidatePattern(pat) {
			return fmt.Errorf("tree.flatten[%d]: invalid glob %q", i, pat)
		}
	}
	for i, o := range p.Overrides {
		if o.Path == "" {
			return fmt.Errorf("overrides[%d]: path is required", i)
		}
		if !doublestar.ValidatePattern(o.Path) {
			return fmt.Errorf("overrides[%d]: invalid glob %q", i, o.Path)
		}
		if o.Skip && o.PageID != 0 {
			return fmt.Errorf("overrides[%d]: skip and page_id are mutually exclusive", i)
		}
	}
	return nil
}

// MatchesSkip reports whether path matches any tree.skip glob.
func (p *ImportProfile) MatchesSkip(path string) bool {
	return matchesAny(p.Tree.Skip, path)
}

// MatchesFlatten reports whether path matches any tree.flatten glob.
func (p *ImportProfile) MatchesFlatten(path string) bool {
	return matchesAny(p.Tree.Flatten, path)
}

// FindOverride returns the first override whose Path glob matches path,
// or nil if none match. First-match-wins follows document order, which is
// the most predictable resolution for human-authored YAML.
func (p *ImportProfile) FindOverride(path string) *PathOverride {
	for i := range p.Overrides {
		o := &p.Overrides[i]
		if matched, _ := doublestar.Match(o.Path, path); matched {
			return o
		}
	}
	return nil
}

func matchesAny(patterns []string, path string) bool {
	for _, pat := range patterns {
		if matched, _ := doublestar.Match(pat, path); matched {
			return true
		}
	}
	return false
}
