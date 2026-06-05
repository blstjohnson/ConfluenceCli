package sync

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
)

// ImageRef is a local image referenced by a page, resolved to its bytes so
// the executor can upload it as an attachment after the page is created or
// updated. Filename is the basename used as the ri:attachment filename.
type ImageRef struct {
	Filename string
	Data     []byte
}

// imageExtensions is the set of file extensions (lowercase, with dot) that
// rewriteImages treats as inline images. An <img> pointing at anything else
// is left as a literal tag and warned about.
var imageExtensions = map[string]struct{}{
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".svg":  {},
	".webp": {},
	".bmp":  {},
}

var (
	imgTagRe = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	imgSrcRe = regexp.MustCompile(`(?i)\bsrc\s*=\s*"([^"]*)"`)
	imgAltRe = regexp.MustCompile(`(?i)\balt\s*=\s*"([^"]*)"`)

	// aLinkRe matches a goldmark-rendered anchor (<a href="...">text</a>).
	// `\b` after "a" keeps it from matching <ac:link>/<ac:structured-macro>
	// (the next char "c" is a word char, so there's no boundary). Plain
	// links to image files are the ones we turn into attachment downloads.
	aLinkRe    = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	hrefAttrRe = regexp.MustCompile(`(?i)\bhref\s*=\s*"([^"]*)"`)
	htmlTagRe  = regexp.MustCompile(`<[^>]*>`)
)

// rewriteImages turns local image references in the storage payload into
// Confluence attachment macros and collects the referenced files so they can
// be uploaded to the page. Two markdown forms are handled:
//
//   - image embeds ![alt](x.svg) → <img> → <ac:image><ri:attachment/></ac:image>
//   - plain links  [text](x.svg)  → <a>  → <ac:link><ri:attachment/>...</ac:link>
//     (an attachment download link, so the link is no longer a broken relative href)
//
// relPath is the sync-root-relative path of the source markdown file; image
// srcs are resolved relative to its directory. When the reference escapes the
// --from root (e.g. ../../diagrams/x.svg pointing elsewhere in the repo), it is
// resolved against repoFS using repoRel (the sync-root-relative-to-repo path);
// repoFS is nil for non-git trees, in which case only the --from filesystem is
// consulted. Remote images (http/https/protocol-relative/data URIs) are left
// untouched. A local image src that cannot be read, or an <img> whose extension
// is not a known image type, is left as-is and logged (pass a nil logger to
// silence). Plain links to non-image files are left untouched. Content inside
// <![CDATA[...]]> (code blocks) is passed through verbatim.
//
// Returned images are deduplicated by basename; the first occurrence's bytes
// win. A basename collision between two different source files on the same
// page is logged.
func rewriteImages(storage, relPath string, fsys, repoFS fs.FS, repoRel string, logger *log.Logger) (string, []ImageRef, error) {
	lower := strings.ToLower(storage)
	if !strings.Contains(lower, "<img") && !strings.Contains(lower, "<a ") {
		return storage, nil, nil
	}

	baseDir := path.Dir(relPath)
	if baseDir == "." {
		baseDir = ""
	}

	collected := map[string][]byte{} // basename → bytes
	var order []string               // basenames in first-seen order

	collect := func(base string, data []byte) {
		if existing, seen := collected[base]; !seen {
			collected[base] = data
			order = append(order, base)
		} else if logger != nil && !bytesEqual(existing, data) {
			logger.Printf("sync images: %s references two different files named %q; keeping the first", relPath, base)
		}
	}

	// loadImage reads the bytes for a markdown image/link src, trying the
	// repo-root filesystem first (so references that escape --from but stay
	// inside the repo resolve) and falling back to the --from filesystem.
	loadImage := func(src string) (data []byte, base string, ok bool) {
		if repoFS != nil {
			if rp := repoImagePath(repoRel, baseDir, src); rp != "" && !escapesRoot(rp) {
				if d, err := fs.ReadFile(repoFS, rp); err == nil {
					return d, path.Base(rp), true
				}
			}
		}
		if fp := resolveImagePath(baseDir, src); fp != "" && !escapesRoot(fp) {
			if d, err := fs.ReadFile(fsys, fp); err == nil {
				return d, path.Base(fp), true
			}
		}
		return nil, "", false
	}

	rewriteImg := func(tag string) string {
		srcMatch := imgSrcRe.FindStringSubmatch(tag)
		if srcMatch == nil {
			return tag
		}
		src := strings.TrimSpace(srcMatch[1])
		if src == "" || isRemoteRef(src) {
			return tag
		}

		ext := imageExtOf(src)
		if _, ok := imageExtensions[ext]; !ok {
			if logger != nil {
				logger.Printf("sync images: %s references %q with unsupported extension %q; left as-is", relPath, src, ext)
			}
			return tag
		}

		data, base, ok := loadImage(src)
		if !ok {
			if logger != nil {
				logger.Printf("sync images: %s references %q which could not be read; left as-is", relPath, src)
			}
			return tag
		}
		collect(base, data)

		alt := ""
		if m := imgAltRe.FindStringSubmatch(tag); m != nil {
			alt = m[1]
		}
		return imageMacro(base, alt)
	}

	rewriteLink := func(tag string) string {
		m := aLinkRe.FindStringSubmatch(tag)
		if m == nil {
			return tag
		}
		hrefMatch := hrefAttrRe.FindStringSubmatch(m[1])
		if hrefMatch == nil {
			return tag
		}
		src := strings.TrimSpace(hrefMatch[1])
		if src == "" || isRemoteRef(src) {
			return tag
		}
		// Only plain links to image files become attachment downloads;
		// links to other targets are left for Confluence to render as-is.
		if _, ok := imageExtensions[imageExtOf(src)]; !ok {
			return tag
		}

		data, base, ok := loadImage(src)
		if !ok {
			if logger != nil {
				logger.Printf("sync images: %s links to %q which could not be read; left as-is", relPath, src)
			}
			return tag
		}
		collect(base, data)
		return attachmentLinkMacro(base, stripTags(m[2]))
	}

	rewriteSegment := func(seg string) string {
		seg = imgTagRe.ReplaceAllStringFunc(seg, rewriteImg)
		seg = aLinkRe.ReplaceAllStringFunc(seg, rewriteLink)
		return seg
	}

	out := mapOutsideCDATA(storage, rewriteSegment)

	if len(order) == 0 {
		return out, nil, nil
	}
	images := make([]ImageRef, 0, len(order))
	for _, base := range order {
		images = append(images, ImageRef{Filename: base, Data: collected[base]})
	}
	return out, images, nil
}

// imageMacro renders the Confluence storage-format image macro for an
// attachment by filename, carrying the alt text when present.
func imageMacro(filename, alt string) string {
	if alt != "" {
		return fmt.Sprintf(`<ac:image ac:alt="%s"><ri:attachment ri:filename="%s" /></ac:image>`,
			xmlEscapeAttr(alt), xmlEscapeAttr(filename))
	}
	return fmt.Sprintf(`<ac:image><ri:attachment ri:filename="%s" /></ac:image>`, xmlEscapeAttr(filename))
}

// resolveImagePath resolves a markdown image src against the source file's
// directory, returning a cleaned forward-slash fsys path. The src is
// percent-decoded first so "my%20image.png" maps to the on-disk filename.
func resolveImagePath(baseDir, src string) string {
	if decoded, err := url.PathUnescape(src); err == nil {
		src = decoded
	}
	src = strings.ReplaceAll(src, "\\", "/")
	// Drop any query/fragment a markdown author might have appended.
	if i := strings.IndexAny(src, "?#"); i >= 0 {
		src = src[:i]
	}
	src = strings.TrimPrefix(src, "./")
	if strings.HasPrefix(src, "/") {
		// Absolute-from-root reference: treat as sync-root-relative.
		return path.Clean(strings.TrimPrefix(src, "/"))
	}
	return path.Clean(path.Join(baseDir, src))
}

// repoImagePath resolves a markdown image/link src to a repo-root-relative
// slash path, by joining the source page's repo-relative directory
// (repoRel + baseDir) with src. This lets references that escape the --from
// root (../../diagrams/x.svg) resolve against the wider repository, mirroring
// how the plantuml/git-file rewriters resolve their targets. An absolute
// "/foo" src is treated as repo-root-relative.
func repoImagePath(repoRel, baseDir, src string) string {
	if decoded, err := url.PathUnescape(src); err == nil {
		src = decoded
	}
	src = strings.ReplaceAll(src, "\\", "/")
	if i := strings.IndexAny(src, "?#"); i >= 0 {
		src = src[:i]
	}
	src = strings.TrimPrefix(src, "./")
	if strings.HasPrefix(src, "/") {
		return path.Clean(strings.TrimPrefix(src, "/"))
	}
	return path.Clean(path.Join(repoRel, baseDir, src))
}

// escapesRoot reports whether a cleaned slash path points at or above its
// filesystem root (".." or "../..."), which io/fs rejects and which signals
// the reference left the tree we can read.
func escapesRoot(p string) bool {
	return p == ".." || p == "." || strings.HasPrefix(p, "../")
}

// imageExtOf returns the lowercase extension (with dot) of a markdown
// image/link src, after decoding and stripping any query/fragment.
func imageExtOf(src string) string {
	if decoded, err := url.PathUnescape(src); err == nil {
		src = decoded
	}
	if i := strings.IndexAny(src, "?#"); i >= 0 {
		src = src[:i]
	}
	return strings.ToLower(path.Ext(src))
}

// attachmentLinkMacro renders a Confluence link to an uploaded attachment by
// filename, carrying the link text as a plain-text body. Empty text falls back
// to the filename so the link is never blank.
func attachmentLinkMacro(filename, text string) string {
	body := strings.TrimSpace(text)
	if body == "" {
		body = filename
	}
	return fmt.Sprintf(
		`<ac:link><ri:attachment ri:filename="%s" /><ac:plain-text-link-body><![CDATA[%s]]></ac:plain-text-link-body></ac:link>`,
		xmlEscapeAttr(filename), cdataSafe(body),
	)
}

// stripTags removes any HTML tags from goldmark-rendered link inner content,
// leaving plain text suitable for a plain-text-link-body.
func stripTags(s string) string {
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, ""))
}

// cdataSafe guards a string against a premature ]]> terminator by splitting
// the sequence across a CDATA boundary.
func cdataSafe(s string) string {
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}

// isRemoteRef reports whether src is a URL confcli should leave untouched
// (already-hosted images), rather than a local file to upload.
func isRemoteRef(src string) bool {
	lower := strings.ToLower(src)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "//") ||
		strings.HasPrefix(lower, "data:") ||
		strings.Contains(lower, "://")
}

// imageFingerprint returns a deterministic digest of a page's images, folded
// into the content hash so that editing an image (without touching the
// markdown) still triggers a re-sync. Order-independent: sorted by filename.
func imageFingerprint(images []ImageRef) string {
	if len(images) == 0 {
		return ""
	}
	sorted := make([]ImageRef, len(images))
	copy(sorted, images)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Filename < sorted[j].Filename })

	var b strings.Builder
	for _, img := range sorted {
		sum := sha1.Sum(img.Data)
		b.WriteString(img.Filename)
		b.WriteByte(0)
		b.WriteString(hex.EncodeToString(sum[:]))
		b.WriteByte('\n')
	}
	return b.String()
}

// imageMime returns the MIME type for an attachment filename, used so
// Confluence renders the image inline. Falls back to a small built-in table
// for types the stdlib mime database may miss, then to octet-stream.
func imageMime(filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "application/octet-stream"
	}
}

// mapOutsideCDATA applies fn to the parts of s that are outside
// <![CDATA[...]]> sections, leaving CDATA bodies verbatim. Mirrors the
// CDATA-skipping logic in converter.go's selfCloseVoidTags/escapeUnknownTags.
func mapOutsideCDATA(s string, fn func(string) string) string {
	var b strings.Builder
	b.Grow(len(s))
	rest := s
	for {
		start := strings.Index(rest, "<![CDATA[")
		if start < 0 {
			b.WriteString(fn(rest))
			break
		}
		b.WriteString(fn(rest[:start]))
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

// xmlEscapeAttr escapes a string for use inside a double-quoted XML attribute.
func xmlEscapeAttr(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
