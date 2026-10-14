package markdownutil

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// MarkdownOutsideCode applies a rewrite only outside code spans and blocks.
// Parsing fences avoids treating a shorter fence inside an example as its end.
func MarkdownOutsideCode(source string, rewrite func(string) string) string {
	return markdownParts(source, rewrite, func(s string) string { return s })
}

// MaskCode retains byte offsets and line breaks while hiding code examples.
func MaskCode(source string) string {
	return markdownParts(source, func(s string) string { return s }, func(s string) string {
		b := []byte(s)
		for i := range b {
			if b[i] != '\n' && b[i] != '\r' {
				b[i] = ' '
			}
		}
		return string(b)
	})
}

func markdownParts(source string, rewrite, code func(string) string) string {
	type span struct{ start, end int }
	var spans []span
	data := []byte(source)
	doc := goldmark.DefaultParser().Parse(text.NewReader(data))
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindFencedCodeBlock, ast.KindCodeBlock:
			lines := n.Lines()
			if lines.Len() > 0 {
				spans = append(spans, span{lines.At(0).Start, lines.At(lines.Len() - 1).Stop})
			}
			return ast.WalkSkipChildren, nil
		case ast.KindCodeSpan:
			start, end := len(data), 0
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				if t, ok := c.(*ast.Text); ok {
					if t.Segment.Start < start {
						start = t.Segment.Start
					}
					if t.Segment.Stop > end {
						end = t.Segment.Stop
					}
				}
			}
			if end > start {
				spans = append(spans, span{start, end})
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var b strings.Builder
	pos := 0
	for _, s := range spans {
		if s.start < pos {
			continue
		}
		b.WriteString(rewrite(source[pos:s.start]))
		b.WriteString(code(source[s.start:s.end]))
		pos = s.end
	}
	b.WriteString(rewrite(source[pos:]))
	return b.String()
}

// Mask code while retaining line boundaries for table/fence-aware rewriters.
func ProtectCode(source string) (string, func(string) string) {
	prefix := fmt.Sprintf("CONFCLICODE%x", sha256.Sum256([]byte(source)))
	for strings.Contains(source, prefix) {
		prefix += "X"
	}
	var originals, tokens []string
	masked := markdownParts(source, func(s string) string { return s }, func(s string) string {
		token := fmt.Sprintf("%sN%dZ", prefix, len(originals)) + strings.Repeat("\n", strings.Count(s, "\n"))
		originals = append(originals, s)
		tokens = append(tokens, token)
		return token
	})
	return masked, func(s string) string {
		for i, token := range tokens {
			s = strings.ReplaceAll(s, token, originals[i])
		}
		return s
	}
}
