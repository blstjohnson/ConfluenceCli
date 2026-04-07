package transforms

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// TransformProfile is the top-level YAML structure for a transform profile.
type TransformProfile struct {
	Folder  FolderConfig  `yaml:"folder"`
	Page    PageConfig    `yaml:"page"`
	Pages   []PageOverride `yaml:"pages"`
}

// FolderConfig controls folder naming and structure.
type FolderConfig struct {
	Naming      string `yaml:"naming"`       // e.g. "slug", "title", "id"
	LengthLimit int    `yaml:"length_limit"`  // max folder name length (0 = no limit)
	FlatLeaves  bool   `yaml:"flat_leaves"`   // flatten leaf pages into parent dir
	SkipRoot    bool   `yaml:"skip_root"`     // omit root space folder
}

// PageConfig holds default page-level settings.
type PageConfig struct {
	Format       string           `yaml:"format"`        // e.g. "markdown", "storage", "html"
	StripTOC     bool             `yaml:"strip_toc"`
	SaveMetadata bool             `yaml:"save_metadata"`
	Transforms   []TransformSpec  `yaml:"transforms"`
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

// LoadProfile loads a TransformProfile from YAML bytes.
func LoadProfile(data []byte) (*TransformProfile, error) {
	var p TransformProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse transform profile: %w", err)
	}
	return &p, nil
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
// If a matching override has Skip: true, the second return value is true.
func (p *TransformProfile) ResolvePageConfig(pageID int, pagePath string) (PageConfig, bool) {
	result := p.Page

	for _, override := range p.Pages {
		if !override.MatchesPage(pageID, pagePath) {
			continue
		}

		if override.Skip {
			return result, true
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

	return result, false
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

// ApplySetOverrides applies --set key=value overrides to a TransformProfile.
// Supported dot-paths:
//
//	folder.naming, folder.length_limit, folder.flat_leaves, folder.skip_root
//	page.format, page.strip_toc, page.save_metadata
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
	case "folder.skip_root":
		b, err := parseBool(val)
		if err != nil {
			return err
		}
		p.Folder.SkipRoot = b
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
	default:
		return fmt.Errorf("unknown override key %q", key)
	}
	return nil
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
