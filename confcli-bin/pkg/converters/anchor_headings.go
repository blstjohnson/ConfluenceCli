package converters

import (
	"strings"

	nethtml "golang.org/x/net/html"
)

func storageHeadingText(n *storageNode) string {
	if n.name == "" {
		return n.text
	}
	if n.name == "ac:structured-macro" || n.name == "ac:parameter" || strings.HasPrefix(n.name, "ri:") {
		return ""
	}
	if n.name == "br" {
		return " "
	}
	var b strings.Builder
	for _, c := range n.children {
		b.WriteString(storageHeadingText(c))
	}
	return b.String()
}

func isStorageHeading(n *storageNode) bool {
	return len(n.name) == 2 && n.name[0] == 'h' && n.name[1] >= '1' && n.name[1] <= '6'
}

func storageHeadingLocations(root *storageNode) ([]*storageNode, []headingLocation) {
	var nodes []*storageNode
	var headings []headingLocation
	var walk func(*storageNode)
	walk = func(n *storageNode) {
		if isStorageHeading(n) {
			heading := strings.Join(strings.Fields(storageHeadingText(n)), " ")
			if heading != "" {
				nodes = append(nodes, n)
				headings = append(headings, headingLocation{heading, int(n.name[1] - '0')})
			}
		}
		if n.name == "ac:structured-macro" {
			if body := n.find("ac:rich-text-body"); body != nil {
				walk(body)
			}
			return
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
	return nodes, headings
}

// Include native Confluence heading IDs as aliases, while accepting visible
// link text only when it identifies a unique real heading. Never guess from
// similar anchor names or from headings belonging to an included page.
func pageAnchorIndex(root *storageNode, rendered, title string) map[string]string {
	owners := anchorHeadingNodes(root)
	index := map[string]string{}
	counts := map[string]int{}
	var headings []headingLocation
	var nodes []*storageNode
	positions := map[*storageNode]int{}
	compact := func(s string) string { return strings.Join(strings.Fields(s), "") }
	prefix := compact(title) + "-"
	var walk func(*storageNode)
	walk = func(n *storageNode) {
		if isStorageHeading(n) {
			heading := strings.Join(strings.Fields(storageHeadingText(n)), " ")
			if heading != "" {
				counts[heading]++
				positions[n] = len(nodes)
				nodes = append(nodes, n)
				headings = append(headings, headingLocation{heading, int(n.name[1] - '0')})
			}
		}
		if n.name == "ac:structured-macro" {
			if body := n.find("ac:rich-text-body"); body != nil {
				walk(body)
			}
			return
		}
		for _, c := range n.children {
			walk(c)
		}
	}
	walk(root)
	fragments := headingFragments(headings)
	for i, node := range nodes {
		heading, fragment := headings[i].text, fragments[i]
		if id := node.attr("id"); id != "" && fragment != "" {
			index[id] = fragment
		}
		if counts[heading] == 1 && fragment != "" {
			index[heading] = fragment
		}
		if title != "" && fragment != "" {
			alias := prefix + compact(heading)
			if _, exists := index[alias]; !exists {
				index[alias] = fragment
			}
		}
	}
	for anchor, owner := range owners {
		if i, ok := positions[owner]; ok && fragments[i] != "" {
			index[anchor] = fragments[i]
			if title != "" {
				index[prefix+compact(anchor)] = fragments[i]
				index[prefix+compact(anchor)+compact(headings[i].text)] = fragments[i]
			}
		}
	}
	if rendered != "" {
		if doc, err := nethtml.Parse(strings.NewReader(rendered)); err == nil {
			var text func(*nethtml.Node) string
			text = func(n *nethtml.Node) string {
				if n.Type == nethtml.TextNode {
					return n.Data
				}
				var b strings.Builder
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					b.WriteString(text(c))
				}
				return b.String()
			}
			// Match rendered headings to storage occurrences in document order.
			// Includes can repeat heading text, but have another page's ID prefix.
			next := 0
			var visit func(*nethtml.Node)
			visit = func(n *nethtml.Node) {
				if len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
					heading := strings.Join(strings.Fields(text(n)), " ")
					if counts[heading] > 0 {
						for _, a := range n.Attr {
							if a.Key == "id" && (title == "" || strings.HasPrefix(a.Val, prefix)) {
								for i := next; i < len(headings); i++ {
									if headings[i].text == heading && headings[i].level == int(n.Data[1]-'0') {
										next = i + 1
										if fragments[i] != "" {
											index[a.Val] = fragments[i]
										}
										break
									}
								}
							}
						}
					}
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					visit(c)
				}
			}
			visit(doc)
		}
	}
	return index
}

// AnchorHeadings maps named Confluence anchors to the visible heading that
// owns them. Anchors immediately preceding a heading belong to that heading;
// anchors within a section otherwise belong to the preceding heading.
func AnchorHeadings(storage string) (map[string]string, error) {
	root, err := parseStorage(storage)
	if err != nil {
		return nil, err
	}
	return anchorHeadings(root), nil
}

func anchorHeadings(root *storageNode) map[string]string {
	result := map[string]string{}
	for anchor, owner := range anchorHeadingNodes(root) {
		result[anchor] = strings.Join(strings.Fields(storageHeadingText(owner)), " ")
	}
	return result
}

func anchorHeadingNodes(root *storageNode) map[string]*storageNode {
	result := map[string]*storageNode{}
	var last *storageNode
	var pending []string
	flush := func(heading *storageNode) {
		for _, anchor := range pending {
			if heading != nil {
				result[anchor] = heading
			}
		}
		pending = nil
	}
	var walk func(*storageNode, *storageNode)
	walk = func(n *storageNode, enclosing *storageNode) {
		if isStorageHeading(n) {
			flush(n)
			last = n
			for _, c := range n.children {
				walk(c, n)
			}
			return
		}
		if n.name == "ac:structured-macro" {
			if n.attr("ac:name") == "anchor" {
				if anchor := n.params()[""]; anchor != "" {
					if enclosing != nil {
						result[anchor] = enclosing
					} else {
						pending = append(pending, anchor)
					}
				}
			} else if body := n.find("ac:rich-text-body"); body != nil {
				walk(body, enclosing)
			}
			return // macro parameters/code are not document text
		}
		if n.name == "" && strings.TrimSpace(n.text) != "" {
			flush(last)
		}
		for _, c := range n.children {
			walk(c, enclosing)
		}
	}
	walk(root, nil)
	flush(last)
	return result
}
