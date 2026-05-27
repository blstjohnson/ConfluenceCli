package transforms

import (
	"io/fs"
	"log"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

// RewriteGitFileLinks rewrites markdown links to non-markdown files in
// the repository (e.g. .yaml, .json, .sql, .sh) into Confluence macros
// — typically "view-git-file" without renderpuml, so the file appears
// as source in a panel rather than a broken relative href.
//
// This is the catch-all companion to RewritePlantUMLLinks: that one
// handles .puml/.plantuml diagrams; RewriteMarkdownLinks handles .md
// → page links; this one handles whatever's left in the repo.
//
// Skipped (passed through unchanged):
//   - image embeds: ![alt](href)
//   - external URLs (contain "://"), mailto:, tel:, in-page anchors
//   - .md / .markdown / .puml / .plantuml (handled by other rewriters)
//   - common image extensions (rendering an image as a source panel
//     would be confusing)
//   - links inside fenced code blocks
//
// When Extensions is non-empty, only listed extensions are rewritten;
// otherwise the catch-all default applies.
type RewriteGitFileLinks struct {
	Macro           string
	Parameters      map[string]string
	Branch          string
	SyncRootRelRepo string
	// Extensions optionally limits which extensions trigger a rewrite.
	// Stored as lowercase, leading dot stripped.
	Extensions []string
	Logger     *log.Logger

	// Mode selects the emission style for matched links:
	//   "" or "link"  — wrap the href in the configured view-git-file macro.
	//   "inline"      — read the file via FSys and emit a Confluence "code"
	//                    structured macro with the file body inside CDATA.
	Mode string

	// PerExtension overrides Mode per extension (lowercased, no leading
	// dot). Values: "link" or "inline".
	PerExtension map[string]string

	// InlineMaxBytes caps the file size eligible for inline emission. A
	// non-positive value means no cap. Oversize files fall back to link
	// mode with a warning.
	InlineMaxBytes int64

	// FSys is rooted at the sync source root (--from). Required for any
	// link whose effective mode is "inline"; missing FSys forces those
	// links to link mode (with a warning) so the sync still completes.
	FSys fs.FS
}

func (r *RewriteGitFileLinks) Name() string {
	return "rewrite/git-file-links"
}

// gitFileLinkRe matches a markdown link with optional bold wrappers.
// The URL group is broad — extension filtering happens in code so the
// transform can skip the right set without per-extension regex
// gymnastics.
var gitFileLinkRe = regexp.MustCompile(
	`(!?)(\*\*)?\[([^\]]*)\]\(([^()\s]+)\)(\*\*)?`,
)

// extensionsAlwaysSkipped are handled by other rewriters (md, puml) or
// are nonsensical to render as a source panel (images).
var extensionsAlwaysSkipped = map[string]struct{}{
	"md": {}, "markdown": {},
	"puml": {}, "plantuml": {},
	"png": {}, "jpg": {}, "jpeg": {}, "gif": {},
	"svg": {}, "webp": {}, "bmp": {}, "ico": {},
}

func (r *RewriteGitFileLinks) Apply(ctx *TransformContext) error {
	if r.Macro == "" || len(r.Parameters) == 0 {
		return nil
	}
	logger := r.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}

	allow := normalizeExtensions(r.Extensions)
	perExt := normalizePerExtension(r.PerExtension)
	defaultMode := normalizeMode(r.Mode)

	pageDir := path.Dir(ctx.PagePath)
	if pageDir == "." {
		pageDir = ""
	}
	repoPageDir := joinRepoPath(r.SyncRootRelRepo, pageDir)

	names := make([]string, 0, len(r.Parameters))
	for name := range r.Parameters {
		names = append(names, name)
	}
	sort.Strings(names)

	lines := strings.Split(ctx.PreContent, "\n")
	tableLines := markTableLines(lines)
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if tableLines[i] {
			// Confluence storage format doesn't render block-level
			// structured macros inside table cells; leave the link as-is.
			continue
		}
		lines[i] = gitFileLinkRe.ReplaceAllStringFunc(line, func(match string) string {
			sub := gitFileLinkRe.FindStringSubmatch(match)
			if len(sub) < 6 {
				return match
			}
			if sub[1] == "!" {
				return match
			}
			href := sub[4]
			leading := sub[2]
			trailing := sub[5]

			if !isRepoFileHref(href) {
				return match
			}

			// Strip any trailing #anchor; view-git-file doesn't take one.
			cleanHref := href
			if i := strings.Index(cleanHref, "#"); i >= 0 {
				cleanHref = cleanHref[:i]
			}
			if cleanHref == "" {
				return match
			}

			ext := strings.TrimPrefix(strings.ToLower(path.Ext(cleanHref)), ".")
			if ext == "" {
				// No extension — pass through unless the user explicitly
				// included "" in Extensions (unusual). Rendering arbitrary
				// extensionless files is risky.
				return match
			}
			if _, skip := extensionsAlwaysSkipped[ext]; skip {
				return match
			}
			if len(allow) > 0 {
				if _, ok := allow[ext]; !ok {
					return match
				}
			}

			decoded, err := url.PathUnescape(cleanHref)
			if err != nil {
				decoded = cleanHref
			}
			repoPath := path.Clean(joinRepoPath(repoPageDir, decoded))
			if repoPath == ".." || strings.HasPrefix(repoPath, "../") {
				logger.Printf("rewrite/git-file-links: %s: link %q escapes repo root", ctx.PagePath, href)
				return match
			}

			mode := defaultMode
			if m, ok := perExt[ext]; ok {
				mode = m
			}

			var macro string
			if mode == "inline" {
				syncRelTarget := path.Clean(joinRepoPath(pageDir, decoded))
				inlineMacro, ok := r.tryInline(logger, ctx.PagePath, href, syncRelTarget, ext)
				if ok {
					macro = inlineMacro
				} else {
					macro = buildParamMacro(r.Macro, names, r.Parameters, repoPath, r.Branch)
				}
			} else {
				macro = buildParamMacro(r.Macro, names, r.Parameters, repoPath, r.Branch)
			}

			if leading == "**" && trailing == "**" {
				return macro
			}
			return leading + macro + trailing
		})
	}
	ctx.PreContent = strings.Join(lines, "\n")
	return nil
}

// tryInline reads the file via FSys and returns a Confluence code-macro
// containing its contents. On any failure (no FSys, escapes sync root,
// missing file, oversize) it logs a warning and returns ("", false) so
// the caller falls back to link mode.
func (r *RewriteGitFileLinks) tryInline(logger *log.Logger, pagePath, href, syncRelTarget, ext string) (string, bool) {
	if r.FSys == nil {
		logger.Printf("rewrite/git-file-links: %s: inline mode requested for %q but no source filesystem provided; falling back to link", pagePath, href)
		return "", false
	}
	if syncRelTarget == ".." || strings.HasPrefix(syncRelTarget, "../") || syncRelTarget == "" {
		logger.Printf("rewrite/git-file-links: %s: inline target %q escapes --from; falling back to link", pagePath, href)
		return "", false
	}
	info, err := fs.Stat(r.FSys, syncRelTarget)
	if err != nil {
		logger.Printf("rewrite/git-file-links: %s: cannot stat %q for inline: %v; falling back to link", pagePath, href, err)
		return "", false
	}
	if info.IsDir() {
		logger.Printf("rewrite/git-file-links: %s: inline target %q is a directory; falling back to link", pagePath, href)
		return "", false
	}
	if r.InlineMaxBytes > 0 && info.Size() > r.InlineMaxBytes {
		logger.Printf("rewrite/git-file-links: %s: %q is %d bytes (over inline cap %d); falling back to link", pagePath, href, info.Size(), r.InlineMaxBytes)
		return "", false
	}
	data, err := fs.ReadFile(r.FSys, syncRelTarget)
	if err != nil {
		logger.Printf("rewrite/git-file-links: %s: cannot read %q for inline: %v; falling back to link", pagePath, href, err)
		return "", false
	}
	return buildInlineCodeMacro(string(data), syncRelTarget, ext), true
}

// buildInlineCodeMacro emits a Confluence "code" structured macro with
// CDATA-wrapped body, a language hint derived from the extension, and a
// title set to the file's sync-rel path so the user sees what file the
// panel shows.
func buildInlineCodeMacro(body, syncRelTarget, ext string) string {
	var b strings.Builder
	b.WriteString(`<ac:structured-macro ac:name="code" ac:schema-version="1">`)
	if lang := languageForExt(ext); lang != "" {
		b.WriteString(`<ac:parameter ac:name="language">`)
		b.WriteString(xmlBodyEscape(lang))
		b.WriteString(`</ac:parameter>`)
	}
	if syncRelTarget != "" {
		b.WriteString(`<ac:parameter ac:name="title">`)
		b.WriteString(xmlBodyEscape(syncRelTarget))
		b.WriteString(`</ac:parameter>`)
	}
	b.WriteString(`<ac:plain-text-body><![CDATA[`)
	b.WriteString(cdataEscape(body))
	b.WriteString(`]]></ac:plain-text-body></ac:structured-macro>`)
	return b.String()
}

// cdataEscape splits any literal "]]>" sequence in s by closing and
// re-opening the CDATA section, so embedded "]]>" survives parsing.
func cdataEscape(s string) string {
	if !strings.Contains(s, "]]>") {
		return s
	}
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}

// languageForExt maps a (lowercase, no-dot) extension to the language
// hint Confluence's code macro recognises. The default is to pass the
// extension through verbatim; Confluence falls back to plain rendering
// for anything it doesn't recognise, so a slightly wrong hint is harmless.
func languageForExt(ext string) string {
	switch ext {
	case "":
		return ""
	case "sh", "bash", "zsh":
		return "bash"
	case "ps1":
		return "powershell"
	case "py":
		return "python"
	case "rb":
		return "ruby"
	case "rs":
		return "rust"
	case "kt", "kts":
		return "kotlin"
	case "ts":
		return "typescript"
	case "js", "mjs", "cjs":
		return "javascript"
	case "yml":
		return "yaml"
	case "md", "markdown":
		return ""
	case "txt", "log":
		return ""
	}
	return ext
}

// normalizeMode lower-cases and validates a mode string. Unknown values
// fall back to "link" so a typo in the profile doesn't silently disable
// the rewriter — the user gets the safe default.
func normalizeMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "inline":
		return "inline"
	default:
		return "link"
	}
}

// normalizePerExtension lower-cases keys (stripping any leading dot) and
// values, dropping entries whose value isn't a recognised mode.
func normalizePerExtension(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(k), "."))
		if key == "" {
			continue
		}
		mode := normalizeMode(v)
		out[key] = mode
	}
	return out
}

// isRepoFileHref returns true for hrefs that look like repo-relative
// file references — i.e. not external, not anchor, not mailto/tel,
// not absolute paths (which on Confluence resolve to wiki URLs like
// /display/... or /pages/viewpage.action), and not query-bearing URLs.
//
// The repo-relative case is intentionally narrow: only the form
// `relative/path.ext` or `../relative/path.ext` is treated as a file
// reference. Anything more URL-ish is left for the page to render
// however Confluence interprets it.
func isRepoFileHref(href string) bool {
	if href == "" {
		return false
	}
	if strings.HasPrefix(href, "#") || strings.HasPrefix(href, "/") {
		return false
	}
	if strings.Contains(href, "://") || strings.Contains(href, "?") {
		return false
	}
	if strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
		return false
	}
	return true
}

// normalizeExtensions lowercases and strips leading dots for fast lookup.
// Empty input returns nil (caller uses len(...) == 0 as "no restriction").
func normalizeExtensions(exts []string) map[string]struct{} {
	if len(exts) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		e = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(e), "."))
		if e == "" {
			continue
		}
		out[e] = struct{}{}
	}
	return out
}
