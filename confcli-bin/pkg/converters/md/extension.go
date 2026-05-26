package md

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// confluenceExtension wires the Confluence-specific paragraph
// transformer (alert detection) and node renderers (fenced code,
// blockquote-as-panel) into a goldmark.Markdown instance.
type confluenceExtension struct{}

func (e *confluenceExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithParagraphTransformers(
		util.Prioritized(&alertParagraphTransformer{}, 100),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(newConfluenceRenderer(), 100),
	))
}
