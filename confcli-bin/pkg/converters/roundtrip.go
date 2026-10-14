package converters

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	nethtml "golang.org/x/net/html"
)

// RoundTripOptions provides the page context missing from storage-format links.
type RoundTripOptions struct {
	BaseURL, SpaceKey             string
	LocalRepoPath, CurrentPageDir string
	PageID                        int
	InlineTransclusions           bool
	FetchPage                     func(id int) (RoundTripPage, error)
	transclusionStack             []int
	// A zero ID with no error means the referenced title does not exist.
	ResolvePage  func(space, title string) (int, error)
	TitleForPage func(id string) string
	Warn         func(string)
}

// PrepareRoundTripExport uses storage as the document spine, keeping macro
// boundaries and link targets in their original positions. Rendered plugin
// macros are taken from export_view only when their macro ID matches exactly.
// The restore function reinstates Markdown comments/embeds after html2md has
// finished its cleanup (which would otherwise discard comments and anchors).
func PrepareRoundTripExport(storage, exportView string, opts RoundTripOptions) (string, func(string) string, error) {
	root, err := parseStorage(storage)
	if err != nil {
		return "", nil, fmt.Errorf("parse storage for macro preservation: %w", err)
	}
	rendered := renderedMacros(exportView)
	prefix := fmt.Sprintf("CONFCLIRT%x", sha256.Sum256([]byte(storage)))
	for strings.Contains(storage, prefix) || strings.Contains(exportView, prefix) {
		prefix += "X"
	}
	var originals []string
	tableDepth := 0
	orderedDepth := 0
	stash := func(s string) string {
		token := fmt.Sprintf("%sN%dZ", prefix, len(originals))
		originals = append(originals, s)
		return token
	}
	codeCell := func(content, language string) string {
		encoded := html.EscapeString(content)
		encoded = strings.NewReplacer("\n", "&#10;", "\r", "&#13;", "|", "&#124;", "`", "&#96;", "*", "&#42;", "[", "&#91;", "]", "&#93;", "_", "&#95;", "\\", "&#92;").Replace(encoded)
		return stash(`<pre><code class="language-` + html.EscapeString(language) + `">` + encoded + `</code></pre>`)
	}
	pageLink := func(n *storageNode, allowMissing bool) (string, string, error) {
		ref := n.find("ri:page")
		anchor := n.attr("ac:anchor")
		if ref == nil {
			if anchor != "" {
				return "#" + escapeMarkdownPath(anchor), "", nil
			}
			return "", "", fmt.Errorf("page link has no ri:page or anchor")
		}
		title, space := ref.attr("ri:content-title"), ref.attr("ri:space-key")
		if space == "" {
			space = opts.SpaceKey
		}
		id := ref.attr("ri:content-id")
		if title == "" && id != "" && opts.TitleForPage != nil {
			title = opts.TitleForPage(id)
		}
		if id == "" {
			if opts.ResolvePage == nil {
				return "", "", fmt.Errorf("cannot resolve page %q in space %q", title, space)
			}
			pageID, e := opts.ResolvePage(space, title)
			if e != nil {
				return "", "", e
			}
			if pageID <= 0 {
				if !allowMissing {
					return "", "", fmt.Errorf("included page %q in %q not found", title, space)
				}
				href := strings.TrimRight(opts.BaseURL, "/") + "/display/" + url.PathEscape(space) + "/" + url.QueryEscape(title)
				if anchor != "" {
					href += "#" + escapeMarkdownPath(anchor)
				}
				if opts.Warn != nil {
					opts.Warn(fmt.Sprintf("page %q in %q not found; keeping the Confluence title link", title, space))
				}
				return href, title, nil
			}
			id = fmt.Sprint(pageID)
		}
		href := strings.TrimRight(opts.BaseURL, "/") + "/pages/viewpage.action?pageId=" + id
		if anchor != "" {
			href += "#" + escapeMarkdownPath(anchor)
		}
		return href, title, nil
	}
	attachmentLink := func(n *storageNode) (string, string, error) {
		ref := n.find("ri:attachment")
		if ref == nil {
			return "", "", fmt.Errorf("image/link has no attachment reference")
		}
		id := opts.PageID
		if owner := ref.find("ri:page"); owner != nil {
			title, space := owner.attr("ri:content-title"), owner.attr("ri:space-key")
			if space == "" {
				space = opts.SpaceKey
			}
			if opts.ResolvePage == nil {
				return "", "", fmt.Errorf("cannot resolve attachment owner %q", title)
			}
			var err error
			id, err = opts.ResolvePage(space, title)
			if err != nil {
				return "", "", err
			}
		}
		if id <= 0 {
			return "", "", fmt.Errorf("attachment owner page ID is required")
		}
		filename := ref.attr("ri:filename")
		return strings.TrimRight(opts.BaseURL, "/") + "/download/attachments/" + fmt.Sprint(id) + "/" + url.PathEscape(filename), filename, nil
	}
	var render func(*storageNode) (string, error)
	children := func(n *storageNode) (string, error) {
		var b strings.Builder
		for _, c := range n.children {
			s, e := render(c)
			if e != nil {
				return "", e
			}
			b.WriteString(s)
		}
		return b.String(), nil
	}
	render = func(n *storageNode) (string, error) {
		if n.name == "" {
			return html.EscapeString(n.text), nil
		}
		if n.name == "table" {
			tableDepth++
			defer func() { tableDepth-- }()
		}
		if n.name == "ol" {
			orderedDepth++
			defer func() { orderedDepth-- }()
			// Confluence displays nested ordered lists alphabetically. Markdown
			// cannot express that marker style; retain simple nested lists as HTML.
			// Complex lists with macros still use the Markdown conversion path.
			var hasMacro func(*storageNode) bool
			hasMacro = func(node *storageNode) bool {
				if strings.HasPrefix(node.name, "ac:") || strings.HasPrefix(node.name, "ri:") {
					return true
				}
				for _, c := range node.children {
					if hasMacro(c) {
						return true
					}
				}
				return false
			}
			if tableDepth == 0 && orderedDepth > 1 && !hasMacro(n) {
				inner, e := children(n)
				if e != nil {
					return "", e
				}
				kind := n.attr("type")
				if kind == "" {
					kind = "a"
					if orderedDepth%3 == 0 {
						kind = "i"
					}
					if orderedDepth%3 == 1 {
						kind = "1"
					}
				}
				attrs := ` type="` + html.EscapeString(kind) + `"`
				for _, key := range []string{"start", "style"} {
					if value := n.attr(key); value != "" {
						attrs += ` ` + key + `="` + html.EscapeString(value) + `"`
					}
				}
				return "<p>" + stash("<ol"+attrs+">"+inner+"</ol>") + "</p>", nil
			}
		}
		if tableDepth > 0 && (n.name == "pre" || (n.name == "code" && strings.Contains(n.content(), "\n"))) {
			var plain func(*storageNode) string
			plain = func(n *storageNode) string {
				if n.name == "br" {
					return "\n"
				}
				if n.name == "" {
					return n.text
				}
				var b strings.Builder
				for _, c := range n.children {
					b.WriteString(plain(c))
				}
				return b.String()
			}
			return codeCell(plain(n), ""), nil
		}
		if n.name == "ac:image" {
			href, label, err := attachmentLink(n)
			if ref := n.find("ri:url"); ref != nil {
				href, label, err = ref.attr("ri:value"), "", nil
			}
			if err != nil {
				return "", err
			}
			if alt := n.attr("ac:alt"); alt != "" {
				label = alt
			}
			return `<img src="` + html.EscapeString(href) + `" alt="` + html.EscapeString(label) + `" />`, nil
		}
		if n.name == "ac:link" {
			// Confluence keeps unresolved links without a resource target. They
			// still have useful visible text; do not abort the whole export.
			if n.find("ri:page") == nil && n.find("ri:attachment") == nil && n.attr("ac:anchor") == "" {
				label := n.content()
				if user := n.find("ri:user"); user != nil {
					label = user.attr("ri:username")
					href := strings.TrimRight(opts.BaseURL, "/") + "/display/~" + url.PathEscape(label)
					if label == "" {
						label = user.attr("ri:userkey")
						href = strings.TrimRight(opts.BaseURL, "/") + "/users/viewuserprofile.action?key=" + url.QueryEscape(label)
					}
					return `<a href="` + html.EscapeString(href) + `">` + html.EscapeString(label) + `</a>`, nil
				}
				if opts.Warn != nil {
					opts.Warn(fmt.Sprintf("link %q has no resource target; preserving its text", label))
				}
				return html.EscapeString(label), nil
			}
			href, title, e := pageLink(n, true)
			if n.find("ri:attachment") != nil {
				href, title, e = attachmentLink(n)
			}
			if e != nil {
				return "", e
			}
			label := title
			if body := n.find("ac:plain-text-link-body"); body != nil {
				label = body.content()
			}
			if body := n.find("ac:link-body"); body != nil {
				var visible func(*storageNode) string
				visible = func(n *storageNode) string {
					if n.name == "" {
						return n.text
					}
					if n.name == "br" {
						return " "
					}
					var b strings.Builder
					for _, c := range n.children {
						b.WriteString(visible(c))
					}
					if n.name == "p" || n.name == "div" || n.name == "li" {
						return " " + b.String() + " "
					}
					return b.String()
				}
				label = visible(body)
			}
			if label == "" {
				label = title
				if label == "" {
					label = n.attr("ac:anchor")
				}
			}
			before, after := "", ""
			if strings.TrimLeftFunc(label, unicode.IsSpace) != label {
				before = " "
			}
			if strings.TrimRightFunc(label, unicode.IsSpace) != label {
				after = " "
			}
			return before + `<a href="` + html.EscapeString(href) + `">` + html.EscapeString(strings.TrimSpace(label)) + `</a>` + after, nil
		}
		if n.name == "ac:structured-macro" {
			name := n.attr("ac:name")
			switch name {
			case "excerpt":
				if body := n.find("ac:rich-text-body"); body != nil {
					content, e := children(body)
					if opts.InlineTransclusions {
						meta := transclusionMetadata{Macro: "excerpt", Parameters: n.params()}
						return "<div><p>" + stash(transclusionStart("excerpt", meta)) + "</p>" + content + "<p>" + stash("<!-- confcli-excerpt:end -->") + "</p></div>", e
					}
					return stash(`<a id="confcli-excerpt"></a>`) + content, e
				}
			case "excerpt-include":
				if opts.InlineTransclusions {
					block, e := inlineTransclusion(n, opts)
					return "<div>" + stash(block) + "</div>", e
				}
				href, title, e := pageLink(n, false)
				if e != nil {
					return "", fmt.Errorf("excerpt-include: %w", e)
				}
				if opts.Warn != nil {
					opts.Warn("excerpt-include retained as a link; copy the excerpt body for an offline snapshot")
				}
				return "<p>" + stash("["+escapeMarkdownLabel(title)+"]("+href+"#confcli-excerpt)") + "</p>", nil
			case "handy-tip":
				if body := n.find("ac:rich-text-body"); body != nil {
					content, e := children(body)
					if e != nil {
						return "", e
					}
					if id := n.params()["id"]; id != "" {
						content = stash(`<a id="handytip-`+html.EscapeString(id)+`"></a>`) + content
					}
					return content, nil
				}
			case "status-handy":
				// A static snapshot retains the state without an editable plugin ID.
				return html.EscapeString(n.params()["Status"]), nil
			case "markdown":
				if body := n.find("ac:plain-text-body"); body != nil {
					return "<p>" + stash(body.content()) + "</p>", nil
				}
			case "view-file":
				if n.find("ri:attachment") != nil {
					href, title, e := attachmentLink(n)
					if e != nil {
						return "", e
					}
					return "<p>" + stash("["+escapeMarkdownLabel(title)+"]("+href+")") + "</p>", nil
				}
			case "scroll-ignore":
				if body := n.find("ac:rich-text-body"); body != nil {
					return children(body)
				}
				return "", nil
			case "plantuml", "code":
				if body := n.find("ac:plain-text-body"); body != nil {
					params := n.params()
					language := params["language"]
					if name == "plantuml" {
						language = "plantuml"
					}
					content := strings.TrimSuffix(body.content(), "\n")
					metadata := ""
					if name == "code" && (len(params) > 1 || (len(params) == 1 && language == "")) {
						data, _ := json.Marshal(params)
						metadata = strings.ReplaceAll(string(data), "-", `\u002d`)
					}
					if tableDepth > 0 {
						// A multiline fence inside a pipe table breaks every row
						// after it. Keep this cell on one line; HTML entities retain
						// the exact preformatted text when rendered or imported.
						if metadata == "" {
							return codeCell(content, language), nil
						}
						encoded := html.EscapeString(content)
						encoded = strings.NewReplacer("\n", "&#10;", "\r", "&#13;", "|", "&#124;", "`", "&#96;", "*", "&#42;", "[", "&#91;", "]", "&#93;", "_", "&#95;", "\\", "&#92;").Replace(encoded)
						return stash(`<pre data-confcli-code-params="` + base64.StdEncoding.EncodeToString([]byte(metadata)) + `"><code>` + encoded + `</code></pre>`), nil
					}
					fence := "```"
					for strings.Contains(content, fence) {
						fence += "`"
					}
					block := fence + language + "\n" + content + "\n" + fence
					if metadata != "" {
						block = "<!-- confcli-code-params: " + metadata + " -->\n\n" + block
					}
					return "<p>" + stash(block) + "</p>", nil
				}
			case "include":
				if opts.InlineTransclusions {
					block, e := inlineTransclusion(n, opts)
					return "<div>" + stash(block) + "</div>", e
				}
				href, title, e := pageLink(n, false)
				if e != nil {
					return "", fmt.Errorf("include: %w", e)
				}
				return "<div>" + stash("!["+escapeMarkdownLabel(title)+"]("+href+")") + "</div>", nil
			case "anchor":
				anchor := n.params()[""]
				return stash(`<a id="` + html.EscapeString(anchor) + `"></a>`), nil
			case "expand", "panel", "note", "info", "warning", "tip":
				params := n.params()
				payload := params["title"]
				_, hasTitle := params["title"]
				if len(params) > 1 || (len(params) > 0 && !hasTitle) || strings.HasPrefix(payload, "{") || strings.Contains(payload, "--") || strings.ContainsAny(payload, "\r\n") {
					data, _ := json.Marshal(params)
					payload = strings.ReplaceAll(string(data), "-", `\u002d`)
				}
				body := n.find("ac:rich-text-body")
				content := ""
				if body != nil {
					content, err = children(body)
					if err != nil {
						return "", err
					}
				}
				return "<div><p>" + stash("<!-- "+name+"-start: "+payload+" -->") + "</p>\n" + content + "\n<p>" + stash("<!-- "+name+"-end -->") + "</p></div>", nil
			case "view-git-file":
				// The source path remains meaningful even for panel-less diagrams.
				if p := n.params()["path"]; p != "" {
					href := p
					if opts.LocalRepoPath != "" && opts.CurrentPageDir != "" {
						root, e := filepath.Abs(opts.LocalRepoPath)
						if e != nil {
							return "", e
						}
						currentDir, e := filepath.Abs(opts.CurrentPageDir)
						if e != nil {
							return "", e
						}
						rel, e := filepath.Rel(currentDir, filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(p, "/"))))
						if e != nil {
							return "", e
						}
						href = filepath.ToSlash(rel)
					} else if r := rendered[n.attr("ac:macro-id")]; r != "" {
						if fileURL := renderedFileURL(r); fileURL != "" {
							href = fileURL
						}
					}
					return stash("[" + escapeMarkdownLabel(p) + "](" + escapeMarkdownPath(href) + ")"), nil
				}
			}
			// Code/PlantUML bodies and containers must retain their source.
			if n.find("ac:plain-text-body") == nil && n.find("ac:rich-text-body") == nil {
				if r := rendered[n.attr("ac:macro-id")]; r != "" {
					return r, nil
				}
				if opts.Warn != nil {
					opts.Warn(fmt.Sprintf("macro %q has no matched export_view output; preserved its parameters in a confcli-unrendered-macro comment for manual migration", name))
				}
				data, _ := json.Marshal(struct {
					Name       string            `json:"name"`
					Parameters map[string]string `json:"parameters"`
				}{name, n.params()})
				return "<p>" + stash("<!-- confcli-unrendered-macro: "+strings.ReplaceAll(string(data), "-", `\u002d`)+" -->") + "</p>", nil
			}
			// Layout/plugin wrappers contain configuration next to the body.
			// Rendering all children would leak these parameters into prose.
			if body := n.find("ac:rich-text-body"); body != nil {
				return children(body)
			}
		}
		inner, e := children(n)
		if e != nil {
			return "", e
		}
		if n.name == "root" {
			return inner, nil
		}
		if strings.HasPrefix(n.name, "ac:layout") {
			return "<div>" + inner + "</div>", nil
		}
		var b strings.Builder
		b.WriteString("<" + n.name)
		for _, a := range n.attrs {
			value := a.Value
			if n.name == "a" && storageName(a.Name) == "href" {
				base, _ := url.Parse(opts.BaseURL)
				if u, e := url.Parse(value); e == nil && strings.Contains(u.Path, "/display/") && (u.Host == "" || (base != nil && u.Host == base.Host)) {
					parts := strings.SplitN(strings.SplitN(u.Path, "/display/", 2)[1], "/", 2)
					if len(parts) == 2 && opts.ResolvePage != nil {
						title := strings.ReplaceAll(parts[1], "+", " ")
						id, e := opts.ResolvePage(parts[0], title)
						if e != nil {
							return "", e
						}
						if id > 0 {
							value = strings.TrimRight(opts.BaseURL, "/") + "/pages/viewpage.action?pageId=" + fmt.Sprint(id)
							if u.Fragment != "" {
								value += "#" + escapeMarkdownPath(u.Fragment)
							}
						} else if opts.Warn != nil {
							opts.Warn(fmt.Sprintf("page %q in %q not found; keeping link %q", title, parts[0], value))
						}
					}
				}
				if u, e := url.Parse(value); e == nil && !u.IsAbs() && !strings.HasPrefix(value, "#") {
					if base, e := url.Parse(strings.TrimRight(opts.BaseURL, "/") + "/"); e == nil {
						value = base.ResolveReference(u).String()
					}
				}
			}
			b.WriteString(" " + storageName(a.Name) + `="` + html.EscapeString(value) + `"`)
		}
		switch n.name {
		case "br", "hr", "img", "wbr", "input", "area", "base", "col", "embed", "link", "meta", "param", "source", "track":
			b.WriteString(" />")
		default:
			b.WriteString(">" + inner + "</" + n.name + ">")
		}
		return b.String(), nil
	}
	prepared, err := render(root)
	if err != nil {
		return "", nil, err
	}
	return prepared, func(markdown string) string {
		for i := len(originals) - 1; i >= 0; i-- {
			token := fmt.Sprintf("%sN%dZ", prefix, i)
			for {
				pos := strings.Index(markdown, token)
				if pos < 0 {
					break
				}
				indent := markdown[strings.LastIndex(markdown[:pos], "\n")+1 : pos]
				original := originals[i]
				if strings.Trim(indent, " \t>") == "" {
					original = strings.ReplaceAll(original, "\n", "\n"+indent)
				}
				markdown = markdown[:pos] + original + markdown[pos+len(token):]
			}
		}
		return markdown
	}, nil
}

// StorageExcerptBody returns the original body bytes of an unnamed excerpt.
// This allows a migration to snapshot transcluded content without including
// unrelated parts of its source page.
func StorageExcerptBody(storage string) (string, bool, error) {
	root, err := parseStorage(storage)
	if err != nil {
		return "", false, err
	}
	var find func(*storageNode) *storageNode
	find = func(n *storageNode) *storageNode {
		if n.name == "ac:structured-macro" && n.attr("ac:name") == "excerpt" {
			return n.find("ac:rich-text-body")
		}
		for _, child := range n.children {
			if found := find(child); found != nil {
				return found
			}
		}
		return nil
	}
	body := find(root)
	if body == nil {
		return "", false, nil
	}
	return storage[body.contentStart:body.contentEnd], true, nil
}

type storageNode struct {
	name, text                           string
	attrs                                []xml.Attr
	children                             []*storageNode
	start, end, contentStart, contentEnd int
}

func storageName(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func parseStorage(s string) (*storageNode, error) {
	d := xml.NewDecoder(strings.NewReader(s))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	root := &storageNode{name: "root"}
	stack := []*storageNode{root}
	for {
		tokenStart := int(d.InputOffset())
		t, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := t.(type) {
		case xml.StartElement:
			n := &storageNode{name: storageName(t.Name), attrs: t.Attr, start: tokenStart, contentStart: int(d.InputOffset())}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) < 2 || stack[len(stack)-1].name != storageName(t.Name) {
				return nil, fmt.Errorf("unbalanced storage tag %s", storageName(t.Name))
			}
			stack[len(stack)-1].contentEnd = tokenStart
			stack[len(stack)-1].end = int(d.InputOffset())
			stack = stack[:len(stack)-1]
		case xml.CharData:
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, &storageNode{text: string(t)})
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("unclosed storage tag %s", stack[len(stack)-1].name)
	}
	return root, nil
}

func (n *storageNode) attr(name string) string {
	for _, a := range n.attrs {
		if storageName(a.Name) == name {
			return a.Value
		}
	}
	return ""
}
func (n *storageNode) find(name string) *storageNode {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
		if r := c.find(name); r != nil {
			return r
		}
	}
	return nil
}
func (n *storageNode) content() string {
	var b strings.Builder
	b.WriteString(n.text)
	for _, c := range n.children {
		b.WriteString(c.content())
	}
	return b.String()
}
func (n *storageNode) params() map[string]string {
	p := map[string]string{}
	for _, c := range n.children {
		if c.name == "ac:parameter" {
			p[c.attr("ac:name")] = c.content()
		}
	}
	return p
}
func renderedMacros(s string) map[string]string {
	out := map[string]string{}
	doc, err := nethtml.Parse(strings.NewReader(s))
	if err != nil {
		return out
	}
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if id := getAttr(n, "data-macro-id"); id != "" {
			var b strings.Builder
			renderHTMLNode(&b, n)
			out[id] = b.String()
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func renderedFileURL(s string) string {
	doc, err := nethtml.Parse(strings.NewReader(s))
	if err != nil {
		return ""
	}
	var walk func(*nethtml.Node) string
	walk = func(n *nethtml.Node) string {
		if n.Data == "a" {
			if href := getAttr(n, "href"); strings.Contains(href, "/_git/") {
				return href
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if s := walk(c); s != "" {
				return s
			}
		}
		return ""
	}
	return walk(doc)
}
func escapeMarkdownLabel(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "\n", " ", "\r", " ")
	return r.Replace(s)
}
func escapeMarkdownPath(s string) string {
	return strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "\n", "%0A", "\r", "%0D").Replace(s)
}
