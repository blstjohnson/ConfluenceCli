package transforms

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
)

// PageRef identifies a Confluence page target for an ac:link macro.
type PageRef struct {
	PageID   int    // present for logging only; ac:link uses title
	Title    string // required
	SpaceKey string // empty = same space as source page
}

// RewriteMarkdownLinks rewrites relative markdown links to other repository
// .md files into Confluence <ac:link> macros, so they survive the
// MD→storage conversion as cross-page links rather than literal hrefs.
//
// Runs pre-conversion against ctx.PreContent (the markdown source). The
// emitted XML is passed through by goldmark's unsafe HTML renderer and
// reaches the storage output intact.
//
// Resolution rules:
//   - href is URL-decoded, then path-joined with CurrentPageDir to produce
//     a repo-relative slash path used as the lookup key in PathMap.
//   - Anchors (#section) are preserved on the resulting ac:link as
//     ac:anchor. The anchor is URL-decoded before being emitted.
//   - Cross-space links emit ri:space-key; same-space links omit it.
//
// Skipped (left untouched):
//   - external URLs (anything with "://" or mailto:/tel:)
//   - in-page anchor-only links (#section)
//   - non-.md hrefs (images, code files, etc.)
//   - image links (![alt](path))
//
// Unresolved internal .md links — target is in the repo path namespace but
// not in PathMap — are warned to the logger and reduced to the link text.
//
// Link text is treated as literal plain text via ac:plain-text-link-body.
// Inline markdown like **bold** inside link text is not rendered as rich
// content; that is a known limitation.
type RewriteMarkdownLinks struct {
	// PathMap maps repo-relative slash paths (e.g. "engineering/setup.md")
	// to the target Confluence page. Nil = no-op.
	PathMap map[string]PageRef

	// CurrentPageDir is the repo-relative slash directory of the current
	// file. Empty = file at repo root.
	CurrentPageDir string

	// Logger receives warnings for unresolved or repo-escaping links.
	// If nil, a stderr-backed logger is used.
	Logger *log.Logger
}

// NewRewriteMarkdownLinks builds a RewriteMarkdownLinks ready for use by
// the sync engine.
func NewRewriteMarkdownLinks(pathMap map[string]PageRef, currentPageDir string) *RewriteMarkdownLinks {
	return &RewriteMarkdownLinks{
		PathMap:        pathMap,
		CurrentPageDir: currentPageDir,
	}
}

func (r *RewriteMarkdownLinks) Name() string {
	return "rewrite/md-links"
}

var mdLinkRe = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)\s]+)\)`)

func (r *RewriteMarkdownLinks) Apply(ctx *TransformContext) error {
	if r.PathMap == nil {
		return nil
	}
	logger := r.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}

	ctx.PreContent = mdLinkRe.ReplaceAllStringFunc(ctx.PreContent, func(match string) string {
		sub := mdLinkRe.FindStringSubmatch(match)
		if len(sub) < 4 {
			return match
		}
		bang := sub[1]
		text := sub[2]
		href := sub[3]

		if bang == "!" {
			return match
		}
		if !isInternalMDLink(href) {
			return match
		}

		targetPath, anchor := splitAnchor(href)
		resolved, err := resolveRepoPath(r.CurrentPageDir, targetPath)
		if err != nil {
			logger.Printf("rewrite/md-links: %s: cannot resolve %q: %v", ctx.PagePath, href, err)
			return text
		}

		ref, ok := r.PathMap[resolved]
		if !ok {
			logger.Printf("rewrite/md-links: %s: unresolved link %q (target %q not in map)", ctx.PagePath, href, resolved)
			return text
		}

		return buildAcLink(ref, anchor, text)
	})
	return nil
}

func isInternalMDLink(href string) bool {
	if href == "" {
		return false
	}
	if strings.Contains(href, "://") {
		return false
	}
	if strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
		return false
	}
	if strings.HasPrefix(href, "#") {
		return false
	}
	p := href
	if i := strings.Index(p, "#"); i >= 0 {
		p = p[:i]
	}
	return strings.HasSuffix(strings.ToLower(p), ".md")
}

func splitAnchor(href string) (linkPath, anchor string) {
	i := strings.Index(href, "#")
	if i < 0 {
		return href, ""
	}
	return href[:i], href[i+1:]
}

func resolveRepoPath(currentDir, target string) (string, error) {
	decoded, err := url.PathUnescape(target)
	if err != nil {
		return "", fmt.Errorf("url-decode: %w", err)
	}
	joined := path.Join(currentDir, decoded)
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "", fmt.Errorf("escapes repo root: %s", joined)
	}
	return joined, nil
}

func buildAcLink(ref PageRef, anchor, text string) string {
	var b strings.Builder
	b.WriteString("<ac:link")
	if anchor != "" {
		decoded, err := url.PathUnescape(anchor)
		if err != nil {
			decoded = anchor
		}
		b.WriteString(` ac:anchor="`)
		b.WriteString(xmlAttrEscape(decoded))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	b.WriteString("<ri:page")
	if ref.SpaceKey != "" {
		b.WriteString(` ri:space-key="`)
		b.WriteString(xmlAttrEscape(ref.SpaceKey))
		b.WriteString(`"`)
	}
	b.WriteString(` ri:content-title="`)
	b.WriteString(xmlAttrEscape(ref.Title))
	b.WriteString(`" />`)
	b.WriteString(`<ac:plain-text-link-body><![CDATA[`)
	b.WriteString(cdataSafe(text))
	b.WriteString(`]]></ac:plain-text-link-body>`)
	b.WriteString("</ac:link>")
	return b.String()
}

func xmlAttrEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// cdataSafe guards a string against premature ]]> terminators by splitting
// any occurrence across a CDATA boundary.
func cdataSafe(s string) string {
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}
