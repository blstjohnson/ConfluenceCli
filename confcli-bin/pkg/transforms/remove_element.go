package transforms

import (
	"fmt"
	"regexp"
	"strings"
)

// RemoveElement strips HTML/storage elements matching CSS-like selectors from PreContent.
// Supported selector forms:
//   - "tagname"           — matches <tagname ...>...</tagname>
//   - "tagname.classname" — matches <tagname class="...classname...">...</tagname>
//   - ".classname"        — matches any element with class="...classname..."
//   - "tagname#id"        — matches <tagname id="id">...</tagname>
type RemoveElement struct {
	Selectors []string
	matchers  []elementMatcher
}

type elementMatcher struct {
	tag   string         // empty means "any tag"
	class string         // empty means "no class constraint"
	id    string         // empty means "no id constraint"
	re    *regexp.Regexp // compiled regex to find matching opening tags
}

// NewRemoveElement creates a RemoveElement transform for the given selectors.
func NewRemoveElement(selectors ...string) (*RemoveElement, error) {
	matchers := make([]elementMatcher, len(selectors))
	for i, sel := range selectors {
		m, err := parseSelector(sel)
		if err != nil {
			return nil, fmt.Errorf("invalid selector %q: %w", sel, err)
		}
		matchers[i] = m
	}
	return &RemoveElement{Selectors: selectors, matchers: matchers}, nil
}

func (r *RemoveElement) Name() string {
	return "remove/element"
}

func (r *RemoveElement) Apply(ctx *TransformContext) error {
	for _, m := range r.matchers {
		ctx.PreContent = removeMatchingElements(ctx.PreContent, m)
	}
	return nil
}

func parseSelector(sel string) (elementMatcher, error) {
	var m elementMatcher

	// Handle #id
	if idx := strings.Index(sel, "#"); idx != -1 {
		m.tag = sel[:idx]
		m.id = sel[idx+1:]
	} else if idx := strings.Index(sel, "."); idx != -1 {
		m.tag = sel[:idx]
		m.class = sel[idx+1:]
	} else {
		m.tag = sel
	}

	// Build regex for matching opening tags
	pattern := "<"
	if m.tag != "" {
		pattern += regexp.QuoteMeta(m.tag)
	} else {
		pattern += `(\w[\w:-]*)`
	}
	pattern += `[^>]*`

	if m.class != "" {
		pattern += `class="[^"]*\b` + regexp.QuoteMeta(m.class) + `\b[^"]*"[^>]*`
	}
	if m.id != "" {
		pattern += `id="` + regexp.QuoteMeta(m.id) + `"[^>]*`
	}
	pattern += ">"

	re, err := regexp.Compile(pattern)
	if err != nil {
		return m, err
	}
	m.re = re

	return m, nil
}

// removeMatchingElements removes all elements matching the given matcher.
func removeMatchingElements(content string, m elementMatcher) string {
	var b strings.Builder
	remaining := content

	for {
		loc := m.re.FindStringIndex(remaining)
		if loc == nil {
			b.WriteString(remaining)
			break
		}

		b.WriteString(remaining[:loc[0]])

		// Determine the tag name for finding the closing tag
		tagName := m.tag
		if tagName == "" {
			// Extract from the match
			sub := m.re.FindStringSubmatch(remaining[loc[0]:])
			if len(sub) >= 2 {
				tagName = sub[1]
			}
		}

		// Check for self-closing tag
		matched := remaining[loc[0]:loc[1]]
		if strings.HasSuffix(matched, "/>") {
			remaining = remaining[loc[1]:]
			continue
		}

		// Find closing tag
		closeIdx := findMatchingClose(remaining[loc[0]:], tagName)
		if closeIdx == -1 {
			// No closing tag — remove just the opening tag
			remaining = remaining[loc[1]:]
		} else {
			remaining = remaining[loc[0]+closeIdx:]
		}
	}

	return b.String()
}
