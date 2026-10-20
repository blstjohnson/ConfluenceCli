package sync

import (
	"confcli/pkg/converters"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

func addTableOfContents(source []byte, storage string, minHeadings int) (string, error) {
	if minHeadings <= 0 {
		return storage, nil
	}
	count := 0
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(source))
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, ok := n.(*ast.Heading); ok {
				count++
			}
		}
		return ast.WalkContinue, nil
	})
	return converters.WithLeadingTableOfContents(storage, count >= minHeadings)
}
