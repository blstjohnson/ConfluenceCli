package transforms

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TransformProfile is the top-level YAML structure for an export transform
// profile (Confluence → local files). An empty Kind is treated as "export" for
// back-compat with profiles authored before the discriminator was introduced.
type TransformProfile struct {
	Kind   string         `yaml:"kind,omitempty"`
	Folder FolderConfig   `yaml:"folder"`
	Page   PageConfig     `yaml:"page"`
	Pages  []PageOverride `yaml:"pages"`
}

// FolderConfig controls folder naming and structure.
type FolderConfig struct {
	Naming      string `yaml:"naming"`       // e.g. "slug", "title", "id"
	LengthLimit int    `yaml:"length_limit"`  // max folder name length (0 = no limit)
	FlatLeaves  bool   `yaml:"flat_leaves"`   // flatten leaf pages into parent dir
}

// PageConfig holds default page-level settings.
type PageConfig struct {
	Format       string           `yaml:"format"`        // e.g. "markdown", "storage", "html"
	StripTOC     bool             `yaml:"strip_toc"`
	SaveMetadata bool             `yaml:"save_metadata"`
	Transforms   []TransformSpec  `yaml:"transforms"`

	// RewriteLinks enables rewriting of Confluence internal page links to local
	// files when exporting a single page. The page ID -> file map is built at
	// runtime by scanning RefsDirs. Providing RefsDirs implies RewriteLinks.
	RewriteLinks bool `yaml:"rewrite_links,omitempty"`

	// RefsDirs lists local directories of previously-exported pages used to
	// resolve internal links to local file paths.
	RefsDirs []string `yaml:"refs_dirs,omitempty"`

	// RefsLinkStyle controls how resolved links are written: "relative" (default,
	// relative to the output file's directory) or "absolute".
	RefsLinkStyle string `yaml:"refs_link_style,omitempty"`
}

// PageOverride applies settings to specific pages. Matching is by ID (single or
// list) and/or path glob. Both can be specified (AND logic).
type PageOverride struct {
	ID             interface{}      `yaml:"id"`              // int, string, or []interface{}
	Path           string           `yaml:"path"`            // glob pattern matched against page path
	Format         string           `yaml:"format,omitempty"`
	StripTOC       *bool            `yaml:"strip_toc,omitempty"`
	SaveMetadata   *bool            `yaml:"save_metadata,omitempty"`
	Flatten        *bool            `yaml:"flatten,omitempty"`
	Transforms     []TransformSpec  `yaml:"transforms,omitempty"`
	SkipTransforms bool             `yaml:"skip_transforms,omitempty"`
	Skip           bool             `yaml:"skip,omitempty"`
	SkipContent    bool             `yaml:"skip_content,omitempty"`
	SkipRoot       bool             `yaml:"skip_root,omitempty"`
}

// TransformSpec is a single transform entry in the YAML pipeline.
// The type field selects the transform; params vary by type.
type TransformSpec struct {
	Type   string                 `yaml:"type"`
	Params map[string]interface{} `yaml:"params,omitempty"`
}

// MatchesPage reports whether the override applies to the given page.
func (o *PageOverride) MatchesPage(pageID int, pagePath string) bool {
	idMatch := o.matchesID(pageID)
	pathMatch := o.matchesPath(pagePath)

	hasID := o.ID != nil
	hasPath := o.Path != ""

	if hasID && hasPath {
		return idMatch && pathMatch
	}
	if hasID {
		return idMatch
	}
	if hasPath {
		return pathMatch
	}
	return false
}

func (o *PageOverride) matchesID(pageID int) bool {
	if o.ID == nil {
		return false
	}
	switch v := o.ID.(type) {
	case int:
		return pageID == v
	case float64:
		return pageID == int(v)
	case string:
		return fmt.Sprintf("%d", pageID) == v
	case []interface{}:
		for _, item := range v {
			switch id := item.(type) {
			case int:
				if pageID == id {
					return true
				}
			case float64:
				if pageID == int(id) {
					return true
				}
			case string:
				if fmt.Sprintf("%d", pageID) == id {
					return true
				}
			}
		}
	}
	return false
}

func (o *PageOverride) matchesPath(pagePath string) bool {
	if o.Path == "" {
		return false
	}
	matched, err := filepath.Match(o.Path, pagePath)
	if err != nil {
		return false
	}
	return matched
}

// LoadProfile loads an export TransformProfile from YAML bytes. It rejects
// profiles whose kind is anything other than "" (legacy) or "export".
func LoadProfile(data []byte) (*TransformProfile, error) {
	var p TransformProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse transform profile: %w", err)
	}
	if p.Kind != "" && p.Kind != "export" {
		return nil, fmt.Errorf("transform profile kind %q is not an export profile (use the import loader for kind: import)", p.Kind)
	}
	return &p, nil
}

// peekProfileKind extracts the top-level "kind:" field from raw YAML without
// fully decoding the profile. Returns empty string if the field is absent.
// Callers can use this to route a YAML file to the right loader.
func peekProfileKind(data []byte) (string, error) {
	var head struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return "", fmt.Errorf("parse profile kind: %w", err)
	}
	return head.Kind, nil
}

// LoadProfileFromFile loads a TransformProfile from a YAML file path.
func LoadProfileFromFile(path string) (*TransformProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read transform profile %q: %w", path, err)
	}
	return LoadProfile(data)
}

// ResolveProfile resolves a --transform flag value to a TransformProfile.
// Resolution order:
//  1. If value is a file path that exists, load from that file.
//  2. Look up ~/.confcli/transformations/<name>.yaml
//  3. Return error with helpful message.
func ResolveProfile(value string) (*TransformProfile, error) {
	// 1. Direct file path
	if _, err := os.Stat(value); err == nil {
		return LoadProfileFromFile(value)
	}

	// 2. Named profile in config dir
	configDir, err := transformationsDir()
	if err == nil {
		candidates := []string{
			filepath.Join(configDir, value+".yaml"),
			filepath.Join(configDir, value+".yml"),
			filepath.Join(configDir, value),
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				return LoadProfileFromFile(path)
			}
		}
	}

	// 3. Not found
	return nil, fmt.Errorf(
		"transform profile %q not found: checked as file path and in ~/.confcli/transformations/",
		value,
	)
}

// transformationsDir returns ~/.confcli/transformations/.
func transformationsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".confcli", "transformations"), nil
}

// ResolvePageConfig returns the effective PageConfig for a specific page,
// merging the profile defaults with any matching page overrides.
// If a matching override has Skip: true, the second return value is true
// (skip entire subtree). If SkipContent: true, the third return value is
// true (skip content file only, still process children).
func (p *TransformProfile) ResolvePageConfig(pageID int, pagePath string) (PageConfig, bool, bool) {
	result := p.Page
	skipContent := false

	for _, override := range p.Pages {
		if !override.MatchesPage(pageID, pagePath) {
			continue
		}

		if override.Skip {
			return result, true, false
		}

		if override.SkipContent {
			skipContent = true
		}

		if override.Format != "" {
			result.Format = override.Format
		}
		if override.StripTOC != nil {
			result.StripTOC = *override.StripTOC
		}
		if override.SaveMetadata != nil {
			result.SaveMetadata = *override.SaveMetadata
		}

		if override.SkipTransforms {
			result.Transforms = nil
		} else if len(override.Transforms) > 0 {
			result.Transforms = append(result.Transforms, override.Transforms...)
		}
	}

	return result, false, skipContent
}

// ResolveFlatten returns the effective flatten-leaf decision for a specific page.
// Per-page overrides take precedence over the global FlatLeaves setting.
func (p *TransformProfile) ResolveFlatten(pageID int, pagePath string, globalFlatLeaves bool) bool {
	for _, override := range p.Pages {
		if !override.MatchesPage(pageID, pagePath) {
			continue
		}
		if override.Flatten != nil {
			return *override.Flatten
		}
	}
	return globalFlatLeaves
}

// ResolveFlattenDecision computes how a page should be placed in the export
// tree. It encapsulates the interaction between three settings:
//
//   - folder.flat_leaves (globalFlatLeaves): when true, LEAF pages (no
//     children) have their content file written directly into the parent
//     directory rather than getting their own subfolder. Non-leaf pages keep
//     their folder. This setting does NOT cascade.
//   - per-page flatten: true (matched via overrides): the page is a
//     deep-flatten ANCHOR. The anchor itself keeps its own folder; its
//     descendants at any depth land flat as siblings inside that folder.
//   - inheritedFlatten: true when an ancestor was a deep-flatten anchor, so
//     this page lives inside such an anchor's subtree.
//
// Returns:
//   - flattenThisPage: when true, the page's content file goes into parentDir
//     and no folder is created for it.
//   - childInheritedFlatten: the value to pass as inheritedFlatten when
//     recursing into descendants.
//
// Safe to call on a nil receiver.
func (p *TransformProfile) ResolveFlattenDecision(pageID int, pagePath string, isLeaf, globalFlatLeaves, inheritedFlatten bool) (flattenThisPage bool, childInheritedFlatten bool) {
	perPageFlatten := false
	if p != nil {
		perPageFlatten = p.ResolveFlatten(pageID, pagePath, false)
	}
	flattenThisPage = inheritedFlatten || (isLeaf && (globalFlatLeaves || perPageFlatten))
	childInheritedFlatten = inheritedFlatten || perPageFlatten
	return
}

// ResolveSkipRoot reports whether the page should be exported as a "transparent
// container": no folder, no .md file for the page itself, with children
// promoted into the parent directory. Returns true if any matching override
// has skip_root: true.
func (p *TransformProfile) ResolveSkipRoot(pageID int, pagePath string) bool {
	for _, override := range p.Pages {
		if !override.MatchesPage(pageID, pagePath) {
			continue
		}
		if override.SkipRoot {
			return true
		}
	}
	return false
}

// ApplySetOverrides applies --set key=value overrides to a TransformProfile.
// Supported dot-paths:
//
//	folder.naming, folder.length_limit, folder.flat_leaves
//	page.format, page.strip_toc, page.save_metadata
//	page.rewrite_links, page.refs_dir (alias page.refs_dirs), page.refs_link_style
//	page.clear_macros, page.expand_macros
//
// page.refs_dir / page.refs_dirs and page.clear_macros / page.expand_macros take
// comma-separated values. clear_macros appends a remove_macro transform that
// drops the macro and its content; expand_macros appends one that preserves the
// inner content (unwrap).
func ApplySetOverrides(p *TransformProfile, overrides map[string]string) error {
	for key, val := range overrides {
		if err := applySetOverride(p, key, val); err != nil {
			return fmt.Errorf("--set %s=%s: %w", key, val, err)
		}
	}
	return nil
}

func applySetOverride(p *TransformProfile, key, val string) error {
	switch key {
	case "folder.naming":
		p.Folder.Naming = val
	case "folder.length_limit":
		n, err := parseInt(val)
		if err != nil {
			return err
		}
		p.Folder.LengthLimit = n
	case "folder.flat_leaves":
		b, err := parseBool(val)
		if err != nil {
			return err
		}
		p.Folder.FlatLeaves = b
	case "page.format":
		p.Page.Format = val
	case "page.strip_toc":
		b, err := parseBool(val)
		if err != nil {
			return err
		}
		p.Page.StripTOC = b
	case "page.save_metadata":
		b, err := parseBool(val)
		if err != nil {
			return err
		}
		p.Page.SaveMetadata = b
	case "page.rewrite_links":
		b, err := parseBool(val)
		if err != nil {
			return err
		}
		p.Page.RewriteLinks = b
	case "page.refs_dir", "page.refs_dirs":
		p.Page.RefsDirs = splitList(val)
	case "page.refs_link_style":
		if val != "relative" && val != "absolute" {
			return fmt.Errorf("expected 'relative' or 'absolute', got %q", val)
		}
		p.Page.RefsLinkStyle = val
	case "page.clear_macros":
		names := splitList(val)
		if len(names) == 0 {
			return fmt.Errorf("expected comma-separated macro names")
		}
		p.Page.Transforms = append(p.Page.Transforms, TransformSpec{
			Type:   "remove_macro",
			Params: map[string]interface{}{"macro_names": names},
		})
	case "page.expand_macros":
		names := splitList(val)
		if len(names) == 0 {
			return fmt.Errorf("expected comma-separated macro names")
		}
		p.Page.Transforms = append(p.Page.Transforms, TransformSpec{
			Type:   "remove_macro",
			Params: map[string]interface{}{"macro_names": names, "preserve_content": true},
		})
	default:
		return fmt.Errorf("unknown override key %q", key)
	}
	return nil
}

// splitList splits a comma-separated --set value into trimmed, non-empty items.
func splitList(val string) []string {
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, fmt.Errorf("expected integer, got %q", s)
	}
	return n, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("expected boolean, got %q", s)
	}
}

// BuildPipeline creates a Pipeline from a slice of TransformSpec using the
// provided registry to instantiate each transform.
func BuildPipeline(specs []TransformSpec, reg *Registry) (*Pipeline, error) {
	transforms := make([]Transform, 0, len(specs))
	for i, spec := range specs {
		t, err := reg.Build(spec)
		if err != nil {
			return nil, fmt.Errorf("transform[%d] (type=%q): %w", i, spec.Type, err)
		}
		transforms = append(transforms, t)
	}
	return NewPipeline(transforms...), nil
}
