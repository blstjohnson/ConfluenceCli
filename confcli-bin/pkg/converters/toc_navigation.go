package converters

import "strings"

// isTOCNavigationExpand distinguishes navigation chrome from an ordinary
// Expand with user-authored content. Empty legacy TOC wrappers arise when an
// export profile removes the inner toc macro before preserving other macros.
func isTOCNavigationExpand(n *storageNode) bool {
	if n.name != "ac:structured-macro" || n.attr("ac:name") != "expand" {
		return false
	}
	body := n.find("ac:rich-text-body")
	if body == nil {
		return false
	}
	onlyNavigation, hasTOC := tocNavigationContent(body)
	if !onlyNavigation {
		return false
	}
	if hasTOC {
		return true
	}
	return isTOCTitle(n.params()["title"])
}

func tocNavigationContent(n *storageNode) (bool, bool) {
	if n.attr("id") != "" || n.attr("name") != "" {
		return false, false
	}
	if n.name == "" {
		return strings.TrimSpace(n.text) == "", false
	}
	if n.name == "ac:structured-macro" {
		return n.attr("ac:name") == "toc", n.attr("ac:name") == "toc"
	}
	switch n.name {
	case "ac:rich-text-body", "p", "div", "span", "br":
	default:
		return false, false
	}
	hasTOC := false
	for _, child := range n.children {
		only, toc := tocNavigationContent(child)
		if !only {
			return false, false
		}
		hasTOC = hasTOC || toc
	}
	return true, hasTOC
}

// WithLeadingTableOfContents puts one real toc inside a leading Expand.
// When add is false, existing TOCs and navigation-only wrappers are removed.
// Otherwise existing TOC parameters are preserved. Only navigation wrappers and actual
// TOC nodes are removed from their old positions; code examples and unrelated
// Expand content remain byte-for-byte intact.
func WithLeadingTableOfContents(storage string, add bool) (string, error) {
	root, err := parseStorage(storage)
	if err != nil {
		return "", err
	}
	toc := ""
	var collect func(*storageNode)
	collect = func(n *storageNode) {
		if n.name == "ac:plain-text-body" || n.name == "ac:parameter" {
			return
		}
		if n.name == "ac:structured-macro" && n.attr("ac:name") == "toc" && toc == "" {
			toc = storage[n.start:n.end]
		}
		for _, child := range n.children {
			collect(child)
		}
	}
	collect(root)
	var ranges [][2]int
	var remove func(*storageNode)
	remove = func(n *storageNode) {
		if n.name == "ac:plain-text-body" || n.name == "ac:parameter" {
			return
		}
		if isTOCNavigationExpand(n) || (n.name == "ac:structured-macro" && n.attr("ac:name") == "toc") {
			ranges = append(ranges, [2]int{n.start, n.end})
			return
		}
		for _, child := range n.children {
			remove(child)
		}
	}
	remove(root)
	for i := len(ranges) - 1; i >= 0; i-- {
		storage = storage[:ranges[i][0]] + storage[ranges[i][1]:]
	}
	if !add {
		return storage, nil
	}
	if toc == "" {
		toc = `<ac:structured-macro ac:name="toc" ac:schema-version="1"></ac:structured-macro>`
	}
	return `<ac:structured-macro ac:name="expand" ac:schema-version="1"><ac:parameter ac:name="title">Оглавление</ac:parameter><ac:rich-text-body>` + toc + `</ac:rich-text-body></ac:structured-macro>` + "\n" + strings.TrimLeft(storage, " \t\r\n"), nil
}
