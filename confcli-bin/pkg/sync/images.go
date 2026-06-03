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
)

// rewriteImages turns local <img> tags in the storage payload into Confluence
// <ac:image><ri:attachment .../></ac:image> macros and collects the referenced
// images (read from fsys) so they can be uploaded to the page.
//
// relPath is the sync-root-relative path of the source markdown file; image
// srcs are resolved relative to its directory. Remote images (http/https/
// protocol-relative/data URIs) are left untouched. A local src that cannot be
// read, or whose extension is not a known image type, is left as-is and logged
// (pass a nil logger to silence). Content inside <![CDATA[...]]> (code blocks)
// is passed through verbatim so literal <img> text in code samples survives.
//
// Returned images are deduplicated by basename; the first occurrence's bytes
// win. A basename collision between two different source files on the same
// page is logged.
func rewriteImages(storage, relPath string, fsys fs.FS, logger *log.Logger) (string, []ImageRef, error) {
	if !strings.Contains(strings.ToLower(storage), "<img") {
		return storage, nil, nil
	}

	baseDir := path.Dir(relPath)
	if baseDir == "." {
		baseDir = ""
	}

	collected := map[string][]byte{} // basename → bytes
	var order []string               // basenames in first-seen order

	rewriteSegment := func(seg string) string {
		return imgTagRe.ReplaceAllStringFunc(seg, func(tag string) string {
			srcMatch := imgSrcRe.FindStringSubmatch(tag)
			if srcMatch == nil {
				return tag
			}
			src := strings.TrimSpace(srcMatch[1])
			if src == "" || isRemoteRef(src) {
				return tag
			}

			fsPath := resolveImagePath(baseDir, src)
			ext := strings.ToLower(path.Ext(fsPath))
			if _, ok := imageExtensions[ext]; !ok {
				if logger != nil {
					logger.Printf("sync images: %s references %q with unsupported extension %q; left as-is", relPath, src, ext)
				}
				return tag
			}

			data, err := fs.ReadFile(fsys, fsPath)
			if err != nil {
				if logger != nil {
					logger.Printf("sync images: %s references %q which could not be read (%v); left as-is", relPath, src, err)
				}
				return tag
			}

			base := path.Base(fsPath)
			if existing, seen := collected[base]; !seen {
				collected[base] = data
				order = append(order, base)
			} else if logger != nil && !bytesEqual(existing, data) {
				logger.Printf("sync images: %s references two different files named %q; keeping the first", relPath, base)
			}

			alt := ""
			if m := imgAltRe.FindStringSubmatch(tag); m != nil {
				alt = m[1]
			}
			return imageMacro(base, alt)
		})
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
