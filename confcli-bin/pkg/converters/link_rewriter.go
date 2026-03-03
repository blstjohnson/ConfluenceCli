package converters

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// LinkRewriteConfig configures how links are rewritten in converted markdown
type LinkRewriteConfig struct {
	// Confluence page link rewriting (for space export)
	PageMap     map[int]string // pageID -> file path relative to space root dir
	ConfBaseURL string         // Confluence base URL to match against

	// PageExistsChecker is an optional callback invoked for Confluence page IDs
	// that are NOT present in PageMap (i.e. links to external Confluence pages).
	// When provided it is called to verify that the target page is accessible
	// (not deleted, not missing). If the callback returns false the link is
	// stripped and only the visible link text is preserved.
	PageExistsChecker func(pageID int) bool

	// TFS/Git link rewriting
	TFSBaseURL    string // TFS base URL pattern to match (e.g., "tfs.ekassir.com")
	LocalRepoPath string // Local path prefix for repo files (can be absolute)

	// Context for relative path computation
	CurrentPageDir  string // Directory of the current page relative to space root (for computing relative Confluence links)
	CurrentFilePath string // Absolute path to the current .md file on disk (for computing relative TFS links)
}

// markdownLinkRe matches markdown links: [text](url)
// Supports URLs that contain balanced (but not nested) parentheses — common in
// Confluence anchor links such as #heading(term)rest(term2).
// Pattern for URL group: zero-or-more non-paren chars, optionally followed by
// one or more (inner-paren-content) segments also separated by non-paren chars.
var markdownLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^()\s]*(?:\([^()]*\)[^()\s]*)*)\)`)

// confluencePageIDRe matches Confluence page URLs containing pageId query parameter.
// Uses [^#]* to skip any other query parameters that may precede pageId (e.g. &pageId=).
var confluencePageIDRe = regexp.MustCompile(`/pages/viewpage\.action[^#]*[?&]pageId=(\d+)`)

// confluenceSpacesPageIDRe matches /spaces/~pageID or /spaces/~pageID/... patterns
var confluenceSpacesPageIDRe = regexp.MustCompile(`/spaces/~(\d+)`)

// confluenceDisplayRe matches /display/SPACE/Page+Title patterns
var confluenceDisplayRe = regexp.MustCompile(`/display/([^/]+)/(.+)`)

// confluenceTinyURLRe matches /x/XXXXX tiny URL patterns
var confluenceTinyURLRe = regexp.MustCompile(`/x/([A-Za-z0-9_-]{4,12})$`)

// RewriteLinks rewrites markdown links according to the provided configuration.
// It handles:
// - Confluence internal page links -> relative file paths (when PageMap is provided)
// - TFS/Git repository links -> local file paths (when TFSBaseURL is provided)
func RewriteLinks(markdown string, config *LinkRewriteConfig) string {
	if config == nil {
		return markdown
	}

	return markdownLinkRe.ReplaceAllStringFunc(markdown, func(match string) string {
		submatches := markdownLinkRe.FindStringSubmatch(match)
		if len(submatches) < 3 {
			return match
		}
		text := submatches[1]
		linkURL := submatches[2]

		// Try Confluence page link rewriting
		if config.PageMap != nil && config.ConfBaseURL != "" {
			if rewritten, ok := rewriteConfluenceLink(linkURL, text, config); ok {
				return rewritten
			}
			// URL belongs to our Confluence instance but could not be resolved to a
			// local file (page not in export, wrong space, unrecognised URL format, …).
			// Remove the link entirely and keep only the visible link text.
			if strings.Contains(linkURL, config.ConfBaseURL) {
				return text
			}
		}

		// Try TFS link rewriting
		if config.TFSBaseURL != "" {
			if rewritten, ok := rewriteTFSLink(linkURL, text, config); ok {
				return rewritten
			}
		}

		return match
	})
}

// rewriteConfluenceLink tries to rewrite a Confluence page URL to a relative file path
func rewriteConfluenceLink(linkURL, text string, config *LinkRewriteConfig) (string, bool) {
	// Strip anchor/fragment before matching patterns.
	// Handle both the literal '#' form and the percent-encoded '%23' form.
	cleanURL := linkURL
	if idx := strings.Index(cleanURL, "#"); idx != -1 {
		cleanURL = cleanURL[:idx]
	}
	if idx := strings.Index(cleanURL, "%23"); idx != -1 {
		cleanURL = cleanURL[:idx]
	}

	// Only process links that point to the configured Confluence instance
	if !strings.Contains(cleanURL, config.ConfBaseURL) {
		return "", false
	}

	var pageID int

	// Try /pages/viewpage.action?pageId=NNN
	if matches := confluencePageIDRe.FindStringSubmatch(cleanURL); len(matches) >= 2 {
		if id, err := strconv.Atoi(matches[1]); err == nil {
			pageID = id
		}
	}

	// Try /spaces/~NNN
	if pageID == 0 {
		if matches := confluenceSpacesPageIDRe.FindStringSubmatch(cleanURL); len(matches) >= 2 {
			if id, err := strconv.Atoi(matches[1]); err == nil {
				pageID = id
			}
		}
	}

	// Try /x/XXXXX (tiny URL — base64url-encoded page ID)
	if pageID == 0 {
		if matches := confluenceTinyURLRe.FindStringSubmatch(cleanURL); len(matches) >= 2 {
			if id, err := DecodeTinyURL(matches[1]); err == nil {
				pageID = int(id)
			}
		}
	}

	if pageID == 0 {
		return "", false
	}

	// Look up the page in our map
	targetPath, exists := config.PageMap[pageID]
	if !exists {
		// Page not in this export (different space or excluded by depth limit).
		// If a page-existence checker is provided, verify the target page is still
		// accessible. If it is deleted or missing, strip the link entirely.
		if config.PageExistsChecker != nil && !config.PageExistsChecker(pageID) {
			return text, true
		}
		// Return the anchor-stripped URL as a fallback external link so the reader
		// can still navigate to the Confluence page.
		return fmt.Sprintf("[%s](%s)", text, cleanURL), true
	}

	// Compute relative path from current page directory to target
	relPath := targetPath
	if config.CurrentPageDir != "" {
		var err error
		relPath, err = filepath.Rel(config.CurrentPageDir, targetPath)
		if err != nil {
			relPath = targetPath
		}
	}

	// Normalize to forward slashes for markdown
	relPath = filepath.ToSlash(relPath)

	return fmt.Sprintf("[%s](%s)", text, relPath), true
}

// rewriteTFSLink tries to rewrite a TFS/Git repository URL to a local file path
func rewriteTFSLink(linkURL, text string, config *LinkRewriteConfig) (string, bool) {
	// Extract hostname from TFSBaseURL so we match regardless of protocol
	// (ssh://tfs.example.com vs https://tfs.example.com)
	tfsHost := config.TFSBaseURL
	if parsed, err := url.Parse(config.TFSBaseURL); err == nil && parsed.Hostname() != "" {
		tfsHost = parsed.Hostname()
	}
	if !strings.Contains(linkURL, tfsHost) {
		return "", false
	}

	// Only rewrite URLs that contain /_git/ (file links).
	// Skip work items (/_workitems/), issues, and other TFS web links.
	if !strings.Contains(linkURL, "/_git/") {
		return "", false
	}

	// Extract the file path after _git/RepoName/
	// Handles both:
	//   ssh://tfs.example.com/Collection/Project/_git/Repo/path/to/file.ext
	//   https://tfs.example.com/Collection/Project/_git/Repo?path=/path/to/file.ext
	filePath := extractTFSFilePath(linkURL)
	if filePath == "" {
		return "", false
	}

	// Build the full local path
	localPath := filePath
	if config.LocalRepoPath != "" {
		localPath = filepath.Join(config.LocalRepoPath, filePath)
	}

	// If the result is an absolute path and we know the current file location,
	// convert to a relative path from the current markdown file's directory
	if filepath.IsAbs(localPath) && config.CurrentFilePath != "" {
		currentDir := filepath.Dir(config.CurrentFilePath)
		if relPath, err := filepath.Rel(currentDir, localPath); err == nil {
			localPath = relPath
		}
	}

	// Normalize to forward slashes for markdown
	localPath = filepath.ToSlash(localPath)

	return fmt.Sprintf("[%s](%s)", text, localPath), true
}

// extractTFSFilePath extracts the file path from a TFS/Git URL
func extractTFSFilePath(rawURL string) string {
	// Try to find _git/RepoName/ in the URL and extract the path after it
	gitIdx := strings.Index(rawURL, "/_git/")
	if gitIdx == -1 {
		return ""
	}

	afterGit := rawURL[gitIdx+len("/_git/"):]

	// Check if this is a query-parameter style URL (path=...)
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Query().Get("path") != "" {
		p := parsed.Query().Get("path")
		// Remove leading slash
		p = strings.TrimPrefix(p, "/")
		return p
	}

	// Otherwise, it's a path-style URL: _git/RepoName/path/to/file
	// Skip the repo name (first segment)
	parts := strings.SplitN(afterGit, "/", 2)
	if len(parts) < 2 {
		return ""
	}

	// Remove any query string or fragment from the path
	filePath := parts[1]
	if idx := strings.IndexAny(filePath, "?#"); idx != -1 {
		filePath = filePath[:idx]
	}

	return filePath
}

// BuildPageFileMap builds a mapping from page ID to file path relative to the space directory.
// This is used during space export to resolve Confluence internal links to local file paths.
func BuildPageFileMap(pageMap map[int]string) map[int]string {
	// pageMap is already in the format we need: pageID -> relative file path
	return pageMap
}

// DecodeTinyURL decodes a Confluence tiny URL ID to a page ID.
// Confluence tiny URLs (/x/XXXXX) encode the page ID as base64url(littleEndian(uint32(pageID))).
func DecodeTinyURL(tinyID string) (int64, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(tinyID)
	if err != nil {
		return 0, fmt.Errorf("invalid tiny URL encoding: %w", err)
	}
	// Pad to 4 bytes if shorter (small page IDs)
	for len(decoded) < 4 {
		decoded = append(decoded, 0)
	}
	if len(decoded) >= 8 {
		return int64(binary.LittleEndian.Uint64(decoded[:8])), nil
	}
	return int64(binary.LittleEndian.Uint32(decoded[:4])), nil
}

// EncodeTinyURL encodes a page ID to a Confluence tiny URL ID.
func EncodeTinyURL(pageID int64) string {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(pageID))
	return base64.RawURLEncoding.EncodeToString(buf)
}
