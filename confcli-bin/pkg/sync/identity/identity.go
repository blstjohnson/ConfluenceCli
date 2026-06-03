// Package identity provides the two-label identity scheme used by `confcli sync`
// to anchor and change-detect synced pages.
//
// Each synced Confluence page carries two global labels:
//
//   - confcli-id-<sha1(normalized_relative_path)>  — stable identity anchor
//   - confcli-hash-<sha1(title \x00 storage_payload)> — change detector
//
// The id label survives content edits and is the lookup key for "is this
// markdown file already on the server?". The hash label survives identity
// changes and is the "no-op skip" detector for "would re-uploading actually
// change anything?".
package identity

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"golang.org/x/text/unicode/norm"

	"confcli/pkg/models"
)

// Label prefixes. Both use hyphen separators (colons are unreliable across
// Confluence Server/DC/Cloud label validation rules).
const (
	IDLabelPrefix   = "confcli-id-"
	HashLabelPrefix = "confcli-hash-"
)

// NormalizePath returns the canonical form of a sync-root-relative path used
// as identity-hash input. Rules: forward slashes, lowercased, unicode NFC.
// The .md extension (if any) is preserved.
//
// Lowercasing is required so the same file produces the same identity on
// Windows (case-insensitive filesystem) and Linux/macOS (case-sensitive).
// NFC ensures non-ASCII filenames (e.g. composed vs decomposed accents)
// don't yield divergent identities depending on the editor or filesystem.
func NormalizePath(relPath string) string {
	p := strings.ReplaceAll(relPath, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	p = strings.ToLower(p)
	return norm.NFC.String(p)
}

// BuildIDLabel returns the confcli-id label for a markdown file at relPath,
// where relPath is relative to the sync root (--from directory).
func BuildIDLabel(relPath string) string {
	sum := sha1.Sum([]byte(NormalizePath(relPath)))
	return IDLabelPrefix + hex.EncodeToString(sum[:])
}

// BuildHashLabel returns the confcli-hash label for a page that would be
// published with the given title, storage-format payload, and image
// fingerprint. The components are joined by NUL bytes to avoid boundary
// ambiguity (no component can legitimately contain a NUL).
//
// imageFingerprint folds the bytes of the page's local images into the hash
// so that editing an image (without touching the markdown) still changes the
// page hash and triggers a re-sync. Pass "" for pages with no local images.
func BuildHashLabel(title, storagePayload, imageFingerprint string) string {
	h := sha1.New()
	h.Write([]byte(title))
	h.Write([]byte{0})
	h.Write([]byte(storagePayload))
	h.Write([]byte{0})
	h.Write([]byte(imageFingerprint))
	return HashLabelPrefix + hex.EncodeToString(h.Sum(nil))
}

// ExtractIDLabel returns the confcli-id label found among labels, or "" if
// none is present. If multiple are present (an inconsistent server state),
// the first one is returned.
func ExtractIDLabel(labels []models.Label) string {
	return findByPrefix(labels, IDLabelPrefix)
}

// ExtractHashLabel returns the confcli-hash label found among labels, or ""
// if none is present.
func ExtractHashLabel(labels []models.Label) string {
	return findByPrefix(labels, HashLabelPrefix)
}

func findByPrefix(labels []models.Label, prefix string) string {
	for _, l := range labels {
		if strings.HasPrefix(l.Name, prefix) {
			return l.Name
		}
	}
	return ""
}

// CQLFilter returns the CQL fragment that locates a page by its id-label
// within a space. Quotes spaceKey defensively; relPath is normalized.
func CQLFilter(spaceKey, relPath string) string {
	return `label = "` + BuildIDLabel(relPath) + `" AND space = "` + spaceKey + `" AND type = "page"`
}
