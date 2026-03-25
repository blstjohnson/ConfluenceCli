package transforms

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// RewriteTFSLinks rewrites TFS/Git repository URLs in markdown PostContent
// to local file paths. This is a refactored version of the logic from
// pkg/converters/link_rewriter.go.
type RewriteTFSLinks struct {
	// TFSBaseURL is the TFS base URL pattern to match (e.g., "https://tfs.ekassir.com").
	TFSBaseURL string
	// LocalRepoPath is the local path prefix for repo files.
	LocalRepoPath string
	// CurrentFilePath is the absolute path to the current .md file on disk
	// (used for computing relative paths).
	CurrentFilePath string

	tfsHost string
}

// NewRewriteTFSLinks creates a RewriteTFSLinks transform.
func NewRewriteTFSLinks(tfsBaseURL, localRepoPath, currentFilePath string) *RewriteTFSLinks {
	host := tfsBaseURL
	if parsed, err := url.Parse(tfsBaseURL); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	return &RewriteTFSLinks{
		TFSBaseURL:      tfsBaseURL,
		LocalRepoPath:   localRepoPath,
		CurrentFilePath: currentFilePath,
		tfsHost:         host,
	}
}

func (r *RewriteTFSLinks) Name() string {
	return "rewrite/tfs-links"
}

// tfsMarkdownLinkRe matches markdown links (same pattern as converters package).
var tfsMarkdownLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^()\s]*(?:\([^()]*\)[^()\s]*)*)\)`)

func (r *RewriteTFSLinks) Apply(ctx *TransformContext) error {
	ctx.PostContent = tfsMarkdownLinkRe.ReplaceAllStringFunc(ctx.PostContent, func(match string) string {
		sub := tfsMarkdownLinkRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		text := sub[1]
		linkURL := sub[2]

		if rewritten, ok := r.rewriteLink(linkURL, text); ok {
			return rewritten
		}
		return match
	})
	return nil
}

func (r *RewriteTFSLinks) rewriteLink(linkURL, text string) (string, bool) {
	if !strings.Contains(linkURL, r.tfsHost) {
		return "", false
	}

	// Only rewrite URLs that contain /_git/ (file links).
	if !strings.Contains(linkURL, "/_git/") {
		return "", false
	}

	filePath := extractTFSFilePath(linkURL)
	if filePath == "" {
		return "", false
	}

	localPath := filePath
	if r.LocalRepoPath != "" {
		localPath = filepath.Join(r.LocalRepoPath, filePath)
	}

	if filepath.IsAbs(localPath) && r.CurrentFilePath != "" {
		currentDir := filepath.Dir(r.CurrentFilePath)
		if relPath, err := filepath.Rel(currentDir, localPath); err == nil {
			localPath = relPath
		}
	}

	localPath = filepath.ToSlash(localPath)
	return fmt.Sprintf("[%s](%s)", text, localPath), true
}

// extractTFSFilePath extracts the file path from a TFS/Git URL.
func extractTFSFilePath(rawURL string) string {
	gitIdx := strings.Index(rawURL, "/_git/")
	if gitIdx == -1 {
		return ""
	}

	afterGit := rawURL[gitIdx+len("/_git/"):]

	if parsed, err := url.Parse(rawURL); err == nil && parsed.Query().Get("path") != "" {
		p := parsed.Query().Get("path")
		p = strings.TrimPrefix(p, "/")
		return p
	}

	parts := strings.SplitN(afterGit, "/", 2)
	if len(parts) < 2 {
		return ""
	}

	filePath := parts[1]
	if idx := strings.IndexAny(filePath, "?#"); idx != -1 {
		filePath = filePath[:idx]
	}

	return filePath
}
