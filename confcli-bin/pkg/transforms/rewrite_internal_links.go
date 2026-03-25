package transforms

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// RewriteInternalLinks rewrites Confluence internal page links in markdown
// PostContent to relative file paths. This is a refactored version of the logic
// from pkg/converters/link_rewriter.go.
type RewriteInternalLinks struct {
	// PageMap maps page IDs to file paths relative to the export root.
	PageMap map[int]string
	// ConfBaseURL is the Confluence base URL to match against.
	ConfBaseURL string
	// CurrentPageDir is the directory of the current page relative to the export root.
	CurrentPageDir string
	// PageExistsChecker verifies that a page ID exists (for pages not in the export).
	// If nil, unresolved links are kept as external links.
	PageExistsChecker func(pageID int) bool
}

// NewRewriteInternalLinks creates a RewriteInternalLinks transform.
func NewRewriteInternalLinks(pageMap map[int]string, confBaseURL, currentPageDir string) *RewriteInternalLinks {
	return &RewriteInternalLinks{
		PageMap:        pageMap,
		ConfBaseURL:    confBaseURL,
		CurrentPageDir: currentPageDir,
	}
}

func (r *RewriteInternalLinks) Name() string {
	return "rewrite/internal-links"
}

var (
	internalLinkRe         = regexp.MustCompile(`\[([^\]]*)\]\(([^()\s]*(?:\([^()]*\)[^()\s]*)*)\)`)
	confPageIDRe           = regexp.MustCompile(`/pages/viewpage\.action[^#]*[?&]pageId=(\d+)`)
	confSpacesPageIDRe     = regexp.MustCompile(`/spaces/~(\d+)`)
	confTinyURLRe          = regexp.MustCompile(`/x/([A-Za-z0-9_-]{4,12})$`)
)

func (r *RewriteInternalLinks) Apply(ctx *TransformContext) error {
	if r.PageMap == nil || r.ConfBaseURL == "" {
		return nil
	}

	ctx.PostContent = internalLinkRe.ReplaceAllStringFunc(ctx.PostContent, func(match string) string {
		sub := internalLinkRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		text := sub[1]
		linkURL := sub[2]

		if rewritten, ok := r.rewriteLink(linkURL, text); ok {
			return rewritten
		}

		// URL belongs to our Confluence but couldn't be resolved — strip the link.
		if strings.Contains(linkURL, r.ConfBaseURL) {
			return text
		}

		return match
	})
	return nil
}

func (r *RewriteInternalLinks) rewriteLink(linkURL, text string) (string, bool) {
	cleanURL := linkURL
	if idx := strings.Index(cleanURL, "#"); idx != -1 {
		cleanURL = cleanURL[:idx]
	}
	if idx := strings.Index(cleanURL, "%23"); idx != -1 {
		cleanURL = cleanURL[:idx]
	}

	if !strings.Contains(cleanURL, r.ConfBaseURL) {
		return "", false
	}

	pageID := r.extractPageID(cleanURL)
	if pageID == 0 {
		return "", false
	}

	targetPath, exists := r.PageMap[pageID]
	if !exists {
		if r.PageExistsChecker != nil && !r.PageExistsChecker(pageID) {
			return text, true
		}
		return fmt.Sprintf("[%s](%s)", text, cleanURL), true
	}

	relPath := targetPath
	if r.CurrentPageDir != "" {
		if rel, err := filepath.Rel(r.CurrentPageDir, targetPath); err == nil {
			relPath = rel
		}
	}
	relPath = filepath.ToSlash(relPath)

	return fmt.Sprintf("[%s](%s)", text, relPath), true
}

func (r *RewriteInternalLinks) extractPageID(url string) int {
	// Try /pages/viewpage.action?pageId=NNN
	if m := confPageIDRe.FindStringSubmatch(url); len(m) >= 2 {
		if id, err := strconv.Atoi(m[1]); err == nil {
			return id
		}
	}

	// Try /spaces/~NNN
	if m := confSpacesPageIDRe.FindStringSubmatch(url); len(m) >= 2 {
		if id, err := strconv.Atoi(m[1]); err == nil {
			return id
		}
	}

	// Try /x/XXXXX (tiny URL)
	if m := confTinyURLRe.FindStringSubmatch(url); len(m) >= 2 {
		if id, err := decodeTinyURL(m[1]); err == nil {
			return int(id)
		}
	}

	return 0
}

// decodeTinyURL decodes a Confluence tiny URL ID to a page ID.
func decodeTinyURL(tinyID string) (int64, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(tinyID)
	if err != nil {
		return 0, fmt.Errorf("invalid tiny URL encoding: %w", err)
	}
	for len(decoded) < 4 {
		decoded = append(decoded, 0)
	}
	if len(decoded) >= 8 {
		return int64(binary.LittleEndian.Uint64(decoded[:8])), nil
	}
	return int64(binary.LittleEndian.Uint32(decoded[:4])), nil
}
