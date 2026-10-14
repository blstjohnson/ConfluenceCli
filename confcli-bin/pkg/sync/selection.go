package sync

import (
	"fmt"
	"io/fs"
	"strings"
)

// NormalizeFileSelection accepts one exact source path relative to --from.
// Backslashes are accepted for Windows users; traversal and absolute paths
// are rejected before planning or making API calls.
func NormalizeFileSelection(value string) (string, error) {
	value = strings.ReplaceAll(value, `\`, "/")
	value = strings.TrimPrefix(value, "./")
	if value == "." || !fs.ValidPath(value) || strings.ContainsAny(value, ":*?[]") ||
		!strings.HasSuffix(strings.ToLower(value), ".md") {
		return "", fmt.Errorf("--file must be an exact Markdown path relative to --from, without traversal or wildcards: %q", value)
	}
	return value, nil
}

// SelectFile limits writes in a fully resolved plan to one source file.
// The full tree must still be used to build the converter's link map and
// locate parents. Ancestors are retained as read-only skip actions even if
// their own content differs. A missing parent must be synced separately;
// this mode never creates or updates another page implicitly.
func SelectFile(full *Plan, value string) (*Plan, error) {
	relPath, err := NormalizeFileSelection(value)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]Action)
	for _, action := range full.Actions {
		if action.RelPath != "" {
			byPath[action.RelPath] = action
		}
	}
	selected, ok := byPath[relPath]
	if !ok {
		return nil, fmt.Errorf("--file %q is not an included Markdown source; check --from and profile skip rules", relPath)
	}
	ancestors := map[string]bool{}
	for parent := selected.ParentRelPath; parent != ""; {
		if parent == relPath || ancestors[parent] {
			return nil, fmt.Errorf("--file %q: cyclic parent mapping at %q", relPath, parent)
		}
		action, exists := byPath[parent]
		if !exists || action.PageID == 0 || action.Kind == ActionCreate {
			return nil, fmt.Errorf("--file %q: parent %q does not exist in Confluence; sync that parent separately first", relPath, parent)
		}
		ancestors[parent] = true
		parent = action.ParentRelPath
	}
	plan := &Plan{}
	for _, action := range full.Actions {
		switch {
		case action.RelPath == relPath:
			plan.Actions = append(plan.Actions, action)
			plan.Stats.add(action.Kind)
			plan.Stats.Images += len(action.Images)
		case ancestors[action.RelPath]:
			plan.Actions = append(plan.Actions, Action{
				Kind: ActionSkip, RelPath: action.RelPath, ParentRelPath: action.ParentRelPath,
				Title: action.Title, PageID: action.PageID, IDLabel: action.IDLabel,
				OldHashLabel: action.OldHashLabel, Reason: "ancestor resolved read-only for --file",
			})
			plan.Stats.add(ActionSkip)
		}
	}
	return plan, nil
}

// SelectFiles keeps an explicit set of exact files and their read-only ancestors.
// Original plan order is retained, so the executor still writes one page at a time.
func SelectFiles(full *Plan, values []string) (*Plan, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("file selection is empty")
	}
	selected := map[string]bool{}
	ancestors := map[string]Action{}
	ids := map[int]string{}
	for _, value := range values {
		name, err := NormalizeFileSelection(value)
		if err != nil {
			return nil, err
		}
		if selected[name] {
			return nil, fmt.Errorf("duplicate selected path %q", name)
		}
		one, err := SelectFile(full, name)
		if err != nil {
			return nil, err
		}
		selected[name] = true
		for _, action := range one.Actions {
			if action.RelPath == name {
				if action.PageID != 0 {
					if other := ids[action.PageID]; other != "" {
						return nil, fmt.Errorf("selected paths %q and %q map to page %d", other, name, action.PageID)
					}
					ids[action.PageID] = name
				}
			} else {
				ancestors[action.RelPath] = action
			}
		}
	}
	plan := &Plan{}
	for _, action := range full.Actions {
		if selected[action.RelPath] {
			plan.Actions = append(plan.Actions, action)
			plan.Stats.add(action.Kind)
			plan.Stats.Images += len(action.Images)
		} else if ancestor, ok := ancestors[action.RelPath]; ok {
			plan.Actions = append(plan.Actions, ancestor)
			plan.Stats.add(ActionSkip)
		}
	}
	return plan, nil
}
