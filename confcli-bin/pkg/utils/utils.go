package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gosimple/slug"

	"confcli/pkg/api"
	"confcli/pkg/converters"
	"confcli/pkg/formatters"
	"confcli/pkg/models"
)

// SanitizeFilename transliterates Unicode characters (e.g., Cyrillic to Latin)
// and produces a short, ASCII-safe, lowercase filename suitable for all platforms.
// Uses hyphens as separators (slug format). Limits to 80 chars to leave room
// for pageID prefix and extension within Windows MAX_PATH (260).
func SanitizeFilename(filename string) string {
	result := slug.Make(filename)
	if result == "" {
		result = "untitled"
	}
	if len(result) > 80 {
		result = result[:80]
		// Trim trailing hyphen if we cut mid-word
		result = strings.TrimRight(result, "-")
	}
	return result
}

// SanitizeFilenameNoLimit is like SanitizeFilename but does not truncate the result.
func SanitizeFilenameNoLimit(filename string) string {
	result := slug.Make(filename)
	if result == "" {
		result = "untitled"
	}
	return result
}

// SanitizeFilenameKeepCase transliterates Unicode but preserves the original casing.
// Useful for display-oriented filenames.
func SanitizeFilenameKeepCase(filename string) string {
	// slug.Make lowercases; we do manual transliteration keeping case
	// Replace invalid filesystem characters with hyphens
	invalidChars := regexp.MustCompile(`[/\\:*?"<>|]`)
	result := invalidChars.ReplaceAllString(filename, "-")

	// Collapse multiple hyphens
	result = regexp.MustCompile(`-{2,}`).ReplaceAllString(result, "-")
	result = strings.Trim(result, "-")

	if result == "" {
		result = "untitled"
	}
	if len(result) > 80 {
		result = result[:80]
		result = strings.TrimRight(result, "-")
	}
	return result
}

// SanitizeFilenameSimple sanitizes a filename without any transliteration.
// It removes all forbidden filesystem characters (\ / : * ? " < > |),
// replaces dots (.) with underscores (_) — since the title part carries no
// file extension — replaces spaces with underscores, and collapses runs of
// multiple underscores into a single one.
// The result is limited to 80 characters to stay within Windows MAX_PATH.
func SanitizeFilenameSimple(filename string) string {
	return sanitizeSimpleCore(filename, false)
}

// SanitizeFilenameSimpleNoLimit is like SanitizeFilenameSimple but does not
// truncate the result to 80 characters.
func SanitizeFilenameSimpleNoLimit(filename string) string {
	return sanitizeSimpleCore(filename, true)
}

// sanitizeSimpleCore is the shared implementation for the Simple sanitizers.
func sanitizeSimpleCore(filename string, noLimit bool) string {
	// Remove forbidden characters on Windows (and generally unsafe on all OSes)
	forbidden := regexp.MustCompile(`[\\/:*?"<>|]`)
	result := forbidden.ReplaceAllString(filename, "")

	// Replace dots with underscores (no file extension inside title)
	result = strings.ReplaceAll(result, ".", "_")

	// Replace spaces with underscores
	result = strings.ReplaceAll(result, " ", "_")

	// Replace hyphens with underscores
	result = strings.ReplaceAll(result, "-", "_")

	// Collapse consecutive underscores into a single one
	result = regexp.MustCompile(`_{2,}`).ReplaceAllString(result, "_")

	// Trim leading / trailing underscores
	result = strings.Trim(result, "_")

	if result == "" {
		result = "untitled"
	}
	if !noLimit && len(result) > 80 {
		result = result[:80]
		result = strings.TrimRight(result, "_")
	}
	return result
}

// StripHTMLTags removes HTML tags from a string
func StripHTMLTags(html string) string {
	// Compile regex to match HTML tags
	re := regexp.MustCompile("<[^>]*>")
	return re.ReplaceAllString(html, "")
}

// GetExtensionForFormat returns the file extension for a given format
func GetExtensionForFormat(format string) string {
	switch format {
	case "markdown", "md":
		return "md"
	case "storage":
		return "storage"
	case "html":
		return "html"
	case "plain":
		return "txt"
	case "edit", "editor":
		return "editor"
	case "export", "export_view":
		return "md"
	default:
		return "txt"
	}
}

// ConvertContentFromStorage converts storage format to the requested format
// Confluence supports "storage", "editor", and "export_view" formats natively
// For other formats, we convert from storage
func ConvertContentFromStorage(storageContent, format, baseURL string) (string, error) {
	switch format {
	case "storage", "html":
		// Storage format is already HTML-based, return as-is
		return storageContent, nil
	case "edit", "editor":
		// Editor format should be fetched directly, not converted from storage
		// This function assumes storage input, so return as-is for editor
		return storageContent, nil
	case "export", "export_view":
		// Export view is already HTML-based (cleaner than storage), return as-is
		// The conversion to markdown will be done separately
		return storageContent, nil
	case "markdown", "md":
		// Use advanced converter with support for Confluence macros
		// Content may come from export_view or storage, both work with the converter
		return converters.StorageToMarkdownAdvanced(storageContent, baseURL)
	case "plain":
		return StripHTMLTags(storageContent), nil
	default:
		// For unknown formats, return storage as-is
		return storageContent, nil
	}
}

// GetContentFormatForAPI returns the appropriate format to request from Confluence API
// Confluence supports "storage", "editor", and "export_view" formats natively
func GetContentFormatForAPI(requestedFormat string) string {
	switch requestedFormat {
	case "edit":
		return "editor"
	case "export", "export_view":
		return "export_view"
	default:
		// For all other formats, fetch storage and convert later
		return "storage"
	}
}

// ExportPageToFile exports a single page to a file
func ExportPageToFile(apiClient api.Client, page models.Page, baseDir, format string, skipContent bool, baseURL string) error {
	ctx := context.Background()

	// Create directory for the page
	pageID, _ := page.ID.Int()
	pageDir := filepath.Join(baseDir, fmt.Sprintf("%d_%s", pageID, SanitizeFilename(page.Title)))
	if err := os.MkdirAll(pageDir, 0755); err != nil {
		return fmt.Errorf("failed to create page directory: %w", err)
	}

	// Export content in requested format(s)
	if !skipContent {
		if format == "both" {
			// Export in both markdown and storage formats
			// Get storage format first
			storageContent, err := apiClient.GetPageContent(ctx, page.ID, "storage", 0)
			if err != nil {
				return fmt.Errorf("failed to get page content: %w", err)
			}

			// Write storage format
			storageFile := filepath.Join(pageDir, "index.storage")
			if err := os.WriteFile(storageFile, []byte(storageContent), 0644); err != nil {
				return fmt.Errorf("failed to write storage content file: %w", err)
			}

			// Convert to markdown and write
			markdownContent, err := converters.StorageToMarkdown(storageContent, baseURL)
			if err != nil {
				return fmt.Errorf("failed to convert to markdown: %w", err)
			}
			markdownFile := filepath.Join(pageDir, "index.md")
			if err := os.WriteFile(markdownFile, []byte(markdownContent), 0644); err != nil {
				return fmt.Errorf("failed to write markdown content file: %w", err)
			}
		} else {
			// Export in single format
			// Get content format - Confluence only supports "storage" and "editor" formats
			apiFormat := GetContentFormatForAPI(format)
			storageContent, err := apiClient.GetPageContent(ctx, page.ID, apiFormat, 0)
			if err != nil {
				return fmt.Errorf("failed to get page content: %w", err)
			}

			// Convert from storage to requested format if needed
			content, err := ConvertContentFromStorage(storageContent, format, baseURL)
			if err != nil {
				return fmt.Errorf("failed to convert content: %w", err)
			}

			ext := GetExtensionForFormat(format)
			contentFile := filepath.Join(pageDir, fmt.Sprintf("index.%s", ext))
			if err := os.WriteFile(contentFile, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write content file: %w", err)
			}
		}
	}

	// Export metadata
	metadata := map[string]interface{}{
		"id":        page.ID,
		"title":     page.Title,
		"spaceId":   page.Space.ID,
		"status":    page.Status,
		"createdAt": page.CreatedAt(),
		"updatedAt": page.UpdatedAt(),
		"version":   page.Version,
	}

	metadataBytes, err := formatters.FormatOutputToString(metadata, "json")
	if err != nil {
		return fmt.Errorf("failed to format page metadata: %w", err)
	}

	metadataFile := filepath.Join(pageDir, "metadata.json")
	if err := os.WriteFile(metadataFile, []byte(metadataBytes), 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}
