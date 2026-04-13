package transforms

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// DropTableColumn removes columns from HTML tables in PreContent.
// Columns can be identified by zero-based index or by header text regex pattern.
type DropTableColumn struct {
	// Indices are zero-based column positions to drop.
	Indices []int
	// HeaderPatterns are regex patterns matched against header cell text.
	// Any column whose header matches at least one pattern is dropped.
	HeaderPatterns []*regexp.Regexp
}

// NewDropTableColumn creates a DropTableColumn transform.
func NewDropTableColumn(indices []int, headerPatterns []string) (*DropTableColumn, error) {
	compiled := make([]*regexp.Regexp, len(headerPatterns))
	for i, p := range headerPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid header_pattern %q: %w", p, err)
		}
		compiled[i] = re
	}
	return &DropTableColumn{Indices: indices, HeaderPatterns: compiled}, nil
}

func (d *DropTableColumn) Name() string {
	return "drop/table_column"
}

func (d *DropTableColumn) Apply(ctx *TransformContext) error {
	if ctx.PreContent == "" {
		return nil
	}
	// Only process if there's at least one <table
	if !strings.Contains(ctx.PreContent, "<table") {
		return nil
	}

	doc, err := html.Parse(strings.NewReader(ctx.PreContent))
	if err != nil {
		return fmt.Errorf("parse HTML: %w", err)
	}

	tables := findElements(doc, "table")
	for _, table := range tables {
		d.processTable(table)
	}

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return fmt.Errorf("render HTML: %w", err)
	}

	// html.Parse wraps content in <html><head></head><body>...</body></html>.
	// Extract just the body content to preserve the original structure.
	ctx.PreContent = extractBody(buf.String())
	return nil
}

// processTable drops the identified columns from a single table.
func (d *DropTableColumn) processTable(table *html.Node) {
	rows := collectRows(table)
	if len(rows) == 0 {
		return
	}

	// Determine which columns to drop.
	dropSet := make(map[int]bool)
	for _, idx := range d.Indices {
		dropSet[idx] = true
	}

	// If header patterns are specified, scan the first row for header matches.
	if len(d.HeaderPatterns) > 0 {
		headerRow := rows[0]
		cells := collectCells(headerRow)
		for colIdx, cell := range cells {
			text := extractText(cell)
			for _, re := range d.HeaderPatterns {
				if re.MatchString(text) {
					dropSet[colIdx] = true
					break
				}
			}
		}
	}

	if len(dropSet) == 0 {
		return
	}

	// Remove cells at the identified column indices from each row.
	for _, row := range rows {
		removeCellsByIndex(row, dropSet)
	}
}

// collectRows returns all <tr> elements under a table, traversing through
// <thead>, <tbody>, <tfoot> if present.
func collectRows(table *html.Node) []*html.Node {
	var rows []*html.Node
	for child := table.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			switch child.Data {
			case "tr":
				rows = append(rows, child)
			case "thead", "tbody", "tfoot":
				for gc := child.FirstChild; gc != nil; gc = gc.NextSibling {
					if gc.Type == html.ElementNode && gc.Data == "tr" {
						rows = append(rows, gc)
					}
				}
			}
		}
	}
	return rows
}

// collectCells returns all <td> and <th> elements in a row.
func collectCells(row *html.Node) []*html.Node {
	var cells []*html.Node
	for child := row.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
			cells = append(cells, child)
		}
	}
	return cells
}

// removeCellsByIndex removes cells at the given column indices from a row.
func removeCellsByIndex(row *html.Node, dropSet map[int]bool) {
	var toRemove []*html.Node
	colIdx := 0
	for child := row.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
			if dropSet[colIdx] {
				toRemove = append(toRemove, child)
			}
			colIdx++
		}
	}
	for _, node := range toRemove {
		row.RemoveChild(node)
	}
}

// findElements returns all descendant elements with the given tag name.
func findElements(n *html.Node, tag string) []*html.Node {
	var result []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == tag {
			result = append(result, node)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return result
}

// extractText returns the concatenated text content of a node and its descendants.
func extractText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}

// extractBody extracts content from between <body> and </body> tags that
// html.Render wraps around the content.
func extractBody(rendered string) string {
	const openTag = "<body>"
	const closeTag = "</body>"
	start := strings.Index(rendered, openTag)
	if start == -1 {
		return rendered
	}
	start += len(openTag)
	end := strings.LastIndex(rendered, closeTag)
	if end == -1 || end <= start {
		return rendered
	}
	return rendered[start:end]
}
