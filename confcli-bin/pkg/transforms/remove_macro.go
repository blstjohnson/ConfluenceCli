package transforms

import (
	"fmt"
	"regexp"
	"strings"
)

// RemoveMacro strips Confluence structured macros by name from PreContent.
// Macro names are matched against the ac:name attribute of <ac:structured-macro> tags.
type RemoveMacro struct {
	// MacroNames is a list of macro names to remove. Each entry is treated as a
	// regex pattern matched against the ac:name attribute value.
	MacroNames []string

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
	return &RemoveMacro{MacroNames: macroNames, compiled: compiled}, nil
}

func (r *RemoveMacro) Name() string {
	return "remove/macro"
}

func (r *RemoveMacro) Apply(ctx *TransformContext) error {
	ctx.PreContent = r.removeMacros(ctx.PreContent)
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
