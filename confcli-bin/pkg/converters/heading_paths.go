package converters

import (
	"crypto/sha256"
	"fmt"
	"html"
	"regexp"
	"strings"

	"confcli/pkg/markdownutil"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type headingLocation struct {
	text  string
	level int
}

// Obsidian compares heading text without these punctuation characters. In
// particular, punctuation-only differences do not make a heading unique.
func normalizeHeading(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune("!\"#$%&()*+,.:;<=>?@^`{|}~/[]\\\r\n", r) {
			return ' '
		}
		return r
	}, s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func headingPathText(s string) string {
	s = strings.NewReplacer("%%", " ", "[[", " ", "]]", " ").Replace(s)
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(":#|^\\\r\n", r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// Resolve exactly as Obsidian's heading subpath resolver: each component must
// match a later heading at a deeper level. Checking only existence of the last
// component incorrectly accepts links to an earlier occurrence.
func resolveHeadingPath(headings []headingLocation, fragment string) int {
	parts := strings.FieldsFunc(fragment, func(r rune) bool { return r == '#' })
	if len(parts) == 0 {
		return -1
	}
	part, level := 0, 0
	for i, h := range headings {
		if h.level > level && normalizeHeading(h.text) == normalizeHeading(parts[part]) {
			part++
			level = h.level
			if part == len(parts) {
				return i
			}
		}
	}
	return -1
}

func headingFragments(headings []headingLocation) []string {
	result := make([]string, len(headings))
	counts := map[string]int{}
	for _, h := range headings {
		counts[normalizeHeading(h.text)]++
	}
	var parents []int
	for i, h := range headings {
		for len(parents) > 0 && headings[parents[len(parents)-1]].level >= h.level {
			parents = parents[:len(parents)-1]
		}
		fragment := headingPathText(h.text)
		if counts[normalizeHeading(h.text)] == 1 && resolveHeadingPath(headings, fragment) == i {
			result[i] = fragment
		} else {
			for p := len(parents) - 1; p >= 0; p-- {
				fragment = headingPathText(headings[parents[p]].text) + "#" + fragment
				if resolveHeadingPath(headings, fragment) == i {
					result[i] = fragment
					break
				}
			}
			// The first of indistinguishable sibling headings is still reachable.
			if result[i] == "" && resolveHeadingPath(headings, headingPathText(h.text)) == i {
				result[i] = headingPathText(h.text)
			}
			if result[i] == "" {
				// No heading path can select this occurrence (for example, an
				// earlier child heading has the same name as a later sibling).
				result[i] = fmt.Sprintf("^confcli-%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%s", i, h.text))))
			}
		}
		parents = append(parents, i)
	}
	return result
}

// PrepareMarkdownHeadingAnchors gives nested Obsidian heading paths stable
// Confluence anchor macros on import. The temporary <a> elements are internal
// conversion input only; exported Markdown contains no generated anchor tags.
func PrepareMarkdownHeadingAnchors(source []byte) (string, map[string]string) {
	doc := goldmark.New().Parser().Parse(text.NewReader(source))
	var headings []headingLocation
	var offsets []int
	var ends []int
	var explicitIDs []string
	blockID := regexp.MustCompile(`\s+\^([A-Za-z0-9-]+)\s*$`)
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if h, ok := n.(*ast.Heading); ok {
			if h.Lines().Len() == 0 {
				return ast.WalkContinue, nil
			}
			var b strings.Builder
			_ = ast.Walk(h, func(child ast.Node, enter bool) (ast.WalkStatus, error) {
				if enter {
					switch t := child.(type) {
					case *ast.Text:
						b.Write(t.Value(source))
					case *ast.String:
						b.Write(t.Value)
					}
				}
				return ast.WalkContinue, nil
			})
			title := html.UnescapeString(b.String())
			id := ""
			if match := blockID.FindStringSubmatch(title); match != nil {
				id = match[1]
				title = title[:len(title)-len(match[0])]
			}
			headings = append(headings, headingLocation{title, h.Level})
			explicitIDs = append(explicitIDs, id)
			segment := h.Lines().At(0)
			offsets = append(offsets, strings.LastIndexByte(string(source[:segment.Start]), '\n')+1)
			last := h.Lines().At(h.Lines().Len() - 1)
			ends = append(ends, last.Stop)
		}
		return ast.WalkContinue, nil
	})
	fragments := headingFragments(headings)
	anchors := legacyHeadingAnchors(source, headings, offsets, ends, fragments)
	out := string(source)
	for i := len(fragments) - 1; i >= 0; i-- {
		fragment := fragments[i]
		var prefix strings.Builder
		if id := explicitIDs[i]; id != "" {
			anchors["^"+id] = id
			prefix.WriteString(`<a id="` + id + `"></a>` + "\n\n")
			// Block IDs are navigation metadata, not part of the visible title.
			if match := blockID.FindStringIndex(out[offsets[i]:ends[i]]); match != nil {
				out = out[:offsets[i]+match[0]] + out[offsets[i]+match[1]:]
			}
		}
		if strings.Contains(fragment, "#") || strings.HasPrefix(fragment, "^") {
			if anchors[fragment] != "" {
				out = out[:offsets[i]] + prefix.String() + out[offsets[i]:]
				continue
			}
			id := strings.TrimPrefix(fragment, "^")
			if !strings.HasPrefix(fragment, "^") {
				id = fmt.Sprintf("confcli-heading-%x", sha256.Sum256([]byte(fragment)))
			}
			anchors[fragment] = id
			if id != explicitIDs[i] {
				prefix.WriteString(`<a id="` + id + `"></a>` + "\n\n")
			}
		}
		out = out[:offsets[i]] + prefix.String() + out[offsets[i]:]
	}
	return out, anchors
}

// Older Markdown exports can contain real HTML anchor targets inside or just
// before a heading. Confluence includes inline anchor names in the heading's
// DOM id, so a link to visible heading text alone can miss that old target.
// Reuse the existing named target while keeping heading links in Markdown.
func legacyHeadingAnchors(source []byte, headings []headingLocation, starts, ends []int, fragments []string) map[string]string {
	result := map[string]string{}
	masked := markdownutil.MaskCode(string(source))
	pattern := regexp.MustCompile(`<a\s+(?:id|name)="([^"]+)"\s*></a>`)
	owners, inline := map[int]string{}, map[int]bool{}
	counts := map[string]int{}
	for _, span := range pattern.FindAllStringSubmatchIndex(masked, -1) {
		name := html.UnescapeString(masked[span[2]:span[3]])
		counts[name]++
		owner, inHeading := -1, false
		for i := range headings {
			if starts[i] <= span[0] && span[0] < ends[i] {
				owner, inHeading = i, true
				break
			}
			if starts[i] >= span[1] {
				gap := pattern.ReplaceAllString(masked[span[1]:starts[i]], "")
				if strings.TrimSpace(gap) == "" {
					owner = i
				}
				break
			}
			owner = i
		}
		if owner >= 0 && (owners[owner] == "" || (inHeading && !inline[owner])) {
			owners[owner], inline[owner] = name, inHeading
		}
	}
	for i, anchor := range owners {
		if fragments[i] != "" && counts[anchor] == 1 {
			result[fragments[i]] = anchor
		}
	}
	return result
}
