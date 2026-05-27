// Package md converts Markdown to Confluence storage format (XHTML).
//
// Standard Markdown elements (headings, paragraphs, lists, tables, inline
// formatting) pass through goldmark's HTML renderer unchanged because the
// Confluence storage format accepts well-formed XHTML. Only nodes that
// require Confluence-specific output are overridden:
//
//   - Fenced code blocks render as ac:structured-macro "code" with the
//     language parameter set from the fence info string.
//   - GFM alerts (> [!NOTE], [!TIP], [!IMPORTANT], [!WARNING], [!CAUTION])
//     render as Confluence panel macros (info / note / warning).
package md

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

var defaultMarkdown = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		&confluenceExtension{},
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		html.WithUnsafe(),
		// Confluence storage format is XHTML-strict and rejects HTML5
		// void tags like <hr> or <br>. WithXHTML emits the self-closing
		// form (<hr />, <br />, <img ... />) the parser expects.
		html.WithXHTML(),
	),
)

// ToStorage converts Markdown bytes to Confluence storage format (XHTML).
// Empty input returns an empty string.
func ToStorage(markdown []byte) (string, error) {
	if len(markdown) == 0 {
		return "", nil
	}
	var buf bytes.Buffer
	if err := defaultMarkdown.Convert(markdown, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
