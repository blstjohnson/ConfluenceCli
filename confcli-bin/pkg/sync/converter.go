package sync

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"path"
	"regexp"
	"strings"

	"confcli/pkg/converters/md"
	"confcli/pkg/transforms"
)

// BuildPathMap walks fsys with the profile and returns a path→PageRef map
// keyed by sync-root-relative slash paths. It is the first pass of the
// two-pass sync (forward-link rewriter needs every page's title up front,
// before the converter runs on the first file).
//
// Titles are derived deterministically from filenames via the same rule
// the engine uses, so the map matches what BuildPlan will emit.
//
// PageID and SpaceKey are left zero — RewriteMarkdownLinks only uses
// Title for the ac:link macro, and same-space links omit the space key.
func BuildPathMap(profile *transforms.ImportProfile, fsys fs.FS) (map[string]transforms.PageRef, error) {
	files, err := Discover(profile, fsys)
	if err != nil {
		return nil, fmt.Errorf("discover markdown files: %w", err)
	}
	pathMap := make(map[string]transforms.PageRef, len(files))
	for _, f := range files {
		pathMap[f] = transforms.PageRef{Title: titleFor(f)}
	}
	return pathMap, nil
}

// NewMarkdownConverter returns a Converter that runs the forward-link
// rewriter against the markdown source, then feeds the result through
// md.ToStorage. logger receives unresolved-link warnings; pass nil to
// silence them.
//
// pathMap must cover every markdown file in the source tree — use
// BuildPathMap to construct it. A nil map disables rewriting (links pass
// through as literal hrefs that goldmark renders as regular anchors).
//
// Implementation note: the rewriter emits Confluence storage XML
// (<ac:link>...</ac:link>) inline in the markdown source. The colon in
// the tag name disqualifies it as a CommonMark HTML tag, so goldmark
// falls back to autolink-and-escape rules that mangle the XML. To keep
// the rewritten XML intact through goldmark, each <ac:link>...</ac:link>
// chunk is replaced with an HTML-comment placeholder before conversion
// and restored verbatim in the storage output.
func NewMarkdownConverter(pathMap map[string]transforms.PageRef, logger *log.Logger) Converter {
	return func(_ context.Context, source []byte, relPath string) (string, error) {
		ctx := &transforms.TransformContext{
			PreContent: string(source),
			PagePath:   relPath,
		}
		if pathMap != nil {
			rewriter := &transforms.RewriteMarkdownLinks{
				PathMap:        pathMap,
				CurrentPageDir: path.Dir(relPath),
				Logger:         logger,
			}
			if rewriter.CurrentPageDir == "." {
				rewriter.CurrentPageDir = ""
			}
			if err := rewriter.Apply(ctx); err != nil {
				return "", fmt.Errorf("rewrite md links: %w", err)
			}
		}

		stashed, restore := stashAcLinks(ctx.PreContent)

		storage, err := md.ToStorage([]byte(stashed))
		if err != nil {
			return "", fmt.Errorf("markdown to storage: %w", err)
		}
		return restore(storage), nil
	}
}

// acLinkRe matches a complete <ac:link>...</ac:link> block (non-greedy).
// The forward-link rewriter is currently the only producer; if other
// transforms start emitting similar macros, extend this list.
var acLinkRe = regexp.MustCompile(`<ac:link[\s\S]*?</ac:link>`)

// stashAcLinks replaces every <ac:link>...</ac:link> in src with an
// HTML-comment placeholder that goldmark passes through unchanged, and
// returns a restore function that swaps the originals back into the
// converted output. Placeholders use a unique sentinel so accidental
// markdown-side comments cannot collide.
func stashAcLinks(src string) (string, func(string) string) {
	originals := []string{}
	stashed := acLinkRe.ReplaceAllStringFunc(src, func(match string) string {
		placeholder := fmt.Sprintf("<!--confcli-aclink-%d-->", len(originals))
		originals = append(originals, match)
		return placeholder
	})
	if len(originals) == 0 {
		return stashed, func(s string) string { return s }
	}
	return stashed, func(rendered string) string {
		for i, orig := range originals {
			ph := fmt.Sprintf("<!--confcli-aclink-%d-->", i)
			rendered = strings.Replace(rendered, ph, orig, 1)
		}
		return rendered
	}
}
