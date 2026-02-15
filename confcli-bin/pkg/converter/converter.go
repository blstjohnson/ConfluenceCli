package converter

import (
	"fmt"
	"regexp"
	"strings"
)

// StorageToMarkdown converts Confluence storage format to Markdown
func StorageToMarkdown(storageContent string) (string, error) {
	// This is a basic implementation that handles common Confluence storage elements
	// In a real implementation, we would use a proper HTML to Markdown converter
	// since Confluence storage format is essentially XML/HTML-based
	
	result := storageContent
	
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
	
	// Remove extra whitespace
	result = regexp.MustCompile(`\n\s*\n`).ReplaceAllString(result, "\n\n")
	
	return result, nil
}

// MarkdownToStorage converts Markdown to Confluence storage format
func MarkdownToStorage(markdownContent string) (string, error) {
	// This is a simplified implementation
	// In a real implementation, we would use a proper Markdown to HTML converter
	// and wrap it in Confluence storage format XML

	// For now, we'll return the content as-is with a note
	return fmt.Sprintf("Markdown to Storage conversion not fully implemented.\nOriginal content:\n%s", markdownContent), nil
}