package converters

import (
	"fmt"
	"html"
	"path"
	"regexp"
	"strings"
)

// GitFileRef describes one view-git-file macro (Git for Confluence plugin)
// that renders a PlantUML diagram from a repository file.
type GitFileRef struct {
	Path         string // repo-relative file path, e.g. docs/diagrams/x.puml
	Branch       string // raw branch parameter, e.g. refs/remotes/origin/develop
	RepositoryID string // plugin-side repository id (opaque number)
}

// viewGitFileMacroRe matches a view-git-file structured macro in storage
// format and captures its inner parameters.
var viewGitFileMacroRe = regexp.MustCompile(
	`(?si)<ac:structured-macro\b[^>]*\bac:name\s*=\s*"view-git-file"[^>]*>(.*?)</ac:structured-macro>`)

var acParameterRe = regexp.MustCompile(
	`(?si)<ac:parameter\b[^>]*\bac:name\s*=\s*"([^"]*)"[^>]*>(.*?)</ac:parameter>`)

// gitFileImgRe matches the <img> the Git for Confluence plugin emits in
// export_view for a rendered PlantUML file. The src looks like
// /download/export/git-for-confluence<digits>png and carries no hint of
// the source file, so the storage macro is the only way to recover it.
var gitFileImgRe = regexp.MustCompile(
	`(?i)<img\b[^>]*\bsrc\s*=\s*(?:"[^"]*git-for-confluence[^"]*"|'[^']*git-for-confluence[^']*')[^>]*/?>`)

// gitFileImgWrapperRe matches the bare list wrapper the plugin puts around
// the image when the panel is off:
//
//	<div><ul style="list-style-type: none; padding-left: 0"><li><img ...></li></ul></div>
//
// The same wrapper also appears inside <div class="git-plugin-content">
// when the panel is on; callers decide per match whether to touch it.
var gitFileImgWrapperRe = regexp.MustCompile(
	`(?is)(?:<div>\s*<ul\b[^>]*>\s*<li>\s*)?` +
		`<img\b[^>]*\bsrc\s*=\s*(?:"[^"]*git-for-confluence[^"]*"|'[^']*git-for-confluence[^']*')[^>]*/?>` +
		`(?:\s*</li>\s*</ul>\s*</div>)?`)

// ExtractGitFileRefs returns, in document order, every view-git-file macro
// in the storage HTML that has renderpuml=true — i.e. the ones export_view
// renders as a git-for-confluence image. Macros showing a file as source
// (no renderpuml) produce no image and are skipped so that positional
// pairing with the images stays aligned.
func ExtractGitFileRefs(storage string) []GitFileRef {
	var out []GitFileRef
	for _, m := range viewGitFileMacroRe.FindAllStringSubmatch(storage, -1) {
		var ref GitFileRef
		renderPUML := false
		for _, p := range acParameterRe.FindAllStringSubmatch(m[1], -1) {
			name := strings.ToLower(strings.TrimSpace(p[1]))
			val := strings.TrimSpace(html.UnescapeString(p[2]))
			switch name {
			case "path":
				ref.Path = val
			case "branch":
				ref.Branch = val
			case "repository-id":
				ref.RepositoryID = val
			case "renderpuml":
				renderPUML = strings.EqualFold(val, "true")
			}
		}
		if !renderPUML || ref.Path == "" {
			continue
		}
		out = append(out, ref)
	}
	return out
}

// HasGitFileImages reports whether export_view HTML contains rendered
// Git-for-Confluence PlantUML images.
func HasGitFileImages(htmlContent string) bool {
	return gitFileImgRe.MatchString(htmlContent)
}

// CountGitFileImages counts rendered Git-for-Confluence images in
// export_view HTML.
func CountGitFileImages(htmlContent string) int {
	return len(gitFileImgRe.FindAllString(htmlContent, -1))
}

// InjectGitFileRefs replaces panel-less Git-for-Confluence PlantUML images
// in export_view HTML with a bold link to the source file, resolved
// positionally from refs (the i-th image pairs with the i-th ref).
//
// Images that sit inside a <div class="git-plugin-container"> (macro with
// renderpanel=true) are left alone: the panel already carries the file
// name and a link, and handleGitPluginContainer renders those. Without the
// panel the image is the only trace of the diagram, and stripJunkImages
// later removes /download/export/ images, so the page would otherwise lose
// the diagram entirely.
//
// The emitted form — a minimal git-plugin-container — renders to the same
// markdown as the panel variant (**[name](path)**), which is also the
// shape `confcli sync` turns back into a view-git-file macro.
func InjectGitFileRefs(exportView string, refs []GitFileRef) string {
	if len(refs) == 0 || !HasGitFileImages(exportView) {
		return exportView
	}
	var b strings.Builder
	idx := 0  // index into refs, advanced for every image (panel or not)
	prev := 0 // end of the previous match, for the in-panel check
	for _, loc := range gitFileImgWrapperRe.FindAllStringIndex(exportView, -1) {
		b.WriteString(exportView[prev:loc[0]])
		match := exportView[loc[0]:loc[1]]
		// A panel container opens after the previous image and before this
		// one iff this image belongs to a panel.
		inPanel := strings.Contains(exportView[prev:loc[0]], "git-plugin-content")
		ref, ok := GitFileRef{}, false
		if idx < len(refs) {
			ref, ok = refs[idx], true
		}
		idx++
		if inPanel || !ok {
			b.WriteString(match)
		} else {
			b.WriteString(gitFileRefHTML(ref))
		}
		prev = loc[1]
	}
	b.WriteString(exportView[prev:])
	return b.String()
}

// gitFileRefHTML builds a minimal git-plugin-container so the existing
// handleGitPluginContainer renders it exactly like a panel-wrapped macro:
// **[basename](path)**. Emitting <strong><a> directly would come out as
// [**basename**](path) after the markdown library normalises emphasis.
func gitFileRefHTML(ref GitFileRef) string {
	name := path.Base(ref.Path)
	p := html.EscapeString(ref.Path)
	return fmt.Sprintf(`<div class="git-plugin-container"><strong>%s</strong><a href="%s">%s</a></div>`,
		html.EscapeString(name), p, p)
}
