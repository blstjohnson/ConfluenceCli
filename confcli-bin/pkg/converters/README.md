# Converters Package

This package provides conversion utilities for Confluence content, primarily for converting Confluence storage format (XML/HTML-based) to Markdown.

## Available Converters

### Basic Converter: `StorageToMarkdown`

The basic regex-based converter that handles:
- HTML headings (h1-h6)
- Bold and italic text
- Links
- Lists (ordered and unordered)
- Basic HTML tag stripping

**Limitations:**
- Doesn't handle Confluence-specific macros well
- May not preserve all formatting
- Limited support for complex Confluence elements

### Advanced Converter: `StorageToMarkdownAdvanced`

The advanced converter built on top of the `html-to-markdown/v2` library with a custom Confluence plugin. This is the **recommended** converter for most use cases.

**Supports:**
- ✅ Info/Warning/Note/Tip panels → GitHub-style alerts (`> [!NOTE]`)
- ✅ Status macros → Emoji badges (🔴 🟡 🟢 🔵 ⚪)
- ✅ Expand macros → HTML `<details>` elements
- ✅ Code blocks with syntax highlighting
- ✅ User mentions → `@DisplayName`
- ✅ TOC macros (skipped gracefully)
- ✅ All standard HTML elements (via commonmark plugin)

**Usage:**
```go
import "confcli/pkg/converters"

markdown, err := converters.StorageToMarkdownAdvanced(storageContent, baseURL)
if err != nil {
    // handle error
}
```

## Architecture

The advanced converter uses:
- [`html-to-markdown/v2`](https://github.com/JohannesKaufmann/html-to-markdown) - Core HTML to Markdown conversion library
- Custom `ConfluencePlugin` - Handles Confluence-specific XML elements and macros

## Future Enhancements

Potential improvements for the advanced converter:
- Image download and embedding
- Mermaid diagram extraction from draw.io
- Jira issue link conversion
- Page properties extraction for frontmatter
- Attachment handling

## References

- [confluence-md](https://github.com/jackchuka/confluence-md) - Inspiration for the advanced converter
- [html-to-markdown/v2](https://github.com/JohannesKaufmann/html-to-markdown) - Core conversion library
