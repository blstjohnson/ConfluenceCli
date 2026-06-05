package transforms

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

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
	PlantUML   PlantUMLConfig  `yaml:"plantuml"`
	GitFiles   GitFilesConfig  `yaml:"git_files"`

	// titleRegex is index-aligned with Tree.Title.Rewrites and holds the
	// compiled patterns. Populated by validate (or lazily by TitleFor when
	// the profile was constructed directly without going through Load*).
	titleRegex []*regexp.Regexp
}

// PlantUMLConfig controls how markdown links to .puml files are
// rewritten into Confluence macros at sync time. When Macro or
// Parameters is empty the rewriter is disabled and .puml links stay
// as plain text links.
//
// The shape is intentionally generic: different Confluence plugins
// take different macro and parameter names (the "View Git File"
// plugin uses macro="view-git-file" with parameters like path,
// branch, repository-id, renderpuml; a URL-based PlantUML plugin
// might use macro="plantuml" with just a "url" parameter). Each
// parameter value supports two placeholders that the rewriter
// substitutes per .puml link:
//
//	{path}   — repo-root-relative slash path of the .puml file
//	{branch} — git branch name. By default the short name (e.g.
//	           "feature/x"); set branch_ref: remote|local to expand it
//	           to the full ref form (refs/remotes/origin/feature/x) that
//	           the view-git-file plugin requires.
//
// Static parameters (no placeholders) are emitted verbatim, useful
// for things like repository-id, renderpuml, renderpanel toggles.
type PlantUMLConfig struct {
	// Macro is the Confluence macro name written into the ac:name
	// attribute (e.g. "view-git-file", "plantuml").
	Macro string `yaml:"macro"`

	// Parameters maps ac:parameter names to value templates. Values
	// may reference {path} and {branch}; any other text is verbatim.
	Parameters map[string]string `yaml:"parameters"`

	// Branch overrides the git branch auto-detected from .git/HEAD.
	// Useful for pinning rendered diagrams to a release branch.
	Branch string `yaml:"branch"`

	// RepoRoot overrides the git repo root auto-detected by walking up
	// from --from. Absolute path, or path relative to --from. Set this
	// when the sync source is not inside a git working tree.
	RepoRoot string `yaml:"repo_root"`

	// BranchRef expands a bare {branch} value to the full git ref form the
	// Confluence git plugin expects (the view-git-file plugin errors on a
	// bare short name). One of:
	//   ""/"short" — leave {branch} as the short name (default)
	//   "remote"   — refs/remotes/origin/<branch>
	//   "local"    — refs/heads/<branch>
	// A {branch} value already in refs/... form is left untouched, so the
	// expansion is idempotent.
	BranchRef string `yaml:"branch_ref"`
}

// GitFilesConfig rewrites links to non-markdown files in the repo into
// the same kind of Confluence macro PlantUMLConfig uses — typically
// view-git-file without the renderpuml flag, so the file is displayed
// as source rather than rendered. Catches whatever the .md and .puml
// rewriters did not, e.g. *.yaml, *.json, *.sql references.
//
// Skipped (passed through unchanged):
//   - image embeds (![alt](href))
//   - external URLs (containing ://) and mailto:/tel:/#anchor
//   - .md, .markdown, .puml, .plantuml — handled by the other rewriters
//   - image extensions (.png, .jpg, .jpeg, .gif, .svg, .webp, .bmp,
//     .ico) — rendering an image as a code panel would be unhelpful
//
// Same {path} / {branch} placeholder rules as PlantUMLConfig. The
// shared Branch and RepoRoot from PlantUMLConfig apply here too; the
// fields below override when set.
type GitFilesConfig struct {
	Macro      string            `yaml:"macro"`
	Parameters map[string]string `yaml:"parameters"`
	Branch     string            `yaml:"branch"`
	RepoRoot   string            `yaml:"repo_root"`
	// Extensions optionally restricts which file extensions the
	// rewriter handles. Empty = catch-all (everything not excluded
	// by the rules above). Case-insensitive, with or without leading
	// dot: ["yaml", ".json"] both work.
	Extensions []string `yaml:"extensions"`

	// Mode picks how matched file links are rewritten:
	//   "link"   (default) — wrap the href in the configured view-git-file
	//                        macro. The file stays in git; Confluence
	//                        fetches it at render time.
	//   "inline" —          read the file from --from at sync time and
	//                        embed its contents in a Confluence "code"
	//                        macro on the page.
	Mode string `yaml:"mode"`

	// BranchRef expands a bare {branch} value to a full git ref, exactly as
	// PlantUMLConfig.BranchRef does. See that field for the accepted values.
	BranchRef string `yaml:"branch_ref"`

	// PerExtension overrides Mode for specific extensions (lowercased,
	// with or without leading dot). Useful when most files should link
	// but a few configuration formats should inline (or vice versa).
	PerExtension map[string]string `yaml:"per_extension"`

	// Inline tunes the inline-mode emitter.
	Inline GitFilesInlineConfig `yaml:"inline"`
}

// GitFilesInlineConfig configures inline-mode emission for GitFilesConfig.
type GitFilesInlineConfig struct {
	// MaxBytes caps the source file size that may be inlined. Files
	// larger than this fall back to link mode (with a warning). Zero or
	// negative means no cap.
	MaxBytes int64 `yaml:"max_bytes"`
}

var validGitFilesModes = map[string]struct{}{
	"":       {},
	"link":   {},
	"inline": {},
}

// BranchRefPrefix returns the git ref prefix for a branch_ref mode, and
// whether expansion is enabled. Unknown/empty modes disable expansion.
// "origin" is accepted as an alias for "remote".
func BranchRefPrefix(mode string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "remote", "origin":
		return "refs/remotes/origin/", true
	case "local":
		return "refs/heads/", true
	default:
		return "", false
	}
}

// ExpandBranchRef applies a branch_ref mode to a branch name. An empty
// branch, a branch already in refs/... form, or a disabled mode is returned
// unchanged, making the expansion idempotent and safe to re-run.
func ExpandBranchRef(branch, mode string) string {
	if branch == "" || strings.HasPrefix(branch, "refs/") {
		return branch
	}
	prefix, ok := BranchRefPrefix(mode)
	if !ok {
		return branch
	}
	return prefix + branch
}

var validBranchRefs = map[string]struct{}{
	"": {}, "short": {}, "remote": {}, "origin": {}, "local": {},
}

func validateBranchRef(section, v string) error {
	if _, ok := validBranchRefs[strings.ToLower(strings.TrimSpace(v))]; !ok {
		return fmt.Errorf("%s.branch_ref: must be one of short|remote|local (got %q)", section, v)
	}
	return nil
}

func (c *GitFilesConfig) validate() error {
	if _, ok := validGitFilesModes[c.Mode]; !ok {
		return fmt.Errorf("git_files.mode: must be one of link|inline (got %q)", c.Mode)
	}
	for ext, mode := range c.PerExtension {
		if _, ok := validGitFilesModes[mode]; !ok || mode == "" {
			return fmt.Errorf("git_files.per_extension[%q]: must be link|inline (got %q)", ext, mode)
		}
	}
	return nil
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

	// Title controls how page titles are derived from filenames.
	Title TitleConfig `yaml:"title"`
}

// TitleConfig drives the file-name → Confluence-page-title rewrite that
// runs after the .md extension is stripped. The rewrites apply only to
// the basename stem; the relative path used for identity and forward-link
// resolution is not touched.
type TitleConfig struct {
	// Rewrites are applied in order to the stem. Each entry is a Go RE2
	// regex; references like $1 in the replacement work as ReplaceAllString.
	Rewrites []TitleRewrite `yaml:"rewrites"`

	// Trim collapses surrounding whitespace after the rewrites apply.
	Trim bool `yaml:"trim"`
}

// TitleRewrite is one regex/replacement pair for TitleConfig.Rewrites.
type TitleRewrite struct {
	Pattern     string `yaml:"pattern"`
	Replacement string `yaml:"replacement"`
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
	if err := p.compileTitleRewrites(); err != nil {
		return err
	}
	if err := p.GitFiles.validate(); err != nil {
		return err
	}
	if err := validateBranchRef("plantuml", p.PlantUML.BranchRef); err != nil {
		return err
	}
	if err := validateBranchRef("git_files", p.GitFiles.BranchRef); err != nil {
		return err
	}
	return nil
}

// compileTitleRewrites compiles every Tree.Title.Rewrites pattern and
// caches the result on the profile. Returns the first compile error.
func (p *ImportProfile) compileTitleRewrites() error {
	if len(p.Tree.Title.Rewrites) == 0 {
		p.titleRegex = nil
		return nil
	}
	compiled := make([]*regexp.Regexp, len(p.Tree.Title.Rewrites))
	for i, rw := range p.Tree.Title.Rewrites {
		if rw.Pattern == "" {
			return fmt.Errorf("tree.title.rewrites[%d]: pattern is required", i)
		}
		re, err := regexp.Compile(rw.Pattern)
		if err != nil {
			return fmt.Errorf("tree.title.rewrites[%d]: invalid regex %q: %w", i, rw.Pattern, err)
		}
		compiled[i] = re
	}
	p.titleRegex = compiled
	return nil
}

// TitleFor derives the Confluence page title from a sync-root-relative
// markdown path. It strips the .md extension, then applies any configured
// title.rewrites in order, then optionally trims surrounding whitespace.
//
// When called on a profile that bypassed validate (e.g. constructed inline
// in a test), the rewrites are compiled lazily on first call. Patterns
// that fail to compile are silently skipped — validate catches them up
// front for the normal load path.
func (p *ImportProfile) TitleFor(relPath string) string {
	base := path.Base(relPath)
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	rewrites := p.Tree.Title.Rewrites
	if len(rewrites) > 0 {
		if len(p.titleRegex) != len(rewrites) {
			compiled := make([]*regexp.Regexp, len(rewrites))
			for i, rw := range rewrites {
				if rw.Pattern == "" {
					continue
				}
				if re, err := regexp.Compile(rw.Pattern); err == nil {
					compiled[i] = re
				}
			}
			p.titleRegex = compiled
		}
		for i, re := range p.titleRegex {
			if re == nil {
				continue
			}
			base = re.ReplaceAllString(base, rewrites[i].Replacement)
		}
	}
	if p.Tree.Title.Trim {
		base = strings.TrimSpace(base)
	}
	return base
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
