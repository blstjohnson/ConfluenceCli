package md

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// confluenceRenderer overrides goldmark's default HTML rendering for
// nodes that require Confluence storage-format output: fenced code
// blocks become "code" structured macros, and blockquotes tagged as
// GFM alerts become panel macros (info / note / warning).
type confluenceRenderer struct{}

func newConfluenceRenderer() renderer.NodeRenderer {
	return &confluenceRenderer{}
}

func (r *confluenceRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
}

// renderFencedCodeBlock emits a Confluence "code" structured macro.
// The fence info string (e.g. "go" in ```go) becomes the language
// parameter; absent info means no language parameter is emitted.
// Code content is wrapped in CDATA to preserve special characters.
func (r *confluenceRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	block := n.(*ast.FencedCodeBlock)

	_, _ = w.WriteString(`<ac:structured-macro ac:name="code">`)
	if lang := block.Language(source); len(lang) > 0 {
		_, _ = w.WriteString(`<ac:parameter ac:name="language">`)
		_, _ = w.Write(lang)
		_, _ = w.WriteString(`</ac:parameter>`)
	}
	_, _ = w.WriteString(`<ac:plain-text-body><![CDATA[`)
	lines := block.Lines()
	var body strings.Builder
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		body.Write(line.Value(source))
	}
	// A literal "]]>" inside the code would close the CDATA early and
	// expose the remainder as raw XHTML, crashing Confluence's parser.
	// Split it across two CDATA sections so the bytes survive verbatim.
	_, _ = w.WriteString(strings.ReplaceAll(body.String(), "]]>", "]]]]><![CDATA[>"))
	_, _ = w.WriteString(`]]></ac:plain-text-body></ac:structured-macro>`)
	return ast.WalkSkipChildren, nil
}

// renderBlockquote emits either a Confluence panel macro (if the
// blockquote was tagged as a GFM alert by alertParagraphTransformer)
// or a plain <blockquote>. The children walk between entering and
// exiting calls renders the body content.
func (r *confluenceRenderer) renderBlockquote(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	bq := n.(*ast.Blockquote)
	raw, hasAlert := bq.AttributeString(alertAttrKey)
	if !hasAlert {
		if entering {
			_, _ = w.WriteString("<blockquote>\n")
		} else {
			_, _ = w.WriteString("</blockquote>\n")
		}
		return ast.WalkContinue, nil
	}
	level := AlertLevel(raw.([]byte))
	if entering {
		_, _ = w.WriteString(`<ac:structured-macro ac:name="`)
		_, _ = w.WriteString(level.confluencePanelMacro())
		_, _ = w.WriteString(`"><ac:rich-text-body>`)
	} else {
		_, _ = w.WriteString(`</ac:rich-text-body></ac:structured-macro>`)
	}
	return ast.WalkContinue, nil
}
