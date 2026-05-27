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
		pathMap[f] = transforms.PageRef{Title: profile.TitleFor(f)}
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
func NewMarkdownConverter(pathMap map[string]transforms.PageRef, plantuml *transforms.RewritePlantUMLLinks, gitFiles *transforms.RewriteGitFileLinks, logger *log.Logger) Converter {
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

		if plantuml != nil {
			// Inject the per-call logger so the rewriter's "escapes repo
			// root" warnings land in the sync command's stderr.
			if plantuml.Logger == nil {
				plantuml.Logger = logger
			}
			if err := plantuml.Apply(ctx); err != nil {
				return "", fmt.Errorf("rewrite plantuml links: %w", err)
			}
		}

		// Git-files rewriter runs last so the .md and .puml handlers
		// claim their links first; this one catches whatever's left
		// (yaml, json, sql, …) that would otherwise become a broken
		// relative href on the rendered Confluence page.
		if gitFiles != nil {
			if gitFiles.Logger == nil {
				gitFiles.Logger = logger
			}
			if err := gitFiles.Apply(ctx); err != nil {
				return "", fmt.Errorf("rewrite git-file links: %w", err)
			}
		}

		stashed, restore := stashAcLinks(ctx.PreContent)

		storage, err := md.ToStorage([]byte(stashed))
		if err != nil {
			return "", fmt.Errorf("markdown to storage: %w", err)
		}
		out := restore(storage)
		out = selfCloseVoidTags(out)
		out = escapeUnknownTags(out)
		if strings.TrimSpace(out) == "" {
			// Confluence rejects empty page content. Folder-marker files
			// authored as empty placeholders are legitimate; substitute a
			// minimal paragraph so they create as parent stubs.
			out = "<p />"
		}
		return out, nil
	}
}

// knownHTMLTags is the allow-list for escapeUnknownTags. Any bare-form
// tag <name> or </name> (no attributes, no namespace) whose name is not
// in this set is escaped to &lt;name&gt;. The list covers HTML5; the
// rare cases this misses (custom web components etc.) can be worked
// around by adding attributes (which suppress the rewrite).
var knownHTMLTags = map[string]struct{}{
	"a": {}, "abbr": {}, "address": {}, "area": {}, "article": {},
	"aside": {}, "audio": {}, "b": {}, "base": {}, "bdi": {}, "bdo": {},
	"blockquote": {}, "body": {}, "br": {}, "button": {}, "canvas": {},
	"caption": {}, "cite": {}, "code": {}, "col": {}, "colgroup": {},
	"data": {}, "datalist": {}, "dd": {}, "del": {}, "details": {},
	"dfn": {}, "dialog": {}, "div": {}, "dl": {}, "dt": {}, "em": {},
	"embed": {}, "fieldset": {}, "figcaption": {}, "figure": {},
	"footer": {}, "form": {}, "h1": {}, "h2": {}, "h3": {}, "h4": {},
	"h5": {}, "h6": {}, "head": {}, "header": {}, "hgroup": {}, "hr": {},
	"html": {}, "i": {}, "iframe": {}, "img": {}, "input": {}, "ins": {},
	"kbd": {}, "label": {}, "legend": {}, "li": {}, "link": {}, "main": {},
	"map": {}, "mark": {}, "menu": {}, "meta": {}, "meter": {}, "nav": {},
	"noscript": {}, "object": {}, "ol": {}, "optgroup": {}, "option": {},
	"output": {}, "p": {}, "param": {}, "picture": {}, "pre": {},
	"progress": {}, "q": {}, "rb": {}, "rp": {}, "rt": {}, "rtc": {},
	"ruby": {}, "s": {}, "samp": {}, "script": {}, "section": {},
	"select": {}, "slot": {}, "small": {}, "source": {}, "span": {},
	"strong": {}, "style": {}, "sub": {}, "summary": {}, "sup": {},
	"table": {}, "tbody": {}, "td": {}, "template": {}, "textarea": {},
	"tfoot": {}, "th": {}, "thead": {}, "time": {}, "title": {}, "tr": {},
	"track": {}, "u": {}, "ul": {}, "var": {}, "video": {}, "wbr": {},
}

// bareTagRe matches a simple open or close tag with no attributes and
// no namespace (so <ac:link> and <a href="...">  are skipped). Group 1
// is the slash for close tags; group 2 is the tag name.
var bareTagRe = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9]*)>`)

// escapeUnknownTags rewrites <FooBar>-style placeholders that user-
// authored markdown sometimes contains (e.g. "<FIWalletId>" as a field-
// name placeholder in a table cell). Goldmark's WithUnsafe renderer
// passes those through as raw HTML, then Confluence's strict XHTML
// parser rejects them with "expected </FIWalletId>".
//
// Scope: matches only bare <name> / </name> with no attributes; any
// real HTML usually has attributes or is in knownHTMLTags. Skips
// content inside CDATA sections so code samples are untouched.
func escapeUnknownTags(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	replace := func(in string) string {
		return bareTagRe.ReplaceAllStringFunc(in, func(match string) string {
			sub := bareTagRe.FindStringSubmatch(match)
			// Case-sensitive lookup: XHTML tag names are lowercase, and
			// real HTML users write them that way. Mixed-case forms like
			// <Object> or <Data> are placeholder text the user meant
			// literally — escape those even though "object"/"data" are
			// valid HTML names in lowercase.
			if _, ok := knownHTMLTags[sub[2]]; ok {
				return match
			}
			return "&lt;" + sub[1] + sub[2] + "&gt;"
		})
	}
	var b strings.Builder
	b.Grow(len(s))
	rest := s
	for {
		start := strings.Index(rest, "<![CDATA[")
		if start < 0 {
			b.WriteString(replace(rest))
			break
		}
		b.WriteString(replace(rest[:start]))
		end := strings.Index(rest[start:], "]]>")
		if end < 0 {
			b.WriteString(rest[start:])
			break
		}
		end += start + len("]]>")
		b.WriteString(rest[start:end])
		rest = rest[end:]
	}
	return b.String()
}

// voidTagRe matches the start of an HTML5 void tag (br, hr, img, input,
// area, base, col, embed, link, meta, param, source, track, wbr) that
// isn't already self-closed. We rewrite the trailing > to /> to make the
// output XHTML-strict for Confluence's storage-format parser.
var voidTagRe = regexp.MustCompile(`<(br|hr|img|input|area|base|col|embed|link|meta|param|source|track|wbr)((?:\s+[^>/]*)?)\s*>`)

// selfCloseVoidTags rewrites HTML5 void tags in s to XHTML self-closing
// form, but only outside <![CDATA[...]]> sections (code-macro bodies
// contain user-supplied text that must pass through verbatim).
func selfCloseVoidTags(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	rest := s
	for {
		start := strings.Index(rest, "<![CDATA[")
		if start < 0 {
			b.WriteString(voidTagRe.ReplaceAllString(rest, "<$1$2 />"))
			break
		}
		b.WriteString(voidTagRe.ReplaceAllString(rest[:start], "<$1$2 />"))
		end := strings.Index(rest[start:], "]]>")
		if end < 0 {
			// Unterminated CDATA — copy the rest verbatim.
			b.WriteString(rest[start:])
			break
		}
		end += start + len("]]>")
		b.WriteString(rest[start:end])
		rest = rest[end:]
	}
	return b.String()
}

// opaqueXMLRes matches Confluence storage-format XML blocks that
// rewriters inject into the markdown source. They contain colons in
// the tag name, which disqualifies them as CommonMark HTML tags —
// goldmark would autolink-and-escape them. The stash swaps each block
// for an HTML-comment placeholder that goldmark passes through, then
// restores the original verbatim in the storage output.
//
// Add to this list any new rewriter that emits Confluence-namespaced
// tags into PreContent.
var opaqueXMLRes = []*regexp.Regexp{
	regexp.MustCompile(`<ac:link[\s\S]*?</ac:link>`),
	regexp.MustCompile(`<ac:structured-macro[\s\S]*?</ac:structured-macro>`),
}

// stashAcLinks replaces every opaque Confluence XML block in src with
// an HTML-comment placeholder; returns a restore function that swaps
// the originals back into the converted output.
func stashAcLinks(src string) (string, func(string) string) {
	originals := []string{}
	stashed := src
	for _, re := range opaqueXMLRes {
		stashed = re.ReplaceAllStringFunc(stashed, func(match string) string {
			placeholder := fmt.Sprintf("<!--confcli-xml-%d-->", len(originals))
			originals = append(originals, match)
			return placeholder
		})
	}
	if len(originals) == 0 {
		return stashed, func(s string) string { return s }
	}
	return stashed, func(rendered string) string {
		for i, orig := range originals {
			ph := fmt.Sprintf("<!--confcli-xml-%d-->", i)
			rendered = strings.Replace(rendered, ph, orig, 1)
		}
		return rendered
	}
}
