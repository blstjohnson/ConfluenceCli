package markdownutil

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var tableFollowingList = regexp.MustCompile(`^[ \t]*(?:[-*+]|[0-9]+[.)])[ \t]+`)
var tableListItem = regexp.MustCompile(`^([ \t]*)([0-9]{1,9}[.)]|[-*+])([ \t]+)(.*)$`)

// NormalizeTableLists keeps tables within the list items that introduce them.
// A dedented pipe table followed by a higher-numbered item resumes the same
// ordered list. Nesting the table keeps numbering continuous even in renderers
// that discard the start attribute of a new <ol>, including Confluence.
func NormalizeTableLists(source string) string {
	lines := strings.Split(source, "\n")
	masked := strings.Split(maskTableSource(source), "\n")
	for i := 1; i < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(masked[i-1]), "|") || !isTableDelimiter(strings.TrimSpace(masked[i])) {
			continue
		}
		start, end := i-1, i+1
		for end < len(lines) && strings.HasPrefix(strings.TrimSpace(masked[end]), "|") {
			end++
		}
		next := end
		for next < len(lines) && strings.TrimSpace(masked[next]) == "" {
			next++
		}
		if next == len(lines) {
			continue
		}
		following := tableListItem.FindStringSubmatch(masked[next])
		if following == nil {
			continue
		}
		nextOrdered := following[2][0] >= '0' && following[2][0] <= '9'
		nextIndent := tableLineIndent(following[1])
		nextNumber, _ := strconv.Atoi(following[2][:len(following[2])-1])
		previous := start - 1
		for previous >= 0 && strings.TrimSpace(masked[previous]) == "" {
			previous--
		}
		if previous < 0 {
			continue
		}
		item := tableListItem.FindStringSubmatch(masked[previous])
		if item == nil || tableLineIndent(item[1]) < nextIndent {
			continue
		}
		indent := tableLineIndent(item[1]) + len(item[2]) + 1
		// Find the enclosing ordered item at the continuation's depth, without
		// crossing another table, heading or independent paragraph.
		continued := false
		for j := previous; j >= 0; j-- {
			line := strings.TrimSpace(masked[j])
			if line == "" {
				continue
			}
			parent := tableListItem.FindStringSubmatch(masked[j])
			if parent != nil {
				parentIndent := tableLineIndent(parent[1])
				parentOrdered := parent[2][0] >= '0' && parent[2][0] <= '9'
				if nextOrdered && parentIndent == nextIndent {
					if parentOrdered {
						number, _ := strconv.Atoi(parent[2][:len(parent[2])-1])
						continued = nextNumber > number
					}
					break
				}
				if !nextOrdered && parentOrdered && parentIndent < nextIndent {
					continued = nextIndent >= parentIndent+len(parent[2])+1
					break
				}
			}
			if strings.HasPrefix(line, "#") || (parent == nil && tableLineIndent(masked[j]) <= nextIndent) {
				break
			}
		}
		if !continued || tableLineIndent(masked[start]) >= indent {
			continue
		}
		for j := start; j < end; j++ {
			padding := indent - tableLineIndent(masked[j])
			if padding > 0 {
				lines[j] = strings.Repeat(" ", padding) + lines[j]
				masked[j] = strings.Repeat(" ", padding) + masked[j]
			}
		}
		i = end - 1
	}
	return SeparateTableLists(strings.Join(lines, "\n"))
}

// SeparateTableLists keeps a list immediately following a pipe table from being
// parsed as more table rows. Only an explicit table header/delimiter and bordered
// rows trigger the repair; cell contents and code examples remain unchanged.
func SeparateTableLists(source string) string {
	lines := strings.Split(source, "\n")
	masked := strings.Split(maskTableSource(source), "\n")
	result := make([]string, 0, len(lines))
	inTable := false
	tableIndent := 0
	for i, line := range lines {
		current := strings.TrimSpace(masked[i])
		if inTable && !strings.HasPrefix(current, "|") {
			// A dedented list item already closes a properly nested table.
			if tableFollowingList.MatchString(masked[i]) && tableLineIndent(masked[i]) >= tableIndent {
				blank := ""
				if i > 0 && strings.HasSuffix(lines[i-1], "\r") {
					blank = "\r"
				}
				result = append(result, blank)
			}
			inTable = false
		}
		if i > 0 && strings.HasPrefix(strings.TrimSpace(masked[i-1]), "|") && isTableDelimiter(current) {
			inTable = true
			tableIndent = tableLineIndent(masked[i])
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// Raw HTML, including <pre> examples, is already responsible for its own block
// boundaries. Do not insert Markdown separators inside it.
func maskTableSource(source string) string {
	masked := []byte(MaskCode(source))
	doc := goldmark.DefaultParser().Parse(text.NewReader(masked))
	mask := func(start, end int) {
		for i := start; i < end; i++ {
			if masked[i] != '\n' && masked[i] != '\r' {
				masked[i] = ' '
			}
		}
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		block, ok := n.(*ast.HTMLBlock)
		if entering && ok {
			for i := 0; i < block.Lines().Len(); i++ {
				segment := block.Lines().At(i)
				mask(segment.Start, segment.Stop)
			}
			if block.HasClosure() {
				mask(block.ClosureLine.Start, block.ClosureLine.Stop)
			}
		}
		return ast.WalkContinue, nil
	})
	return string(masked)
}

func tableLineIndent(line string) int {
	indent := 0
	for _, ch := range line {
		switch ch {
		case ' ':
			indent++
		case '\t':
			indent += 4 - indent%4
		default:
			return indent
		}
	}
	return indent
}

func isTableDelimiter(line string) bool {
	if !strings.HasPrefix(line, "|") {
		return false
	}
	cells := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|"), "|")
	for _, cell := range cells {
		cell = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(cell), ":"), ":")
		if cell == "" || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}
