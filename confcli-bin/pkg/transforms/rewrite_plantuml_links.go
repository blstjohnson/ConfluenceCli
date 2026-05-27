package transforms

import (
	"log"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

// RewritePlantUMLLinks rewrites markdown links to .puml files into
// Confluence macros (typically "view-git-file") that render the diagram
// from the repository at view time. The source stays in git and updates
// flow without re-syncing.
//
// Runs pre-conversion on PreContent. Skips:
//   - image embeds (![text](file.puml))
//   - links inside fenced code blocks
//   - links whose target resolves outside the repo root (logged)
//
// Bold wrappers around the link (**[text](file.puml)**) are stripped
// when they bracket only the link — matching EmbedPlantUMLLinks's
// behavior — so the macro doesn't end up inside bold formatting.
type RewritePlantUMLLinks struct {
	// Macro is the Confluence macro name (e.g. "view-git-file").
	Macro string

	// Parameters maps ac:parameter names to value templates. Values
	// may contain {path} and {branch} placeholders that the rewriter
	// substitutes per link; other text is emitted verbatim.
	Parameters map[string]string

	// Branch is the git branch name substituted into {branch}.
	Branch string

	// SyncRootRelRepo is the slash path from the git repo root to the
	// sync source root (--from). Empty or "." means --from == repo root.
	// Used to resolve page-relative .puml paths against the repo, since
	// PagePath values are sync-root-relative.
	SyncRootRelRepo string

	// Logger receives warnings for unresolvable links. Defaults to a
	// stderr-backed logger when nil.
	Logger *log.Logger
}

func (r *RewritePlantUMLLinks) Name() string {
	return "rewrite/plantuml-links"
}

// plantumlLinkRe matches a markdown link to a .puml / .plantuml file.
// Groups: 1 leading "!" (image), 2 leading "**", 3 link text, 4 URL,
// 5 trailing "**". Mirrors EmbedPlantUMLLinks's regex for consistency.
var plantumlLinkRe = regexp.MustCompile(
	`(!?)(\*\*)?\[([^\]]*)\]\(([^()\s]*\.(?i:puml|plantuml))\)(\*\*)?`,
)

func (r *RewritePlantUMLLinks) Apply(ctx *TransformContext) error {
	if r.Macro == "" || len(r.Parameters) == 0 {
		return nil
	}
	logger := r.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}

	pageDir := path.Dir(ctx.PagePath)
	if pageDir == "." {
		pageDir = ""
	}
	repoPageDir := joinRepoPath(r.SyncRootRelRepo, pageDir)

	// Sort parameter names so the emitted XML is deterministic across
	// runs. Map iteration order in Go is randomized; stable output
	// matters for hash-based change detection and for diffs in tests.
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
			// Block-level macros don't render inside table cells;
			// leave the markdown link as-is.
			continue
		}
		lines[i] = plantumlLinkRe.ReplaceAllStringFunc(line, func(match string) string {
			sub := plantumlLinkRe.FindStringSubmatch(match)
			if len(sub) < 6 {
				return match
			}
			if sub[1] == "!" {
				return match
			}
			href := sub[4]
			leading := sub[2]
			trailing := sub[5]

			decoded, err := url.PathUnescape(href)
			if err != nil {
				decoded = href
			}
			repoPath := path.Clean(joinRepoPath(repoPageDir, decoded))
			if repoPath == ".." || strings.HasPrefix(repoPath, "../") {
				logger.Printf("rewrite/plantuml-links: %s: link %q escapes repo root", ctx.PagePath, href)
				return match
			}

			macro := r.buildMacro(repoPath, names)

			if leading == "**" && trailing == "**" {
				return macro
			}
			return leading + macro + trailing
		})
	}
	ctx.PreContent = strings.Join(lines, "\n")
	return nil
}

func (r *RewritePlantUMLLinks) buildMacro(repoPath string, paramNames []string) string {
	return buildParamMacro(r.Macro, paramNames, r.Parameters, repoPath, r.Branch)
}

// buildParamMacro emits an <ac:structured-macro> with the given name
// and parameters. paramNames is the order parameters are emitted in
// (pass a sorted slice for deterministic output). {path} and {branch}
// placeholders in each parameter value are substituted; everything
// else is XML-body-escaped.
//
// Shared between RewritePlantUMLLinks and RewriteGitFileLinks — both
// emit the same macro shape, just with different selection logic and
// (typically) different parameters.
func buildParamMacro(macro string, paramNames []string, params map[string]string, repoPath, branch string) string {
	var b strings.Builder
	b.WriteString(`<ac:structured-macro ac:name="`)
	b.WriteString(xmlAttrEscape(macro))
	b.WriteString(`" ac:schema-version="1">`)
	for _, name := range paramNames {
		value := params[name]
		value = strings.ReplaceAll(value, "{path}", repoPath)
		value = strings.ReplaceAll(value, "{branch}", branch)
		b.WriteString(`<ac:parameter ac:name="`)
		b.WriteString(xmlAttrEscape(name))
		b.WriteString(`">`)
		b.WriteString(xmlBodyEscape(value))
		b.WriteString(`</ac:parameter>`)
	}
	b.WriteString(`</ac:structured-macro>`)
	return b.String()
}

// xmlBodyEscape escapes characters that are special in XML element
// content: &, <, >. Attributes also need " escaped (handled separately
// by xmlAttrEscape).
func xmlBodyEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// joinRepoPath joins repo-root-relative slash paths the way path.Join
// does but treats "" as identity. Cleans the result so "a/b/../c" → "a/c".
func joinRepoPath(parts ...string) string {
	var filtered []string
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		filtered = append(filtered, p)
	}
	if len(filtered) == 0 {
		return ""
	}
	return path.Clean(strings.Join(filtered, "/"))
}
