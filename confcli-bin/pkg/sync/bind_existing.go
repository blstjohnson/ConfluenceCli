package sync

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"confcli/pkg/models"
	"confcli/pkg/sync/identity"
	"confcli/pkg/transforms"
	"confcli/pkg/utils"
)

// ExistingChildLocator finds a direct child using the same filename rules as
// hierarchy --clean-names. It must reject ambiguous matches and include managed
// matches so the caller can detect another source's conflicting identity label.
type ExistingChildLocator interface {
	FindExistingChild(context.Context, string, int, string, bool) (*models.Page, error)
}

// BindExisting resolves names and parents before the converter's full-tree link
// map is built. Overrides are added only to a copy in memory; source files and
// profile YAML are never edited. Selected ancestors are resolved but not written.
func BindExisting(ctx context.Context, profile *transforms.ImportProfile, fsys fs.FS, locator PageLocator, space string, rootID int, selected []string) (*transforms.ImportProfile, error) {
	children, ok := locator.(ExistingChildLocator)
	if !ok {
		return nil, fmt.Errorf("--update-existing requires a locator supporting existing children")
	}
	if profile == nil || rootID <= 0 {
		return nil, fmt.Errorf("--update-existing requires a profile and a positive root ID")
	}
	e := &Engine{profile: profile, locator: locator, rootID: rootID}
	root, err := e.walk(fsys, ".")
	if err != nil {
		return nil, err
	}
	type source struct{ file, parent string }
	var sources []source
	parents := map[string]string{}
	var collect func(*dirNode, string)
	collect = func(node *dirNode, parent string) {
		if !node.flatten && node.marker != "" {
			sources = append(sources, source{node.marker, parent})
			parents[node.marker] = parent
			parent = node.marker
		}
		for _, file := range node.files {
			sources = append(sources, source{file, parent})
			parents[file] = parent
		}
		for _, child := range node.subdirs {
			collect(child, parent)
		}
	}
	collect(root, "")
	required := map[string]bool{}
	for _, file := range selected {
		file, err = NormalizeFileSelection(file)
		if err != nil {
			return nil, err
		}
		if _, included := parents[file]; !included {
			return nil, fmt.Errorf("--file %q is not an included Markdown source; check --from and profile skip rules", file)
		}
		for p := file; p != ""; p = parents[p] {
			required[p] = true
		}
	}
	resolved := map[string]int{"": rootID}
	claimed := map[int]string{}
	var overrides []transforms.PathOverride
	for _, src := range sources {
		strict := len(selected) == 0 || required[src.file]
		title := profile.TitleFor(src.file)
		override := profile.FindOverride(src.file)
		explicitTitle := override != nil && override.Title != ""
		var page *models.Page
		var lookupErr error
		if override != nil && override.PageID != 0 {
			// Explicit IDs retain their existing validation and title semantics.
			page, lookupErr = e.findExisting(ctx, space, src.file, title)
		} else {
			page, lookupErr = locator.FindByPath(ctx, space, src.file)
			if lookupErr == nil && page == nil && resolved[src.parent] != 0 {
				page, lookupErr = children.FindExistingChild(ctx, space, resolved[src.parent], title, explicitTitle)
			}
			if lookupErr == nil && page != nil {
				if !existingTitleMatches(page.Title, title, explicitTitle) {
					lookupErr = fmt.Errorf("existing title %q differs from planned title %q; set overrides.title", page.Title, title)
				}
				for _, label := range page.Labels {
					if strings.HasPrefix(label.Name, identity.IDLabelPrefix) && label.Name != identity.BuildIDLabel(src.file) {
						lookupErr = fmt.Errorf("page %v (%q) is already bound to another source (%s)", page.ID, page.Title, label.Name)
					}
				}
			}
		}
		if lookupErr != nil {
			if strict {
				return nil, fmt.Errorf("--update-existing: locate %q: %w", src.file, lookupErr)
			}
			// Unselected siblings need only a best-effort title for forward links.
			// They cannot prevent a selected file from being published.
			continue
		}
		if page == nil {
			continue // A missing parent is never guessed from another branch.
		}
		id, valid := page.ID.Int()
		if !valid || id <= 0 {
			return nil, fmt.Errorf("--update-existing: invalid page ID for %q", src.file)
		}
		if strict {
			if other := claimed[id]; other != "" {
				return nil, fmt.Errorf("--update-existing: %q and %q both resolve to page %d", other, src.file, id)
			}
			claimed[id] = src.file
		}
		resolved[src.file] = id
		overrides = append(overrides, transforms.PathOverride{Path: exactPathGlob(src.file), PageID: id, Title: page.Title})
	}
	boundProfile := *profile
	boundProfile.Overrides = append(overrides, profile.Overrides...)
	return &boundProfile, nil
}

func exactPathGlob(file string) string {
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "?", "\\?", "[", "\\[", "]", "\\]").Replace(file)
}

func existingTitleMatches(actual, planned string, exact bool) bool {
	if exact {
		return actual == planned
	}
	want := identity.NormalizePath(utils.SanitizeFilenameSimpleNoLimit(planned))
	return want == identity.NormalizePath(utils.SanitizeFilenameSimpleNoLimit(actual)) ||
		want == identity.NormalizePath(utils.SanitizeFilenameSimple(actual))
}
