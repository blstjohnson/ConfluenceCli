package converters

import (
	"fmt"
	"regexp"
	"strings"
)

// StorageToMarkdown converts Confluence storage format to Markdown
// This is the basic implementation using regex-based conversion
// For better conversion with support for Confluence macros (info/warning/note/tip panels,
// status macros, expand macros, code blocks, user mentions), use StorageToMarkdownAdvanced
func StorageToMarkdown(storageContent string, baseURL string) (string, error) {
	// This is a basic implementation that handles common Confluence storage elements
	// In a real implementation, we would use a proper HTML to Markdown converter
	// since Confluence storage format is essentially XML/HTML-based

	result := storageContent

	// Remove Confluence-specific macros and tags first
	result = removeConfluenceMacros(result, baseURL)

	// Convert headings
	reH1 := regexp.MustCompile(`<h1>(.*?)</h1>`)
	result = reH1.ReplaceAllString(result, "# $1")

	reH2 := regexp.MustCompile(`<h2>(.*?)</h2>`)
	result = reH2.ReplaceAllString(result, "## $1")

	reH3 := regexp.MustCompile(`<h3>(.*?)</h3>`)
	result = reH3.ReplaceAllString(result, "### $1")

	reH4 := regexp.MustCompile(`<h4>(.*?)</h4>`)
	result = reH4.ReplaceAllString(result, "#### $1")

	reH5 := regexp.MustCompile(`<h5>(.*?)</h5>`)
	result = reH5.ReplaceAllString(result, "##### $1")

	reH6 := regexp.MustCompile(`<h6>(.*?)</h6>`)
	result = reH6.ReplaceAllString(result, "###### $1")

	// Convert bold
	reBold := regexp.MustCompile(`<strong>(.*?)</strong>`)
	result = reBold.ReplaceAllString(result, "**$1**")

	// Convert italic
	reItalic := regexp.MustCompile(`<em>(.*?)</em>`)
	result = reItalic.ReplaceAllString(result, "*$1*")

	// Convert links
	reLink := regexp.MustCompile(`<a href="(.*?)">(.*?)</a>`)
	result = reLink.ReplaceAllString(result, "[$2]($1)")

	// Convert paragraphs (simplified)
	reP := regexp.MustCompile(`</p>\s*<p>`)
	result = reP.ReplaceAllString(result, "\n\n")

	// Clean up paragraph tags
	result = strings.ReplaceAll(result, "<p>", "")
	result = strings.ReplaceAll(result, "</p>", "")

	// Convert line breaks
	result = strings.ReplaceAll(result, "<br>", "\n")
	result = strings.ReplaceAll(result, "<br/>", "\n")
	result = strings.ReplaceAll(result, "<br />", "\n")

	// Convert unordered lists
	result = strings.ReplaceAll(result, "<ul>", "")
	result = strings.ReplaceAll(result, "</ul>", "")
	result = strings.ReplaceAll(result, "<ol>", "")
	result = strings.ReplaceAll(result, "</ol>", "")

	// Convert list items
	reLi := regexp.MustCompile(`<li>(.*?)</li>`)
	result = reLi.ReplaceAllString(result, "- $1\n")

	// Remove remaining HTML tags
	result = stripHTMLTags(result)

	// Remove extra whitespace
	result = regexp.MustCompile(`\n\s*\n`).ReplaceAllString(result, "\n\n")

	return result, nil
}

// removeConfluenceMacros removes Confluence-specific XML macros and tags
func removeConfluenceMacros(content string, baseURL string) string {
	result := content

	// Convert <ac:link> to Markdown links
	result = convertAcLinks(result, baseURL)

	// Remove <ri:page> and related tags (after processing links)
	reRiPage := regexp.MustCompile(`<ri:page[^>]*>`)
	result = reRiPage.ReplaceAllString(result, "")

	reRiSpace := regexp.MustCompile(`<ri:space[^>]*>`)
	result = reRiSpace.ReplaceAllString(result, "")

	reRiUser := regexp.MustCompile(`<ri:user[^>]*>`)
	result = reRiUser.ReplaceAllString(result, "")

	reRiLabel := regexp.MustCompile(`<ri:label[^>]*>`)
	result = reRiLabel.ReplaceAllString(result, "")

	// Remove <ac:emoticon> tags
	reEmoticon := regexp.MustCompile(`<ac:emoticon[^>]*>.*?</ac:emoticon>`)
	result = reEmoticon.ReplaceAllString(result, "")

	// Remove <ac:image> tags
	reImage := regexp.MustCompile(`(?s)<ac:image[^>]*>.*?</ac:image>`)
	result = reImage.ReplaceAllString(result, "[image]")

	// Remove <ac:structured-macro> tags (including content)
	reMacro := regexp.MustCompile(`(?s)<ac:structured-macro[^>]*>.*?</ac:structured-macro>`)
	result = reMacro.ReplaceAllString(result, "")

	// Remove <ac:parameter> tags
	reParameter := regexp.MustCompile(`(?s)<ac:parameter[^>]*>.*?</ac:parameter>`)
	result = reParameter.ReplaceAllString(result, "")

	// Remove <span> tags but keep content
	reSpan := regexp.MustCompile(`<span[^>]*>(.*?)</span>`)
	result = reSpan.ReplaceAllString(result, "$1")

	// Remove <pre> tags but keep content
	rePre := regexp.MustCompile(`<pre[^>]*>(.*?)</pre>`)
	result = rePre.ReplaceAllString(result, "$1")

	// Remove <code> tags but keep content
	reCode := regexp.MustCompile(`<code[^>]*>(.*?)</code>`)
	result = reCode.ReplaceAllString(result, "`$1`")

	// Remove <div> tags but keep content
	reDiv := regexp.MustCompile(`<div[^>]*>(.*?)</div>`)
	result = reDiv.ReplaceAllString(result, "$1")

	// Remove any remaining ac:* tags
	reAcTags := regexp.MustCompile(`<ac:[^>]+>`)
	result = reAcTags.ReplaceAllString(result, "")

	// Remove any remaining ri:* tags
	reRiTags := regexp.MustCompile(`<ri:[^>]+>`)
	result = reRiTags.ReplaceAllString(result, "")

	return result
}

// convertAcLinks converts Confluence <ac:link> tags to Markdown links
func convertAcLinks(content string, baseURL string) string {
	result := content

	// Pattern to match ac:link with plain-text-link-body
	// Example: <ac:link><ri:page ri:content-id="123" ri:space-key="SPACE"/><ac:plain-text-link-body><![CDATA[Link Text]]></ac:plain-text-link-body></ac:link>
	reLinkWithBody := regexp.MustCompile(`(?s)<ac:link[^>]*>.*?<ri:page[^>]*ri:content-id="([^"]*)"[^>]*>.*?<ac:plain-text-link-body><!\[CDATA\[(.*?)\]\]></ac:plain-text-link-body>.*?</ac:link>`)
	result = reLinkWithBody.ReplaceAllStringFunc(result, func(match string) string {
		submatches := reLinkWithBody.FindStringSubmatch(match)
		if len(submatches) >= 3 {
			pageID := submatches[1]
			linkText := submatches[2]
			// Return Markdown link with full Confluence URL
			return fmt.Sprintf("[%s](%s/spaces/~%s)", linkText, baseURL, pageID)
		}
		return match
	})

	// Pattern for ac:link without body (just ri:page reference)
	reLinkSimple := regexp.MustCompile(`<ac:link[^>]*><ri:page[^>]*ri:content-id="([^"]*)"[^>]*></ac:link>`)
	result = reLinkSimple.ReplaceAllString(result, fmt.Sprintf("[Link](%s/spaces/~%%s)", baseURL))

	// Pattern for ac:link with ri:space-key but no content-id
	reLinkWithSpace := regexp.MustCompile(`<ac:link[^>]*><ri:page[^>]*ri:space-key="([^"]*)"[^>]*></ac:link>`)
	result = reLinkWithSpace.ReplaceAllStringFunc(result, func(match string) string {
		submatches := reLinkWithSpace.FindStringSubmatch(match)
		if len(submatches) >= 2 {
			spaceKey := submatches[1]
			return fmt.Sprintf("[%s](%s/spaces/%s)", spaceKey, baseURL, spaceKey)
		}
		return match
	})

	return result
}

// stripHTMLTags removes any remaining HTML tags from the content
func stripHTMLTags(content string) string {
	// Remove any remaining HTML tags
	reHTML := regexp.MustCompile(`<[^>]*>`)
	result := reHTML.ReplaceAllString(content, "")

	// Decode common HTML entities
	result = strings.ReplaceAll(result, "&nbsp;", " ")
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#39;", "'")
	result = strings.ReplaceAll(result, "&apos;", "'")

	return result
}

// MarkdownToStorage converts Markdown to Confluence storage format
func MarkdownToStorage(markdownContent string) (string, error) {
	// This is a simplified implementation
	// In a real implementation, we would use a proper Markdown to HTML converter
	// and wrap it in Confluence storage format XML

	// For now, we'll return the content as-is with a note
	return fmt.Sprintf("Markdown to Storage conversion not fully implemented.\nOriginal content:\n%s", markdownContent), nil
}

// ExportViewToMarkdown converts Confluence export_view format to Markdown
// export_view is a cleaner HTML representation meant for exporting, with fewer
// Confluence-specific macros and more standard HTML elements
// Uses the same advanced converter as StorageToMarkdownAdvanced for consistent table handling
func ExportViewToMarkdown(exportViewContent string, baseURL string) (string, error) {
	// Use the advanced converter which handles HTML to markdown conversion
	// with support for Confluence-specific elements and proper table formatting
	return StorageToMarkdownAdvanced(exportViewContent, baseURL)
}