package sync

import (
	"context"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/url"
	"path"
	"regexp"
	"strings"

	"confcli/pkg/converters"
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
func NewMarkdownConverter(pathMap map[string]transforms.PageRef, plantuml *transforms.RewritePlantUMLLinks, gitFiles *transforms.RewriteGitFileLinks, logger *log.Logger, includePage ...bool) Converter {
	include := len(includePage) == 0 || includePage[0]
	return newMarkdownConverter(pathMap, plantuml, gitFiles, logger, include, nil)
}

func newMarkdownConverter(pathMap map[string]transforms.PageRef, plantuml *transforms.RewritePlantUMLLinks, gitFiles *transforms.RewriteGitFileLinks, logger *log.Logger, include bool, attachmentPaths []string) Converter {
	return func(_ context.Context, source []byte, relPath string) (string, error) {
		ctx := &transforms.TransformContext{
			PreContent: string(source),
			PagePath:   relPath,
		}
		if pathMap != nil {
			rewriter := &transforms.RewriteMarkdownLinks{
				DisableIncludes: !include,
				AttachmentPaths: attachmentPaths,
				PathMap:         pathMap,
				CurrentPageDir:  path.Dir(relPath),
				Logger:          logger,
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
		out = escapeStrayMarkup(out)
		out = escapeUnpairedCodeTags(out)
		if strings.TrimSpace(out) == "" {
			// Confluence rejects empty page content. Folder-marker files
			// authored as empty placeholders are legitimate; substitute a
			// minimal paragraph so they create as parent stubs.
			out = "<p />"
		}
		return out, nil
	}
}

// A literal placeholder such as <code> can arrive from exported text as an
// unclosed HTML tag. Keep it visible instead of submitting malformed XHTML.
func escapeUnpairedCodeTags(storage string) string {
	paragraphs := regexp.MustCompile(`(?is)<p\b[^>]*>.*?</p>`)
	tags := regexp.MustCompile(`(?i)</?code\s*>`)
	return mapOutsideCDATA(storage, func(segment string) string {
		return paragraphs.ReplaceAllStringFunc(segment, func(paragraph string) string {
			positions := tags.FindAllStringIndex(paragraph, -1)
			stack := []int{}
			unpaired := map[int]bool{}
			for i, span := range positions {
				if strings.HasPrefix(paragraph[span[0]:span[1]], "</") {
					if len(stack) == 0 {
						unpaired[i] = true
					} else {
						stack = stack[:len(stack)-1]
					}
				} else {
					stack = append(stack, i)
				}
			}
			for _, i := range stack {
				unpaired[i] = true
			}
			for i := len(positions) - 1; i >= 0; i-- {
				if unpaired[i] {
					span := positions[i]
					paragraph = paragraph[:span[0]] + html.EscapeString(paragraph[span[0]:span[1]]) + paragraph[span[1]:]
				}
			}
			return paragraph
		})
	})
}

// NewProfileMarkdownConverter runs the import profile around the standard
// converter. The same context spans both phases, so macro boundary placeholders
// can be restored after Markdown (including nested page embeds) is rendered.
func NewProfileMarkdownConverter(profile *transforms.ImportProfile, pathMap map[string]transforms.PageRef, plantuml *transforms.RewritePlantUMLLinks, gitFiles *transforms.RewriteGitFileLinks, logger *log.Logger) (Converter, error) {
	if profile == nil {
		return nil, fmt.Errorf("import profile is required")
	}
	pipeline, err := transforms.BuildPipeline(profile.Transforms, transforms.DefaultRegistry())
	if err != nil {
		return nil, err
	}
	include := profile.Tree.IncludePageMacro == nil || *profile.Tree.IncludePageMacro
	convert := newMarkdownConverter(pathMap, plantuml, gitFiles, logger, include, profile.Attachments.Paths)
	return func(ctx context.Context, source []byte, relPath string) (string, error) {
		prepared, restoreTransclusions, err := converters.RestoreTransclusions(string(source))
		if err != nil {
			return "", fmt.Errorf("%s: %w", relPath, err)
		}
		tctx := &transforms.TransformContext{PreContent: prepared, PagePath: relPath, Format: "storage"}
		if err := pipeline.Run(tctx); err != nil {
			return "", err
		}
		storage, err := convert(ctx, []byte(tctx.PreContent), relPath)
		if err != nil {
			return "", err
		}
		tctx.PostContent = storage
		if err := pipeline.Run(tctx); err != nil {
			return "", err
		}
		return restoreAnchorElements(restoreTransclusions(tctx.PostContent)), nil
	}, nil
}

var anchorElementRe = regexp.MustCompile(`<a\s+(?:id|name)="([^"]*)"\s*></a>`)
var localAnchorLinkRe = regexp.MustCompile(`(?s)<a href="#([^"]*)"[^>]*>(.*?)</a>`)

func restoreAnchorElements(storage string) string {
	rewrite := func(s string) string {
		s = anchorElementRe.ReplaceAllString(s, `<ac:structured-macro ac:name="anchor"><ac:parameter ac:name="">$1</ac:parameter></ac:structured-macro>`)
		return localAnchorLinkRe.ReplaceAllStringFunc(s, func(link string) string {
			m := localAnchorLinkRe.FindStringSubmatch(link)
			anchor := html.UnescapeString(m[1])
			if decoded, err := url.PathUnescape(anchor); err == nil {
				anchor = decoded
			}
			return `<ac:link ac:anchor="` + html.EscapeString(anchor) + `"><ac:link-body>` + m[2] + `</ac:link-body></ac:link>`
		})
	}
	var b strings.Builder
	for {
		start := strings.Index(storage, "<![CDATA[")
		if start < 0 {
			b.WriteString(rewrite(storage))
			break
		}
		b.WriteString(rewrite(storage[:start]))
		end := strings.Index(storage[start:], "]]>")
		if end < 0 {
			b.WriteString(storage[start:])
			break
		}
		end += start + 3
		b.WriteString(storage[start:end])
		storage = storage[end:]
	}
	return b.String()
}

// knownHTMLTags is the allow-list for escapeUnknownTags. Any tag-shaped
// token <name…> / </name> whose name is not in this set and is not
// Confluence-namespaced (no ':') is escaped to &lt;…&gt;. The list covers
// HTML5; custom web components (hyphenated names) are intentionally not
// allowed — they don't appear in Confluence docs and collide with the
// literal <base-dn>-style placeholders this guard exists to catch.
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

// tagLikeRe matches anything shaped like an open, close, or self-closing
// tag: <name…>, </name>, <name … />. Group 1 is the leading slash (close
// tags); group 2 is the tag name, which may carry a namespace colon
// (ac:link), a hyphen (base-dn), or a dot. The attribute span uses a
// quote-aware alternation so a '>' inside a quoted value (<a title="a > b">)
// doesn't end the match early.
var tagLikeRe = regexp.MustCompile(`<(/?)([A-Za-z][\w.:-]*)((?:"[^"]*"|'[^']*'|[^<>"'])*?)\s*(/?)>`)

// escapeUnknownTags rewrites tag-shaped text that user-authored markdown
// contains literally but Confluence's strict XHTML parser would reject.
// Goldmark's WithUnsafe renderer passes raw HTML through verbatim, so
// these reach the storage output unescaped. Three classes show up in
// real docs and each crashes the create/update call:
//
//   - bare placeholders:     <FIWalletId>, <Object>
//   - hyphenated names:      <base-dn>, <digest-value>, <Y-X>, <YYYY-MM-DD>
//     (valid HTML5 custom-element names, so goldmark keeps them)
//   - bare-attribute forms:  <TCP Port>, <xrate service url>
//     (valid HTML5 boolean attributes; Confluence wants attr="value")
//
// Rule: escape any tag whose name is neither a known HTML5 element nor a
// Confluence-namespaced macro (name containing ':', e.g. ac:link / ri:page).
// Real HTML the user mixes in (<em>, <td colspan="2">) is in knownHTMLTags
// and survives untouched. Anything else is literal text that would fail
// the page anyway, so escaping it is strictly safer. Skips content inside
// CDATA sections so code samples are untouched.
func escapeUnknownTags(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	replace := func(in string) string {
		return tagLikeRe.ReplaceAllStringFunc(in, func(match string) string {
			sub := tagLikeRe.FindStringSubmatch(match)
			name := sub[2]
			// Confluence storage macros (<ac:…>, <ri:…>) are namespaced;
			// the colon is the marker. They're emitted by the rewriters
			// and must pass through verbatim.
			if strings.Contains(name, ":") {
				return match
			}
			// Case-sensitive lookup: XHTML tag names are lowercase, and
			// real HTML users write them that way. Mixed-case forms like
			// <Object> or <Data> are placeholder text the user meant
			// literally — escape those even though "object"/"data" are
			// valid HTML names in lowercase.
			if _, ok := knownHTMLTags[name]; ok && validXHTMLTagSyntax.MatchString(match) {
				return match
			}
			esc := strings.ReplaceAll(match, "<", "&lt;")
			return strings.ReplaceAll(esc, ">", "&gt;")
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

// validEntityRe matches a well-formed XML/HTML entity reference at the
// start of the string: numeric (&#160; / &#xA0;) or named (&amp;). A '&'
// that doesn't begin one is stray text and must be escaped.
var validEntityRe = regexp.MustCompile(`^&(#[0-9]+|#x[0-9a-fA-F]+|[a-zA-Z][a-zA-Z0-9]*);`)

var validXHTMLTagSyntax = regexp.MustCompile(`(?s)^</?[A-Za-z][A-Za-z0-9:-]*(?:\s+[A-Za-z_:][A-Za-z0-9_.:-]*\s*=\s*(?:"[^"]*"|'[^']*'))*\s*/?>$`)

// escapeStrayMarkup is the final XHTML safety net. Outside CDATA it
// escapes a '&' that doesn't start a valid entity reference and a '<'
// that doesn't start a tag, comment, or CDATA section. goldmark already
// escapes both in normal content, so in practice this only fires when
// malformed source — e.g. a code fence closed with text glued on the
// same line (```text) — desynchronizes fence pairing and makes goldmark
// spill raw, unescaped text into the output. Escaping it guarantees the
// page stays well-formed enough for Confluence's storage parser to accept
// instead of rejecting the whole create/update; it never alters markup
// that was already valid.
//
// Runs after escapeUnknownTags, which has already neutralized tag-shaped
// placeholders (<base-dn>, <TCP Port>); what's left for this pass is stray
// punctuation such as "<--", "a < b", and "&title=". A '>' needs no
// escaping — it is legal in XHTML character data.
func escapeStrayMarkup(s string) string {
	if !strings.ContainsAny(s, "<&") {
		return s
	}
	replace := func(in string) string {
		var b strings.Builder
		b.Grow(len(in))
		for i := 0; i < len(in); i++ {
			switch in[i] {
			case '&':
				if validEntityRe.MatchString(in[i:]) {
					b.WriteByte('&')
				} else {
					b.WriteString("&amp;")
				}
			case '<':
				// A '<' begins real markup only when followed by a tag-name
				// start (letter), a close-tag '/', or '!'/'?' (comment,
				// CDATA, processing instruction). Anything else is literal.
				if i+1 < len(in) && isMarkupStart(in[i+1]) {
					b.WriteByte('<')
				} else {
					b.WriteString("&lt;")
				}
			default:
				b.WriteByte(in[i])
			}
		}
		return b.String()
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

// isMarkupStart reports whether b, following a '<', could begin a tag,
// comment, CDATA section, or processing instruction.
func isMarkupStart(b byte) bool {
	return b == '/' || b == '!' || b == '?' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
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
//
// Order matters: structured-macro is stashed before ac:link so a macro that
// *contains* an ac:link (e.g. the "include" macro's <ac:parameter><ac:link>
// …</ac:link></ac:parameter>) is captured whole. If ac:link ran first it
// would stash the inner link separately, and the restore (which replaces in
// index order) would leave the inner placeholder un-restored. This assumes
// macros are not nested within each other, which holds for everything the
// rewriters emit today.
var opaqueXMLRes = []*regexp.Regexp{
	regexp.MustCompile(`<ac:structured-macro[\s\S]*?</ac:structured-macro>`),
	regexp.MustCompile(`<ac:link[\s\S]*?</ac:link>`),
}

// stashAcLinks replaces every opaque Confluence XML block in src with
// an HTML-comment placeholder; returns a restore function that swaps
// the originals back into the converted output.
func stashAcLinks(src string) (string, func(string) string) {
	originals := []string{}
	placeholders := []string{}
	stashed := transforms.MarkdownOutsideCode(src, func(s string) string {
		for _, re := range opaqueXMLRes {
			s = re.ReplaceAllStringFunc(s, func(match string) string {
				placeholder := fmt.Sprintf("<!--confcli-xml-%d-->", len(originals))
				if strings.HasPrefix(match, "<ac:link") {
					// Inline links must stay inline: a comment at the start of
					// a paragraph turns the whole paragraph into a raw HTML block.
					placeholder = fmt.Sprintf(`<span data-confcli-xml="%d"></span>`, len(originals))
				}
				originals = append(originals, match)
				placeholders = append(placeholders, placeholder)
				return placeholder
			})
		}
		return s
	})
	if len(originals) == 0 {
		return stashed, func(s string) string { return s }
	}
	return stashed, func(rendered string) string {
		for i, orig := range originals {
			ph := placeholders[i]
			rendered = strings.Replace(rendered, ph, orig, 1)
		}
		return rendered
	}
}
