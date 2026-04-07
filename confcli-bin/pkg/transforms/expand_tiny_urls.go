package transforms

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"confcli/pkg/converters"
)

// TinyURLResolver resolves a Confluence tiny URL ID (e.g. "AbCd") to a
// canonical page URL path (e.g. "/pages/viewpage.action?pageId=12345").
// Returns empty string if the tiny URL cannot be resolved.
type TinyURLResolver func(tinyID string) string

// ExpandTinyURLs is a pre-conversion transform that finds Confluence tiny URLs
// (/x/AbCd) in HTML content and replaces them with canonical page URLs.
// This enables the downstream link rewriter to match and rewrite them properly.
//
// Confluence generates short URLs like /x/AbCd that are opaque redirects.
// Without expansion, the link rewriter cannot match them to pages in the
// exported space because the URLs lack a recognisable pageId parameter and
// may be relative (missing the base URL entirely).
type ExpandTinyURLs struct {
	Resolver    TinyURLResolver
	ConfBaseURL string
}

// NewExpandTinyURLs creates an ExpandTinyURLs transform.
// confBaseURL is the Confluence instance base URL (e.g. "https://confluence.example.com").
// resolver maps tiny URL IDs to canonical URL paths.
func NewExpandTinyURLs(confBaseURL string, resolver TinyURLResolver) *ExpandTinyURLs {
	return &ExpandTinyURLs{
		ConfBaseURL: confBaseURL,
		Resolver:    resolver,
	}
}

func (e *ExpandTinyURLs) Name() string {
	return "expand/tiny-urls"
}

// hrefTinyURLRe matches href attributes containing /x/<tinyID> tiny URL patterns.
// Captures:
//  1. href attribute prefix including opening quote (e.g. `href="`)
//  2. URL prefix before /x/ (may be empty for relative URLs)
//  3. tiny URL ID (e.g. "AbCd")
//  4. optional #anchor or %23anchor
//  5. closing quote
var hrefTinyURLRe = regexp.MustCompile(
	`(href\s*=\s*["'])` + // group 1: href="
		`([^"']*?)` + // group 2: URL prefix before /x/
		`/x/([A-Za-z0-9_-]{4,12})` + // group 3: tiny URL ID
		`((?:[#%][^"']*)?)` + // group 4: optional anchor/fragment
		`(["'])`, // group 5: closing quote
)

func (e *ExpandTinyURLs) Apply(ctx *TransformContext) error {
	if e.Resolver == nil {
		return nil
	}

	ctx.PreContent = hrefTinyURLRe.ReplaceAllStringFunc(ctx.PreContent, func(match string) string {
		sub := hrefTinyURLRe.FindStringSubmatch(match)
		if len(sub) < 6 {
			return match
		}
		prefix := sub[1]    // href="
		urlPrefix := sub[2] // URL part before /x/
		tinyID := sub[3]    // e.g. AbCd
		anchor := sub[4]    // optional #anchor
		quote := sub[5]     // closing "

		canonical := e.Resolver(tinyID)
		if canonical == "" {
			return match
		}

		// Preserve the URL prefix. If the original was absolute
		// (e.g. https://confluence.example.com/x/AbCd), keep the host part.
		// If it was relative (/x/AbCd), produce an absolute URL with the
		// configured base so the link rewriter can match it.
		var newURL string
		if strings.Contains(urlPrefix, "://") {
			// Absolute URL — keep the original scheme+host prefix
			newURL = urlPrefix + canonical
		} else if e.ConfBaseURL != "" {
			// Relative URL — prepend base URL for link rewriter matching
			newURL = strings.TrimRight(e.ConfBaseURL, "/") + canonical
		} else {
			newURL = canonical
		}

		return prefix + newURL + anchor + quote
	})

	return nil
}

// DecodingResolver creates a TinyURLResolver that uses client-side base64
// decoding to resolve tiny URLs to canonical page URL paths.
// This is fast and requires no API calls, but does not validate that the
// decoded page ID actually exists.
func DecodingResolver() TinyURLResolver {
	return func(tinyID string) string {
		pageID, err := converters.DecodeTinyURL(tinyID)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("/pages/viewpage.action?pageId=%d", pageID)
	}
}

// CachingResolver wraps a TinyURLResolver with a thread-safe cache so that
// each unique tiny URL ID is resolved at most once. This is useful when the
// same tiny URL appears across multiple pages in a space export.
func CachingResolver(inner TinyURLResolver) TinyURLResolver {
	var mu sync.Mutex
	cache := make(map[string]string)
	return func(tinyID string) string {
		mu.Lock()
		if v, ok := cache[tinyID]; ok {
			mu.Unlock()
			return v
		}
		mu.Unlock()

		result := inner(tinyID)

		mu.Lock()
		cache[tinyID] = result
		mu.Unlock()
		return result
	}
}

// VerifyingResolver creates a TinyURLResolver that decodes the tiny URL
// client-side and then verifies the page exists using the provided checker.
// pageExists should return true if a page with the given ID is known to exist
// (e.g. present in the export page map or accessible via the Confluence API).
func VerifyingResolver(pageExists func(pageID int) bool) TinyURLResolver {
	return func(tinyID string) string {
		pageID, err := converters.DecodeTinyURL(tinyID)
		if err != nil {
			return ""
		}
		id := int(pageID)
		if pageExists != nil && !pageExists(id) {
			return ""
		}
		return fmt.Sprintf("/pages/viewpage.action?pageId=%d", id)
	}
}
