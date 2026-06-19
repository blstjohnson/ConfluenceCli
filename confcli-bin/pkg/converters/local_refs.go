package converters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// idPrefixRe matches exported content filenames that begin with a numeric page
// ID, e.g. "744208425_my-page.md". Group 1 is the page ID.
var idPrefixRe = regexp.MustCompile(`^(\d+)_`)

// refContentExtensions are the file extensions treated as exported page content
// when scanning a reference directory by filename.
var refContentExtensions = []string{".md", ".html", ".storage", ".editor", ".txt"}

func isRefContentExt(ext string) bool {
	ext = strings.ToLower(ext)
	for _, e := range refContentExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

// BuildPageMapFromDirs scans one or more directories of previously-exported
// Confluence pages and returns a map of page ID -> absolute file path. It is
// used to rewrite internal links in a freshly-exported page so they point at the
// local copies of the pages they reference.
//
// Two sources are consulted:
//  1. Content files whose name begins with "<pageID>_" — the default export
//     naming (e.g. "744208425_title.md").
//  2. "*.meta.json" sidecars written by `hierarchy space --save-metadata`, which
//     carry the authoritative page "id". These also cover --clean-names exports
//     whose content files omit the numeric prefix.
//
// meta.json sidecars take precedence over filename inference, and later
// directories take precedence over earlier ones on ID collision.
func BuildPageMapFromDirs(dirs []string) (map[int]string, error) {
	result := make(map[int]string)
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		if err := scanRefsDir(dir, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func scanRefsDir(dir string, out map[int]string) error {
	var metaFiles []string

	// First pass: infer IDs from the "<id>_" filename prefix.
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		if strings.HasSuffix(name, ".meta.json") {
			metaFiles = append(metaFiles, path)
			return nil
		}
		if !isRefContentExt(filepath.Ext(name)) {
			return nil
		}
		if m := idPrefixRe.FindStringSubmatch(name); len(m) == 2 {
			if id, convErr := strconv.Atoi(m[1]); convErr == nil {
				if abs, absErr := filepath.Abs(path); absErr == nil {
					out[id] = abs
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Second pass: meta.json sidecars override filename inference.
	for _, mp := range metaFiles {
		id, content, ok := resolveMetaSidecar(mp)
		if !ok {
			continue
		}
		if abs, absErr := filepath.Abs(content); absErr == nil {
			out[id] = abs
		}
	}
	return nil
}

// resolveMetaSidecar reads a "*.meta.json" file, extracts its page "id", and
// locates the adjacent content file (same base name, sans ".meta.json").
// Returns (id, contentPath, ok).
func resolveMetaSidecar(metaPath string) (int, string, bool) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return 0, "", false
	}

	var meta struct {
		ID json.Number `json:"id"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if decErr := dec.Decode(&meta); decErr != nil {
		return 0, "", false
	}
	id, err := strconv.Atoi(meta.ID.String())
	if err != nil || id == 0 {
		return 0, "", false
	}

	base := strings.TrimSuffix(filepath.Base(metaPath), ".meta.json")
	dir := filepath.Dir(metaPath)
	for _, ext := range refContentExtensions {
		candidate := filepath.Join(dir, base+ext)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return id, candidate, true
		}
	}
	return id, "", false
}
