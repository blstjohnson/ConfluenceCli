package converters

import (
	"fmt"
	"strings"

	html2md "github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

// ConfluencePlugin implements html-to-markdown plugin for Confluence-specific elements
type ConfluencePlugin struct{}

// Name returns the plugin name
func (p *ConfluencePlugin) Name() string {
	return "confluence"
}

// Init registers Confluence-specific HTML element handlers
func (p *ConfluencePlugin) Init(conv *html2md.Converter) error {
	// Handle Confluence structured macros (info/warning/note/tip panels, status, expand, code, toc)
	conv.Register.RendererFor("ac:structured-macro", html2md.TagTypeBlock, p.handleMacro, html2md.PriorityStandard)
	
	// Handle user mentions
	conv.Register.RendererFor("span", html2md.TagTypeInline, p.handleUserMention, html2md.PriorityStandard)
	
	// Handle expand macro div containers
	conv.Register.RendererFor("div", html2md.TagTypeBlock, p.handleExpandDiv, html2md.PriorityStandard)
	
	return nil
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

	// Get content from ac:rich-text-body
	var content string
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "ac:rich-text-body" {
			content = p.getNodeText(child)
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
				content = strings.TrimSpace(p.getNodeText(child))
			}
		}
	}

	w.WriteString(fmt.Sprintf("<details>\n<summary>%s</summary>\n\n%s\n\n</details>\n", summary, content))
	return html2md.RenderSuccess
}

// handleExpandDiv handles expand macro div containers (alternative format)
func (p *ConfluencePlugin) handleExpandDiv(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	// Check if this is an expand container
	isExpand := false
	for _, attr := range n.Attr {
		if attr.Key == "data-macro-name" && attr.Val == "expand" {
			isExpand = true
			break
		}
	}
	if !isExpand {
		return html2md.RenderTryNext
	}

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
						content = strings.TrimSpace(p.getNodeText(child))
						break
					}
				}
			}
		}
	}

	w.WriteString(fmt.Sprintf("<details>\n<summary>%s</summary>\n\n%s\n\n</details>\n", summary, content))
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

// handleUserMention handles user mentions
func (p *ConfluencePlugin) handleUserMention(ctx html2md.Context, w html2md.Writer, n *html.Node) html2md.RenderStatus {
	// Check if this is a user mention
	isUserMention := false
	for _, attr := range n.Attr {
		if attr.Key == "data-account-id" || attr.Key == "data-user-key" {
			isUserMention = true
			break
		}
	}
	if !isUserMention {
		return html2md.RenderTryNext
	}

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

// getNodeText extracts text content from an HTML node
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

// StorageToMarkdownAdvanced converts Confluence storage format to Markdown
// using the html-to-markdown library with Confluence-specific plugin support
// This provides better conversion than the basic implementation, handling:
// - Info/Warning/Note/Tip panels
// - Status macros
// - Expand macros
// - Code blocks with syntax highlighting
// - User mentions
// - TOC macros (skipped)
// - Tables with proper markdown formatting (including colspan/rowspan support)
func StorageToMarkdownAdvanced(storageContent string, baseURL string) (string, error) {
	converter := html2md.NewConverter(
		html2md.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(
				// Mirror spanned cells to preserve content (instead of leaving them empty)
				table.WithSpanCellBehavior(table.SpanBehaviorMirror),
				// Preserve newlines in table cells instead of skipping tables
				table.WithNewlineBehavior(table.NewlineBehaviorPreserve),
			),
			&ConfluencePlugin{},
		),
	)

	markdown, err := converter.ConvertString(storageContent)
	if err != nil {
		return "", fmt.Errorf("failed to convert HTML to markdown: %w", err)
	}

	return markdown, nil
}
