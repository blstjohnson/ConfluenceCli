package converters

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	html2md "github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

// ConfluencePlugin implements html-to-markdown plugin for Confluence-specific elements
type ConfluencePlugin struct {
	converter *html2md.Converter
}

// Name returns the plugin name
func (p *ConfluencePlugin) Name() string {
	return "confluence"
}

// Init registers Confluence-specific HTML element handlers
func (p *ConfluencePlugin) Init(conv *html2md.Converter) error {
	p.converter = conv

	// Handle Confluence structured macros (info/warning/note/tip panels, status, expand, code, toc)
	conv.Register.RendererFor("ac:structured-macro", html2md.TagTypeBlock, p.handleMacro, html2md.PriorityStandard)

	// Handle user mentions (span with data-account-id/data-user-key)
	conv.Register.RendererFor("span", html2md.TagTypeInline, p.handleSpan, html2md.PriorityStandard)

	// Handle div containers (expand macros, git-plugin-container, etc.)
	conv.Register.RendererFor("div", html2md.TagTypeBlock, p.handleDiv, html2md.PriorityStandard)

	// Handle script tags — strip completely
	conv.Register.RendererFor("script", html2md.TagTypeBlock, p.handleStrip, html2md.PriorityStandard)

	// Handle style tags — strip completely
	conv.Register.RendererFor("style", html2md.TagTypeBlock, p.handleStrip, html2md.PriorityStandard)

	// Handle svg elements — strip completely
	conv.Register.RendererFor("svg", html2md.TagTypeInline, p.handleStrip, html2md.PriorityStandard)

	// Handle time elements — extract datetime
	conv.Register.RendererFor("time", html2md.TagTypeInline, p.handleTime, html2md.PriorityStandard)

	// Handle confluence-userlink anchors
	conv.Register.RendererFor("a", html2md.TagTypeInline, p.handleAnchor, html2md.PriorityStandard)

	// Handle aui-inline-dialog — strip completely
	conv.Register.RendererFor("aui-inline-dialog", html2md.TagTypeBlock, p.handleStrip, html2md.PriorityStandard)

	return nil
}

// handleStrip strips elements completely (script, style, svg, aui-inline-dialog)
func (p *ConfluencePlugin) handleStrip(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	return html2md.RenderSuccess
}

// handleTime extracts datetime from <time> elements
func (p *ConfluencePlugin) handleTime(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	// Try datetime attribute first
	for _, attr := range n.Attr {
		if attr.Key == "datetime" {
			w.WriteString(attr.Val)
			return html2md.RenderSuccess
		}
	}
	// Fall back to text content
	text := strings.TrimSpace(p.getNodeText(n))
	if text != "" {
		w.WriteString(text)
		return html2md.RenderSuccess
	}
	return html2md.RenderTryNext
}

// handleAnchor handles anchor elements, specifically confluence-userlink
func (p *ConfluencePlugin) handleAnchor(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	// Check if this is a confluence-userlink
	isUserLink := false
	for _, attr := range n.Attr {
		if attr.Key == "class" && strings.Contains(attr.Val, "confluence-userlink") {
			isUserLink = true
			break
		}
	}
	if !isUserLink {
		return html2md.RenderTryNext
	}

	// Extract display name from data-username or text content
	displayName := ""
	for _, attr := range n.Attr {
		if attr.Key == "data-username" {
			displayName = attr.Val
			break
		}
	}

	// Prefer text content over data-username (text is usually the full name)
	textContent := strings.TrimSpace(p.getNodeText(n))
	if textContent != "" {
		displayName = textContent
	}

	if displayName == "" {
		displayName = "Unknown User"
	}

	// Clean up deactivated user suffixes
	displayName = strings.TrimSuffix(displayName, " (Unlicensed)")
	displayName = strings.TrimSuffix(displayName, " (Deactivated)")

	w.WriteString(fmt.Sprintf("@%s", displayName))
	return html2md.RenderSuccess
}

// handleSpan handles span elements — dispatches to user mention or passes through
func (p *ConfluencePlugin) handleSpan(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	// Check if this is a user mention
	for _, attr := range n.Attr {
		if attr.Key == "data-account-id" || attr.Key == "data-user-key" {
			return p.handleUserMention(ctx, w, n)
		}
	}
	return html2md.RenderTryNext
}

// handleDiv dispatches div handling based on class/attributes
func (p *ConfluencePlugin) handleDiv(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	for _, attr := range n.Attr {
		if attr.Key == "data-macro-name" && attr.Val == "expand" {
			return p.handleExpandDiv(ctx, w, n)
		}
		if attr.Key == "class" {
			if strings.Contains(attr.Val, "git-plugin-container") {
				return p.handleGitPluginContainer(ctx, w, n)
			}
			if strings.Contains(attr.Val, "aui-inline-dialog") {
				// Strip AUI inline dialogs (duplicate metadata)
				return html2md.RenderSuccess
			}
		}
	}
	return html2md.RenderTryNext
}

// handleMacro handles ac:structured-macro elements
func (p *ConfluencePlugin) handleMacro(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	macroName := ""
	for _, attr := range n.Attr {
		if attr.Key == "ac:name" {
			macroName = attr.Val
			break
		}
	}

	switch macroName {
	case "info", "warning", "note", "tip":
		return p.handlePanelMacro(ctx, w, n, macroName)
	case "status":
		return p.handleStatusMacro(ctx, w, n)
	case "expand":
		return p.handleExpandMacro(ctx, w, n)
	case "code":
		return p.handleCodeMacro(ctx, w, n)
	case "toc":
		return p.handleTocMacro(ctx, w, n)
	default:
		// Unknown macro - let other handlers try or skip
		return html2md.RenderTryNext
	}
}

// handlePanelMacro handles info/warning/note/tip panels
func (p *ConfluencePlugin) handlePanelMacro(ctx html2md.Context, w html2md.Writer, n *html.Node, macroName string) html2md.RenderStatus {
	emoji := "ℹ️"
	label := "Info"
	switch macroName {
	case "warning":
		emoji = "⚠️"
		label = "Warning"
	case "note":
		emoji = "📝"
		label = "Note"
	case "tip":
		emoji = "💡"
		label = "Tip"
	}

	// Get content from ac:rich-text-body — render children through converter
	var content string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "ac:rich-text-body" {
			content = p.renderChildren(ctx, child)
			break
		}
	}

	content = strings.TrimSpace(content)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}

	w.WriteString(fmt.Sprintf("> %s **%s:**\n%s\n\n", emoji, label, strings.Join(lines, "\n")))
	return html2md.RenderSuccess
}

// handleStatusMacro handles status macros
func (p *ConfluencePlugin) handleStatusMacro(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	bgColor := ""
	for _, attr := range n.Attr {
		if attr.Key == "data-background-color" {
			bgColor = attr.Val
			break
		}
	}

	// Get title from ac:rich-text-body
	var title string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "ac:rich-text-body" {
			title = strings.TrimSpace(p.getNodeText(child))
			break
		}
	}

	emoji := ""
	switch bgColor {
	case "red":
		emoji = "🔴"
	case "yellow":
		emoji = "🟡"
	case "green":
		emoji = "🟢"
	case "blue":
		emoji = "🔵"
	case "grey":
		emoji = "⚪"
	}

	if emoji != "" {
		w.WriteString(fmt.Sprintf("%s **%s**", emoji, title))
	} else {
		w.WriteString(fmt.Sprintf("**[%s]**", title))
	}
	return html2md.RenderSuccess
}

// handleExpandMacro handles expand macros
func (p *ConfluencePlugin) handleExpandMacro(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	summary := "Click to expand"
	var content string

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			if child.Data == "ac:parameter" {
				// Check if this is the summary parameter
				for _, attr := range child.Attr {
					if attr.Key == "ac:name" && attr.Val == "summary" {
						summary = p.getNodeText(child)
						break
					}
				}
			} else if child.Data == "ac:rich-text-body" {
				content = strings.TrimSpace(p.renderChildren(ctx, child))
			}
		}
	}

	w.WriteString(fmt.Sprintf("<details>\n<summary>%s</summary>\n\n%s\n\n</details>\n", summary, content))
	return html2md.RenderSuccess
}

// handleExpandDiv handles expand macro div containers (alternative format)
func (p *ConfluencePlugin) handleExpandDiv(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	summary := "Click to expand"
	var content string

	// Find summary and content
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			if child.Data == "span" {
				// Check for expand-control-text class
				for _, attr := range child.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "expand-control-text") {
						summary = strings.TrimSpace(p.getNodeText(child))
						break
					}
				}
			} else if child.Data == "div" {
				for _, attr := range child.Attr {
					if attr.Key == "class" && strings.Contains(attr.Val, "expand-content") {
						content = strings.TrimSpace(p.renderChildren(ctx, child))
						break
					}
				}
			}
		}
	}

	w.WriteString(fmt.Sprintf("<details>\n<summary>%s</summary>\n\n%s\n\n</details>\n", summary, content))
	return html2md.RenderSuccess
}

// handleGitPluginContainer handles Git-for-Confluence plugin containers
// Extracts the file name and TFS link, producing clean markdown
func (p *ConfluencePlugin) handleGitPluginContainer(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	var fileName string
	var sourceLink string
	var imageURL string

	// Walk the tree to find meaningful content
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			// Look for strong elements (file name)
			if node.Data == "strong" && fileName == "" {
				text := strings.TrimSpace(p.getNodeText(node))
				if text != "" {
					fileName = text
				}
			}

			// Look for links that might be the source path
			if node.Data == "a" {
				href := getAttr(node, "href")
				text := strings.TrimSpace(p.getNodeText(node))
				// Prefer links containing _git (TFS links) or "Full path" context
				if href != "" && (strings.Contains(href, "_git") || strings.Contains(href, "tfs")) {
					sourceLink = href
				} else if href != "" && sourceLink == "" && text != "" {
					sourceLink = href
				}
			}

			// Look for images (e.g., rendered PlantUML diagrams)
			if node.Data == "img" {
				src := getAttr(node, "src")
				if src != "" {
					imageURL = src
				}
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)

	// Build clean markdown output
	if fileName == "" && sourceLink == "" && imageURL == "" {
		// Nothing useful extracted, skip the container
		return html2md.RenderSuccess
	}

	var parts []string
	if fileName != "" {
		if sourceLink != "" {
			parts = append(parts, fmt.Sprintf("**%s** ([source](%s))", fileName, sourceLink))
		} else {
			parts = append(parts, fmt.Sprintf("**%s**", fileName))
		}
	} else if sourceLink != "" {
		parts = append(parts, fmt.Sprintf("[source](%s)", sourceLink))
	}

	if imageURL != "" {
		parts = append(parts, fmt.Sprintf("![%s](%s)", fileName, imageURL))
	}

	w.WriteString(strings.Join(parts, "\n\n") + "\n\n")
	return html2md.RenderSuccess
}

// handleCodeMacro handles code macros
func (p *ConfluencePlugin) handleCodeMacro(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	language := ""
	for _, attr := range n.Attr {
		if attr.Key == "ac:parameter" {
			for _, pattr := range n.Attr {
				if pattr.Key == "ac:name" && pattr.Val == "language" {
					language = pattr.Val
					break
				}
			}
		}
	}

	// Try to get language from ac:parameter child
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "ac:parameter" {
			for _, attr := range child.Attr {
				if attr.Key == "ac:name" && attr.Val == "language" {
					language = strings.TrimSpace(p.getNodeText(child))
					break
				}
			}
		}
	}

	var content string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "ac:plain-text-body" {
			content = strings.TrimSpace(p.getNodeText(child))
			break
		}
	}

	content = strings.TrimPrefix(content, "\n")
	content = strings.TrimSuffix(content, "\n")

	w.WriteString(fmt.Sprintf("```%s\n%s\n```\n", language, content))
	return html2md.RenderSuccess
}

// handleTocMacro handles TOC macros (skips them)
func (p *ConfluencePlugin) handleTocMacro(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	// Skip TOC in markdown
	return html2md.RenderSuccess
}

// handleUserMention handles user mentions (span with data-account-id/data-user-key)
func (p *ConfluencePlugin) handleUserMention(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	// Get display name
	displayName := ""
	for _, attr := range n.Attr {
		if attr.Key == "data-display-name" {
			displayName = attr.Val
			break
		}
	}

	if displayName == "" {
		displayName = strings.TrimSpace(p.getNodeText(n))
	}

	if displayName == "" {
		displayName = "Unknown User"
	}

	// Clean up deactivated user suffixes
	displayName = strings.TrimSuffix(displayName, " (Unlicensed)")
	displayName = strings.TrimSuffix(displayName, " (Deactivated)")

	w.WriteString(fmt.Sprintf("@%s", displayName))
	return html2md.RenderSuccess
}

// extractText extracts raw text content from an HTML node (no formatting).
// Standalone version used by preprocessing functions.
func extractText(n *html.Node) string {
	var text strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return text.String()
}

// getNodeText extracts raw text content from an HTML node (no formatting)
func (p *ConfluencePlugin) getNodeText(n *html.Node) string {
	var text strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return text.String()
}

// renderChildren converts child nodes through the markdown converter,
// preserving nested formatting (bold, italic, links, tables, etc.)
// instead of flattening everything to plain text like getNodeText does.
func (p *ConfluencePlugin) renderChildren(ctx html2md.Context, n *html.Node) string {
	if p.converter == nil {
		// Fallback if converter not available
		return p.getNodeText(n)
	}

	// Render the inner HTML of this node through the converter
	var htmlBuf strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		renderHTMLNode(&htmlBuf, child)
	}

	innerHTML := htmlBuf.String()
	if strings.TrimSpace(innerHTML) == "" {
		return ""
	}

	md, err := p.converter.ConvertString(innerHTML)
	if err != nil {
		// Fallback to plain text on error
		return p.getNodeText(n)
	}

	return md
}

// renderHTMLNode serializes an HTML node back to HTML string
func renderHTMLNode(w *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		w.WriteString(html.EscapeString(n.Data))
	case html.ElementNode:
		w.WriteString("<")
		w.WriteString(n.Data)
		for _, attr := range n.Attr {
			w.WriteString(" ")
			if attr.Namespace != "" {
				w.WriteString(attr.Namespace)
				w.WriteString(":")
			}
			w.WriteString(attr.Key)
			w.WriteString(`="`)
			w.WriteString(html.EscapeString(attr.Val))
			w.WriteString(`"`)
		}
		w.WriteString(">")
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			renderHTMLNode(w, child)
		}
		w.WriteString("</")
		w.WriteString(n.Data)
		w.WriteString(">")
	case html.CommentNode:
		// Skip comments
	default:
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			renderHTMLNode(w, child)
		}
	}
}

// getAttr returns the value of an attribute from an HTML node
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// underscoreEscapeRe matches backslash-escaped underscores in markdown link text and code
var underscoreEscapeRe = regexp.MustCompile(`\\(_)`)

// cleanUnderscoreEscaping removes unnecessary backslash escaping of underscores
// in markdown link text [text\_with\_underscores](url) and inline code contexts
func cleanUnderscoreEscaping(markdown string) string {
	// Fix escaped underscores inside markdown link text: [text\_name](url) -> [text_name](url)
	linkTextRe := regexp.MustCompile(`\[([^\]]*\\\_[^\]]*)\]\(`)
	result := linkTextRe.ReplaceAllStringFunc(markdown, func(match string) string {
		return strings.ReplaceAll(match, `\_`, `_`)
	})

	// Fix escaped underscores inside inline code: `code\_name` -> `code_name`
	codeRe := regexp.MustCompile("`[^`]*`")
	result = codeRe.ReplaceAllStringFunc(result, func(match string) string {
		return strings.ReplaceAll(match, `\_`, `_`)
	})

	return result
}

// confluenceTOCLinkRe matches a TOC line: `- [Text](#PagePrefix-EncodedAnchor)` or with indentation
// Confluence TOC anchors contain a PageName prefix followed by URL-encoded text
var confluenceTOCLinkRe = regexp.MustCompile(`^(\s*)-\s+\[([^\]]+)\]\(#[A-Za-z0-9]+-[^\)]*%[0-9A-Fa-f]{2}[^\)]*\)\s*$`)

// headingRe matches markdown headings: # Heading, ## Heading, etc.
var headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

// regenerateTOC detects the Confluence-generated TOC (bulleted list at the start
// with broken URL-encoded anchor links) and replaces it with a proper markdown TOC
// generated from the actual headings in the document.
func regenerateTOC(markdown string) string {
	lines := strings.Split(markdown, "\n")

	// Find the TOC block: consecutive lines at the start matching the Confluence TOC pattern
	// (allow leading blank lines)
	tocStart := -1
	tocEnd := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if tocStart == -1 {
				continue // skip leading blank lines
			}
			// Blank line inside TOC — could be between indentation levels
			continue
		}
		if confluenceTOCLinkRe.MatchString(line) {
			if tocStart == -1 {
				tocStart = i
			}
			tocEnd = i + 1
		} else {
			if tocStart != -1 {
				// End of TOC block
				break
			}
			// First non-blank, non-TOC line before any TOC found — no TOC present
			break
		}
	}

	if tocStart == -1 || tocEnd <= tocStart {
		return markdown // no TOC found
	}

	// Collect all headings from the rest of the document (after the TOC)
	type heading struct {
		level int
		text  string
	}
	var headings []heading
	for i := tocEnd; i < len(lines); i++ {
		if m := headingRe.FindStringSubmatch(lines[i]); m != nil {
			level := len(m[1])
			text := strings.TrimSpace(m[2])
			headings = append(headings, heading{level: level, text: text})
		}
	}

	if len(headings) == 0 {
		return markdown // no headings found, keep original
	}

	// Find minimum heading level to normalize indentation
	minLevel := headings[0].level
	for _, h := range headings {
		if h.level < minLevel {
			minLevel = h.level
		}
	}

	// Generate new TOC
	var toc strings.Builder
	for _, h := range headings {
		indent := strings.Repeat("  ", h.level-minLevel)
		anchor := headingToAnchor(h.text)
		toc.WriteString(fmt.Sprintf("%s- [%s](#%s)\n", indent, h.text, anchor))
	}

	// Replace old TOC with new TOC
	// Skip trailing blank lines after old TOC
	afterTOC := tocEnd
	for afterTOC < len(lines) && strings.TrimSpace(lines[afterTOC]) == "" {
		afterTOC++
	}

	var result strings.Builder
	// Write any leading blank lines before TOC
	for i := 0; i < tocStart; i++ {
		result.WriteString(lines[i])
		result.WriteString("\n")
	}
	// Write new TOC
	result.WriteString(toc.String())
	result.WriteString("\n")
	// Write rest of document
	for i := afterTOC; i < len(lines); i++ {
		result.WriteString(lines[i])
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// headingToAnchor converts a heading text to a GitHub-style markdown anchor.
// Lowercase, spaces → hyphens, remove special punctuation, keep Unicode letters.
func headingToAnchor(text string) string {
	// Remove markdown formatting: **bold**, *italic*, `code`, [link](url)
	// Strip bold/italic markers
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "`", "")
	// Replace [text](url) with just text
	linkRe := regexp.MustCompile(`\[([^\]]*)\]\([^\)]*\)`)
	text = linkRe.ReplaceAllString(text, "$1")

	text = strings.ToLower(text)

	var result strings.Builder
	prevHyphen := false
	for _, r := range text {
		if r == ' ' || r == '\t' {
			if !prevHyphen {
				result.WriteRune('-')
				prevHyphen = true
			}
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r > 127 {
			// Keep: lowercase ASCII letters, digits, hyphens, underscores, and all Unicode (Cyrillic etc.)
			result.WriteRune(r)
			prevHyphen = false
		}
		// Skip other punctuation: (, ), ., ,, :, etc.
	}

	return strings.TrimRight(result.String(), "-")
}

// flattenListsInTableCells converts <ul>/<ol> lists inside <td> cells to
// <br/>-separated items. The table plugin can't handle block-level list elements
// inside cells, but it handles <br/> fine with NewlineBehaviorPreserve.
func flattenListsInTableCells(htmlContent string) string {
	tdRe := regexp.MustCompile(`(?is)(<td\b[^>]*>)(.*?)(</td>)`)
	ulCheckRe := regexp.MustCompile(`(?i)<[uo]l\b`)
	listTagRe := regexp.MustCompile(`(?i)</?[uo]l\b[^>]*>`)
	liOpenRe := regexp.MustCompile(`(?i)<li\b[^>]*>`)

	return tdRe.ReplaceAllStringFunc(htmlContent, func(match string) string {
		sub := tdRe.FindStringSubmatch(match)
		if len(sub) < 4 {
			return match
		}
		content := sub[2]
		if !ulCheckRe.MatchString(content) {
			return match // no lists in this cell
		}

		// Remove <ul>, </ul>, <ol>, </ol> tags
		content = listTagRe.ReplaceAllString(content, "")
		// Replace <li> with nothing (content follows)
		content = liOpenRe.ReplaceAllString(content, "")
		// Replace </li> with <br/> to separate items
		content = strings.ReplaceAll(content, "</li>", "<br/>")

		return sub[1] + content + sub[3]
	})
}

// expandTableSpans normalizes HTML tables by expanding colspan and rowspan attributes
// into individual cells with duplicated content. This produces uniform 2D tables that
// the markdown table plugin can render cleanly.
func expandTableSpans(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent
	}

	changed := false
	var walkTables func(*html.Node)
	walkTables = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			if expandSingleTable(n) {
				changed = true
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walkTables(child)
		}
	}
	walkTables(doc)

	if !changed {
		return htmlContent
	}

	var buf strings.Builder
	html.Render(&buf, doc)
	result := buf.String()

	// html.Render wraps in <html><head></head><body>...</body></html>
	// Extract just the body content
	if bodyStart := strings.Index(result, "<body>"); bodyStart >= 0 {
		result = result[bodyStart+len("<body>"):]
		if bodyEnd := strings.LastIndex(result, "</body>"); bodyEnd >= 0 {
			result = result[:bodyEnd]
		}
	}

	return result
}

// expandSingleTable expands colspan/rowspan in a single table.
// Returns true if any spans were expanded.
func expandSingleTable(tableNode *html.Node) bool {
	// Find tbody (or use table directly)
	tbody := tableNode
	for child := tableNode.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && (child.Data == "tbody" || child.Data == "thead") {
			tbody = child
			break
		}
	}

	// Collect rows
	var rows []*html.Node
	for child := tbody.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "tr" {
			rows = append(rows, child)
		}
	}
	if len(rows) == 0 {
		return false
	}

	// Determine max columns by scanning all rows
	maxCols := 0
	for _, row := range rows {
		cols := 0
		for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
				cs := getAttrInt(cell, "colspan", 1)
				cols += cs
			}
		}
		if cols > maxCols {
			maxCols = cols
		}
	}
	if maxCols == 0 {
		return false
	}

	// Build a grid: grid[row][col] = *html.Node (the cell that occupies this position)
	// This accounts for rowspan/colspan properly
	numRows := len(rows)
	grid := make([][]*html.Node, numRows)
	for i := range grid {
		grid[i] = make([]*html.Node, maxCols)
	}

	// Fill the grid
	for rowIdx, row := range rows {
		colIdx := 0
		for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.Type != html.ElementNode || (cell.Data != "td" && cell.Data != "th") {
				continue
			}
			// Skip columns already occupied by rowspan from above
			for colIdx < maxCols && grid[rowIdx][colIdx] != nil {
				colIdx++
			}
			if colIdx >= maxCols {
				break
			}

			cs := getAttrInt(cell, "colspan", 1)
			rs := getAttrInt(cell, "rowspan", 1)

			// Fill grid positions for this cell's span
			for r := 0; r < rs && rowIdx+r < numRows; r++ {
				for c := 0; c < cs && colIdx+c < maxCols; c++ {
					grid[rowIdx+r][colIdx+c] = cell
				}
			}

			colIdx += cs
		}
	}

	// Check if any spans exist
	hasSpans := false
	for _, row := range rows {
		for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
			if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
				if getAttrInt(cell, "colspan", 1) > 1 || getAttrInt(cell, "rowspan", 1) > 1 {
					hasSpans = true
					break
				}
			}
		}
		if hasSpans {
			break
		}
	}
	if !hasSpans {
		return false
	}

	// Detect "content tables" — tables with a "Параметры сообщения" row.
	// For these tables, column 0 is a visual hierarchy spacer and should be dropped.
	isContentTable := false
	for rowIdx := 0; rowIdx < numRows; rowIdx++ {
		for colIdx := 0; colIdx < maxCols; colIdx++ {
			cell := grid[rowIdx][colIdx]
			if cell != nil {
				text := extractText(cell)
				if strings.Contains(text, "Параметры сообщения") {
					isContentTable = true
					break
				}
			}
		}
		if isContentTable {
			break
		}
	}

	// For content tables, drop the first column (empty hierarchy spacer)
	startCol := 0
	if isContentTable && maxCols > 1 {
		startCol = 1
	}

	// Rebuild each row with expanded cells
	for rowIdx, row := range rows {
		// Remove all existing cell children
		for {
			child := row.FirstChild
			if child == nil {
				break
			}
			row.RemoveChild(child)
		}

		// Add new cells from the grid (skipping startCol columns)
		for colIdx := startCol; colIdx < maxCols; colIdx++ {
			srcCell := grid[rowIdx][colIdx]
			newCell := &html.Node{
				Type: html.ElementNode,
				Data: "td",
			}
			if srcCell != nil {
				newCell.Data = srcCell.Data // preserve th/td
				// Copy class attribute only
				for _, attr := range srcCell.Attr {
					if attr.Key == "class" {
						newCell.Attr = append(newCell.Attr, attr)
						break
					}
				}
				// Deep-clone children from source cell
				for child := srcCell.FirstChild; child != nil; child = child.NextSibling {
					newCell.AppendChild(cloneNode(child))
				}
			}
			row.AppendChild(newCell)
		}
	}

	return true
}

// cloneNode deep-clones an HTML node tree
func cloneNode(n *html.Node) *html.Node {
	clone := &html.Node{
		Type:     n.Type,
		Data:     n.Data,
		DataAtom: n.DataAtom,
	}
	clone.Attr = make([]html.Attribute, len(n.Attr))
	copy(clone.Attr, n.Attr)
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		clone.AppendChild(cloneNode(child))
	}
	return clone
}

// getAttrInt returns an integer attribute value, or defaultVal if not found or invalid
func getAttrInt(n *html.Node, key string, defaultVal int) int {
	for _, attr := range n.Attr {
		if attr.Key == key {
			if v, err := strconv.Atoi(attr.Val); err == nil {
				return v
			}
		}
	}
	return defaultVal
}

// preProcessHTML cleans up Confluence HTML before markdown conversion.
// Strips elements that confuse the html-to-markdown converter.
func preProcessHTML(htmlContent string) string {
	// Flatten lists inside table cells — must be done before other transformations
	// because <ul>/<ol> in <td> breaks the table plugin entirely
	htmlContent = flattenListsInTableCells(htmlContent)

	// Expand colspan/rowspan into individual cells with duplicated content
	// This normalizes tables into uniform 2D grids the table plugin can render
	htmlContent = expandTableSpans(htmlContent)

	// Remove <colgroup>...</colgroup> — these are table column styling that
	// can prevent the table plugin from recognizing the table structure
	colgroupRe := regexp.MustCompile(`(?is)<colgroup[^>]*>.*?</colgroup>`)
	htmlContent = colgroupRe.ReplaceAllString(htmlContent, "")

	// Remove standalone <col .../> tags
	colRe := regexp.MustCompile(`(?i)<col\b[^>]*/?>`)
	htmlContent = colRe.ReplaceAllString(htmlContent, "")

	// Unwrap <div class="table-wrap"> — just a styling wrapper that can interfere
	// with table plugin recognition when divs are handled as block containers
	tableWrapOpenRe := regexp.MustCompile(`(?i)<div\s+class="table-wrap"\s*>`)
	htmlContent = tableWrapOpenRe.ReplaceAllString(htmlContent, "")

	// Unwrap <div class="content-wrapper"> — appears inside table cells,
	// block-level divs inside <td> break the table plugin
	contentWrapOpenRe := regexp.MustCompile(`(?i)<div\s+class="content-wrapper"\s*>`)
	htmlContent = contentWrapOpenRe.ReplaceAllString(htmlContent, "")

	// Remove the matching </div> for unwrapped divs is tricky with regex.
	// Instead, we rely on the converter handling orphan </div> gracefully.
	// But we should NOT remove ALL </div> tags — only the ones we unwrapped.
	// A safe approach: remove the specific div patterns and let the HTML parser
	// handle the extra </div> closers (they'll be ignored by the parser).

	// Unwrap <span style="color: ..."> — purely visual styling that adds no content
	// and clutters the markdown output. Keep the inner content.
	colorSpanRe := regexp.MustCompile(`(?i)<span\s+style="color:\s*rgb\([^)]*\)"\s*>`)
	htmlContent = colorSpanRe.ReplaceAllString(htmlContent, "")
	// Remove matching </span> for color spans — this is safe because we're just
	// unwrapping them. The extra </span> closers will be ignored by the HTML parser.

	return htmlContent
}

// fixAdjacentEmphasis fixes adjacent bold/italic markers that run together
// e.g., **text1****text2** → **text1text2** (merged)
// This happens when consecutive <strong> elements appear in Confluence storage format.
// We merge them instead of splitting to avoid breaking table rows with newlines.
func fixAdjacentEmphasis(markdown string) string {
	// Fix ****  (adjacent bold close+open) → merge into one bold span
	markdown = strings.ReplaceAll(markdown, "****", "")
	return markdown
}

// decodeHTMLEntities decodes remaining HTML entities in converted markdown.
// The converter may leave &lt; &gt; &amp; as-is in certain contexts.
func decodeHTMLEntities(markdown string) string {
	// Decode common HTML entities that appear in Confluence content
	// &lt; and &gt; are safe to decode: patterns like <КА-конверт> or <msgId>
	// won't be parsed as HTML by markdown renderers since they're not valid tag names
	markdown = strings.ReplaceAll(markdown, "&lt;", "<")
	markdown = strings.ReplaceAll(markdown, "&gt;", ">")
	markdown = strings.ReplaceAll(markdown, "&amp;", "&")
	markdown = strings.ReplaceAll(markdown, "&quot;", "\"")
	markdown = strings.ReplaceAll(markdown, "&#39;", "'")
	markdown = strings.ReplaceAll(markdown, "&apos;", "'")
	return markdown
}

// StorageToMarkdownAdvanced converts Confluence storage format to Markdown
// using the html-to-markdown library with Confluence-specific plugin support
func StorageToMarkdownAdvanced(storageContent string, baseURL string) (string, error) {
	// Pre-process HTML to remove elements that break the converter
	storageContent = preProcessHTML(storageContent)

	confluencePlugin := &ConfluencePlugin{}
	converter := html2md.NewConverter(
		html2md.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(
				// Preserve newlines in table cells instead of skipping tables
				table.WithNewlineBehavior(table.NewlineBehaviorPreserve),
			),
			confluencePlugin,
		),
	)

	markdown, err := converter.ConvertString(storageContent)
	if err != nil {
		return "", fmt.Errorf("failed to convert HTML to markdown: %w", err)
	}

	// Post-processing
	markdown = cleanUnderscoreEscaping(markdown)
	markdown = fixAdjacentEmphasis(markdown)
	markdown = decodeHTMLEntities(markdown)
	markdown = regenerateTOC(markdown)

	return markdown, nil
}
