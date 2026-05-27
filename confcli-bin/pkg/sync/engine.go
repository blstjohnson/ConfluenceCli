package sync

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"path"
	"sort"
	"strings"

	"confcli/pkg/models"
	"confcli/pkg/sync/identity"
	"confcli/pkg/transforms"
)

// PageLocator resolves a sync-root-relative markdown path to an existing
// Confluence page, returning nil when no page is found. *Locator satisfies
// this interface.
type PageLocator interface {
	FindByPath(ctx context.Context, spaceKey, relPath string) (*models.Page, error)
}

// ManagedPageLister enumerates Confluence pages under a sync root that
// carry a confcli-id label, so the engine can flag orphans (pages with no
// matching source file). The real implementation is wired in the sync
// command (b9f); the engine accepts the interface so plan-building stays
// network-free in unit tests.
type ManagedPageLister interface {
	ListManaged(ctx context.Context, spaceKey string, rootPageID int) ([]ManagedPage, error)
}

// ManagedPage is the subset of page state the orphan detector needs.
type ManagedPage struct {
	PageID  int
	Title   string
	IDLabel string
}

// Converter converts markdown bytes from a source file to Confluence
// storage-format XHTML. relPath is provided so converters (e.g. the
// forward-link rewriter) can resolve relative paths against the sync tree.
type Converter func(ctx context.Context, markdown []byte, relPath string) (string, error)

// Engine builds an ordered Plan from a markdown source tree plus the
// current Confluence state. It does no writes; producing the plan is a
// separate concern from executing it.
type Engine struct {
	profile *transforms.ImportProfile
	locator PageLocator
	lister  ManagedPageLister
	convert Converter
	logger  *log.Logger
}

// Options bundles Engine constructor arguments.
type Options struct {
	Profile *transforms.ImportProfile
	Locator PageLocator
	Lister  ManagedPageLister // optional; nil disables orphan detection
	Convert Converter         // required
	Logger  *log.Logger       // optional; nil silences warnings
}

// New constructs an Engine. Profile, Locator and Convert are required.
func New(opts Options) (*Engine, error) {
	if opts.Profile == nil {
		return nil, fmt.Errorf("sync.New: Profile is required")
	}
	if opts.Locator == nil {
		return nil, fmt.Errorf("sync.New: Locator is required")
	}
	if opts.Convert == nil {
		return nil, fmt.Errorf("sync.New: Convert is required")
	}
	return &Engine{
		profile: opts.Profile,
		locator: opts.Locator,
		lister:  opts.Lister,
		convert: opts.Convert,
		logger:  opts.Logger,
	}, nil
}

// BuildPlan walks fsys (rooted at the sync source — pass os.DirFS(--from))
// and reconciles each markdown file against Confluence, producing an
// ordered Plan. Parents precede children. Orphan actions, if any, appear
// at the end.
//
// rootPageID is informational — it identifies the Confluence root that
// orphan detection scopes to. It is not embedded in actions because the
// executor already knows it from its own configuration.
func (e *Engine) BuildPlan(ctx context.Context, fsys fs.FS, spaceKey string, rootPageID int) (*Plan, error) {
	root, err := e.walk(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("walk source tree: %w", err)
	}

	plan := &Plan{}
	seen := make(map[string]struct{})
	if err := e.visit(ctx, fsys, root, "", spaceKey, plan, seen); err != nil {
		return nil, err
	}

	if e.lister != nil {
		managed, err := e.lister.ListManaged(ctx, spaceKey, rootPageID)
		if err != nil {
			return nil, fmt.Errorf("list managed pages: %w", err)
		}
		for _, mp := range managed {
			if _, ok := seen[mp.IDLabel]; ok {
				continue
			}
			plan.Actions = append(plan.Actions, Action{
				Kind:    ActionOrphan,
				PageID:  mp.PageID,
				Title:   mp.Title,
				IDLabel: mp.IDLabel,
				Reason:  "no source file matches this id label",
			})
			plan.Stats.add(ActionOrphan)
		}
	}

	return plan, nil
}

// dirNode is the intermediate tree the walker produces. It records the
// effective shape after skip/flatten/marker rules are applied, so visit()
// can emit actions without re-checking the profile.
type dirNode struct {
	path    string     // sync-relative, forward slashes; "." for root
	flatten bool       // tree.flatten matched this directory
	marker  string     // relative path of the folder-marker MD file, or ""
	files   []string   // non-marker MD files (rel paths), sorted
	subdirs []*dirNode // sorted by path
}

// walk produces the dirNode tree for fsys rooted at root, applying the
// import profile's skip and flatten rules. Skip is transitive: a matched
// folder is not descended.
func (e *Engine) walk(fsys fs.FS, root string) (*dirNode, error) {
	return e.walkDir(fsys, root)
}

// Discover returns every markdown file path the engine would process for
// the given profile and source tree, in parent-before-child order. It is
// the same traversal BuildPlan performs but stops short of any API calls,
// so callers (e.g. the sync command) can pre-build a path→title map for
// the forward-link rewriter before the converter pipeline is wired in.
func Discover(profile *transforms.ImportProfile, fsys fs.FS) ([]string, error) {
	if profile == nil {
		return nil, fmt.Errorf("Discover: profile is required")
	}
	e := &Engine{profile: profile}
	root, err := e.walk(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []string
	collectFiles(root, &out)
	return out, nil
}

func collectFiles(node *dirNode, out *[]string) {
	if !node.flatten && node.marker != "" {
		*out = append(*out, node.marker)
	}
	*out = append(*out, node.files...)
	for _, sub := range node.subdirs {
		collectFiles(sub, out)
	}
}

func (e *Engine) walkDir(fsys fs.FS, dirPath string) (*dirNode, error) {
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dirPath, err)
	}

	node := &dirNode{path: dirPath}
	if dirPath != "." && e.profile.MatchesFlatten(dirPath) {
		node.flatten = true
	}

	markerName := folderMarkerName(e.profile.Tree.FolderPage, dirPath)

	for _, entry := range entries {
		entryPath := joinPath(dirPath, entry.Name())

		if e.profile.MatchesSkip(entryPath) {
			continue
		}

		if entry.IsDir() {
			child, err := e.walkDir(fsys, entryPath)
			if err != nil {
				return nil, err
			}
			node.subdirs = append(node.subdirs, child)
			continue
		}

		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}

		// Per-path override skip wins over inclusion.
		if o := e.profile.FindOverride(entryPath); o != nil && o.Skip {
			continue
		}

		if !node.flatten && markerName != "" && strings.ToLower(entry.Name()) == markerName {
			if node.marker == "" {
				node.marker = entryPath
				continue
			}
			// Two markers in one directory shouldn't happen on case-sensitive
			// filesystems; on case-insensitive ones ReadDir would already
			// have collapsed them. Keep first, warn on duplicates.
			if e.logger != nil {
				e.logger.Printf("walk: duplicate folder-marker in %q, ignoring %q", dirPath, entryPath)
			}
			continue
		}

		node.files = append(node.files, entryPath)
	}

	sort.Strings(node.files)
	sort.Slice(node.subdirs, func(i, j int) bool {
		return node.subdirs[i].path < node.subdirs[j].path
	})
	return node, nil
}

// visit emits actions for node in parent-before-child order, recording
// each successfully-classified file's id label in seen for orphan
// detection. parentRel is the sync-relative path of the marker file that
// is the effective parent for files directly inside this folder (empty
// when the effective parent is the sync root page).
func (e *Engine) visit(
	ctx context.Context,
	fsys fs.FS,
	node *dirNode,
	parentRel string,
	spaceKey string,
	plan *Plan,
	seen map[string]struct{},
) error {
	effectiveParent := parentRel
	if !node.flatten && node.marker != "" {
		if err := e.emitFile(ctx, fsys, node.marker, parentRel, spaceKey, plan, seen); err != nil {
			return err
		}
		effectiveParent = node.marker
	}
	for _, f := range node.files {
		if err := e.emitFile(ctx, fsys, f, effectiveParent, spaceKey, plan, seen); err != nil {
			return err
		}
	}
	for _, sub := range node.subdirs {
		if err := e.visit(ctx, fsys, sub, effectiveParent, spaceKey, plan, seen); err != nil {
			return err
		}
	}
	return nil
}

// emitFile builds the Action for a single markdown file. It reads the
// file, converts to storage, computes the new hash, asks the locator
// whether the page already exists, and classifies the action.
func (e *Engine) emitFile(
	ctx context.Context,
	fsys fs.FS,
	relPath string,
	parentRel string,
	spaceKey string,
	plan *Plan,
	seen map[string]struct{},
) error {
	src, err := fs.ReadFile(fsys, relPath)
	if err != nil {
		return fmt.Errorf("read %q: %w", relPath, err)
	}

	storage, err := e.convert(ctx, src, relPath)
	if err != nil {
		return fmt.Errorf("convert %q: %w", relPath, err)
	}

	title := e.profile.TitleFor(relPath)
	idLabel := identity.BuildIDLabel(relPath)
	newHash := identity.BuildHashLabel(title, storage)
	seen[idLabel] = struct{}{}

	existing, err := e.locator.FindByPath(ctx, spaceKey, relPath)
	if err != nil {
		return fmt.Errorf("locate %q: %w", relPath, err)
	}

	if existing == nil {
		plan.Actions = append(plan.Actions, Action{
			Kind:          ActionCreate,
			RelPath:       relPath,
			ParentRelPath: parentRel,
			Title:         title,
			IDLabel:       idLabel,
			NewHashLabel:  newHash,
			Storage:       storage,
			Reason:        "no page with this id label",
		})
		plan.Stats.add(ActionCreate)
		return nil
	}

	pageID, ok := existing.ID.Int()
	if !ok {
		return fmt.Errorf("locate %q: page ID %v is not an integer", relPath, existing.ID)
	}
	oldHash := identity.ExtractHashLabel(existing.Labels)

	if oldHash == newHash {
		plan.Actions = append(plan.Actions, Action{
			Kind:         ActionSkip,
			RelPath:      relPath,
			Title:        existing.Title,
			PageID:       pageID,
			IDLabel:      idLabel,
			OldHashLabel: oldHash,
			Reason:       "hash matches existing page",
		})
		plan.Stats.add(ActionSkip)
		return nil
	}

	plan.Actions = append(plan.Actions, Action{
		Kind:          ActionUpdate,
		RelPath:       relPath,
		ParentRelPath: parentRel,
		Title:         title,
		PageID:        pageID,
		Version:       existing.Version.Number,
		IDLabel:       idLabel,
		OldHashLabel:  oldHash,
		NewHashLabel:  newHash,
		Storage:       storage,
		Reason:        updateReason(oldHash),
	})
	plan.Stats.add(ActionUpdate)
	return nil
}

func updateReason(oldHash string) string {
	if oldHash == "" {
		return "no hash label on existing page"
	}
	return "content hash differs from existing page"
}

func joinPath(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

// folderMarkerName resolves the import-profile FolderPage template against a
// directory path, returning the (lowercased) filename the walker treats as
// the folder's marker. Supports the {dir} placeholder so profiles can use
// the per-folder convention "{dir}.md" alongside the simple "README.md".
//
// The sync root (".") has no folder name to substitute — a template using
// {dir} yields "" there, which simply means the root directory has no
// marker (its children attach to rootPageID).
func folderMarkerName(template, dirPath string) string {
	if template == "" {
		return ""
	}
	if !strings.Contains(template, "{dir}") {
		return strings.ToLower(template)
	}
	if dirPath == "." || dirPath == "" {
		return ""
	}
	dirName := path.Base(dirPath)
	return strings.ToLower(strings.ReplaceAll(template, "{dir}", dirName))
}
