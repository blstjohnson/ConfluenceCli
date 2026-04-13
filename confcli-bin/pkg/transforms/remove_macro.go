package transforms

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// RemoveMacro strips Confluence structured macros by name from PreContent.
// Macro names are matched against the ac:name attribute of <ac:structured-macro> tags.
// For expand macros, the content inside ac:rich-text-body is preserved.
type RemoveMacro struct {
	// MacroNames is a list of macro names to remove. Each entry is treated as a
	// regex pattern matched against the ac:name attribute value.
	MacroNames []string

	// PreserveContent indicates whether to preserve the content inside macros
	// (e.g., the text inside ac:rich-text-body or ac:plain-text-body).
	// This is particularly useful for expand macros where you want to keep
	// the expanded content.
	PreserveContent bool

	compiled []*regexp.Regexp
}

// NewRemoveMacro creates a RemoveMacro transform for the given macro name patterns.
func NewRemoveMacro(macroNames ...string) (*RemoveMacro, error) {
	compiled := make([]*regexp.Regexp, len(macroNames))
	for i, name := range macroNames {
		re, err := regexp.Compile(name)
		if err != nil {
			return nil, fmt.Errorf("invalid macro name pattern %q: %w", name, err)
		}
		compiled[i] = re
	}
	return &RemoveMacro{MacroNames: macroNames, compiled: compiled, PreserveContent: false}, nil
}

// NewRemoveMacroWithContentPreserve creates a RemoveMacro that preserves the
// content inside the removed macros (e.g., expand macro content).
func NewRemoveMacroWithContentPreserve(macroNames ...string) (*RemoveMacro, error) {
	compiled := make([]*regexp.Regexp, len(macroNames))
	for i, name := range macroNames {
		re, err := regexp.Compile(name)
		if err != nil {
			return nil, fmt.Errorf("invalid macro name pattern %q: %w", name, err)
		}
		compiled[i] = re
	}
	return &RemoveMacro{MacroNames: macroNames, compiled: compiled, PreserveContent: true}, nil
}

func (r *RemoveMacro) Name() string {
	return "remove/macro"
}

func (r *RemoveMacro) Apply(ctx *TransformContext) error {
	if r.PreserveContent {
		ctx.PreContent = r.removeMacrosPreserveContent(ctx.PreContent)
	} else {
		ctx.PreContent = r.removeMacros(ctx.PreContent)
	}
	return nil
}

// macroOpenRe matches the opening tag of a structured macro with its ac:name attribute.
var macroOpenRe = regexp.MustCompile(`<ac:structured-macro[^>]*ac:name="([^"]*)"[^>]*>`)

func (r *RemoveMacro) removeMacros(content string) string {
	result := content
	for _, nameRe := range r.compiled {
		result = removeMacrosByPattern(result, nameRe)
	}
	return result
}

// removeMacrosPreserveContent removes macros but preserves their inner content
func (r *RemoveMacro) removeMacrosPreserveContent(content string) string {
	result := content
	for _, nameRe := range r.compiled {
		result = removeMacrosPreserveContentByPattern(result, nameRe)
	}
	return result
}

// removeMacrosPreserveContentByPattern removes all <ac:structured-macro> blocks whose ac:name matches
// the pattern, but preserves the content inside ac:rich-text-body or ac:plain-text-body.
func removeMacrosPreserveContentByPattern(content string, nameRe *regexp.Regexp) string {
	// Parse the HTML fragment
	doc, err := html.Parse(strings.NewReader("<wrap>" + content + "</wrap>"))
	if err != nil {
		// Fallback to simple removal if parsing fails
		return removeMacrosByPattern(content, nameRe)
	}

	// Walk and process macros
	var processMacros func(*html.Node)
	processMacros = func(n *html.Node) {
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if isMacroMatching(child, nameRe) {
				// Move content nodes from ac:rich-text-body or ac:plain-text-body
				// directly into the tree, preserving nested macros as proper DOM elements
				reparentMacroContent(n, child)
				
				// Remove the macro node
				n.RemoveChild(child)
			} else {
				processMacros(child)
			}
			child = next
		}
	}

	processMacros(doc)

	var buf strings.Builder
	if err := html.Render(&buf, doc); err != nil {
		return content
	}

	// Extract content from wrapper
	result := buf.String()
	result = strings.TrimPrefix(result, "<html><head></head><body><wrap>")
	result = strings.TrimSuffix(result, "</wrap></body></html>\n")
	return result
}

// isMacroMatching checks if a node is a structured macro matching the pattern
func isMacroMatching(n *html.Node, nameRe *regexp.Regexp) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if n.Data != "ac:structured-macro" {
		return false
	}
	for _, attr := range n.Attr {
		if attr.Key == "ac:name" {
			return nameRe.MatchString(attr.Val)
		}
	}
	return false
}

// reparentMacroContent moves child nodes from ac:rich-text-body or ac:plain-text-body
// directly into the parent node before the macro. This preserves nested macros as proper
// DOM elements instead of escaped text (which would happen with html.TextNode insertion).
func reparentMacroContent(parent, macro *html.Node) {
	for c := macro.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode &&
			(c.Data == "ac:rich-text-body" || c.Data == "ac:plain-text-body") {
			for inner := c.FirstChild; inner != nil; {
				next := inner.NextSibling
				c.RemoveChild(inner)
				parent.InsertBefore(inner, macro)
				inner = next
			}
			return
		}
	}
}

// removeMacrosByPattern removes all <ac:structured-macro> blocks whose ac:name matches the pattern.
func removeMacrosByPattern(content string, nameRe *regexp.Regexp) string {
	var b strings.Builder
	remaining := content

	for {
		loc := macroOpenRe.FindStringSubmatchIndex(remaining)
		if loc == nil {
			b.WriteString(remaining)
			break
		}

		macroName := remaining[loc[2]:loc[3]]
		if !nameRe.MatchString(macroName) {
			// Not a match — write up to and including the opening tag, continue after it.
			b.WriteString(remaining[:loc[1]])
			remaining = remaining[loc[1]:]
			continue
		}

		// Write everything before this macro
		b.WriteString(remaining[:loc[0]])

		// Find the matching closing tag, handling nesting
		closeIdx := findMatchingClose(remaining[loc[0]:], "ac:structured-macro")
		if closeIdx == -1 {
			// No closing tag found — remove just the opening tag
			remaining = remaining[loc[1]:]
		} else {
			remaining = remaining[loc[0]+closeIdx:]
		}
	}

	return b.String()
}

// findMatchingClose finds the end of the matching closing tag for the given element,
// handling nested elements of the same type. Returns the index past the closing tag
// relative to s, or -1 if not found.
func findMatchingClose(s string, tagName string) int {
	openTag := "<" + tagName
	closeTag := "</" + tagName + ">"
	depth := 0
	i := 0

	for i < len(s) {
		// Check for self-closing tag
		if strings.HasPrefix(s[i:], openTag) {
			end := strings.Index(s[i:], ">")
			if end == -1 {
				return -1
			}
			if s[i+end-1] == '/' {
				// Self-closing tag
				if depth == 0 {
					return i + end + 1
				}
				i += end + 1
				continue
			}
			depth++
			i += end + 1
			continue
		}

		if strings.HasPrefix(s[i:], closeTag) {
			depth--
			if depth == 0 {
				return i + len(closeTag)
			}
			i += len(closeTag)
			continue
		}

		i++
	}

	return -1
}
