package converters

import (
	"html"
	"strings"
)

// StripStorageMacros edits only the selected source ranges. Keeping the other
// bytes intact preserves CDATA, self-closing references and untouched macros.
// HTML parsers cannot safely rewrite these Confluence storage constructs.
func StripStorageMacros(source string, matches func(string) bool, preserve bool) (string, error) {
	root, err := parseStorage(source)
	if err != nil {
		return "", err
	}
	var render func(*storageNode) string
	rangeContent := func(start, end int, nodes []*storageNode) string {
		var b strings.Builder
		pos := start
		for _, n := range nodes {
			if n.name == "" {
				continue
			}
			b.WriteString(source[pos:n.start])
			b.WriteString(render(n))
			pos = n.end
		}
		b.WriteString(source[pos:end])
		return b.String()
	}
	render = func(n *storageNode) string {
		if n.name == "ac:structured-macro" && matches(n.attr("ac:name")) {
			if !preserve {
				return ""
			}
			for _, c := range n.children {
				if c.name == "ac:rich-text-body" {
					return rangeContent(c.contentStart, c.contentEnd, c.children)
				}
				if c.name == "ac:plain-text-body" {
					return html.EscapeString(c.content())
				}
			}
			return ""
		}
		return rangeContent(n.start, n.end, n.children)
	}
	return rangeContent(0, len(source), root.children), nil
}
