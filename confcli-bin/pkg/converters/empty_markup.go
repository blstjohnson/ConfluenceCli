package converters

import (
	"regexp"
	"strings"

	"confcli/pkg/markdownutil"
)

var emptyInlineMarkup = regexp.MustCompile(`(?i)<(span|em|strong|b|i|u|s|del)\s*>([\t\r\n ]*)</(span|em|strong|b|i|u|s|del)\s*>`)
var emptyMacroMarkup = regexp.MustCompile(`(?m)^[\t ]*<!-- (expand|panel|note|info|warning|tip)-start: ([^\r\n]*?) -->[\t\r\n ]*<!-- (expand|panel|note|info|warning|tip)-end -->[\t ]*(?:\r?\n)?`)
var indentedBlankLine = regexp.MustCompile(`(?m)^[\t ]+(\r?\n)`)

func isTOCTitle(title string) bool {
	switch strings.ToLower(strings.Join(strings.Fields(title), " ")) {
	case "оглавление", "содержание", "table of contents", "contents":
		return true
	}
	return false
}

// CleanEmptyMarkdownMarkup removes only empty presentation markup. Whitespace
// inside inline tags is retained, so adjacent words cannot be joined. Tags with
// attributes (especially id/name), code examples, macro metadata and filled
// macro bodies are left intact. A meaningful title is content of its own.
func CleanEmptyMarkdownMarkup(source string) string {
	return markdownutil.MarkdownOutsideCode(source, func(s string) string {
		// Nested retained HTML lists can leave indentation on otherwise blank
		// lines. Keep real line endings, hard breaks and code samples intact.
		s = indentedBlankLine.ReplaceAllString(s, "$1")
		for {
			clean := emptyInlineMarkup.ReplaceAllStringFunc(s, func(match string) string {
				m := emptyInlineMarkup.FindStringSubmatch(match)
				if strings.EqualFold(m[1], m[3]) {
					return m[2]
				}
				return match
			})
			if clean == s {
				break
			}
			s = clean
		}
		return emptyMacroMarkup.ReplaceAllStringFunc(s, func(match string) string {
			m := emptyMacroMarkup.FindStringSubmatch(match)
			if m[1] == m[3] && (strings.TrimSpace(m[2]) == "" || (m[1] == "expand" && isTOCTitle(m[2]))) {
				return ""
			}
			return match
		})
	})
}
