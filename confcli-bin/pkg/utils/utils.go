package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"confcli/pkg/converters"
	"confcli/pkg/models"
	"confcli/pkg/api"
	"confcli/pkg/formatters"
)

// SanitizeFilename removes invalid characters from filenames
func SanitizeFilename(filename string) string {
	// Replace invalid characters with underscores
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")
	filename = strings.ReplaceAll(filename, ":", "_")
	filename = strings.ReplaceAll(filename, "*", "_")
	filename = strings.ReplaceAll(filename, "?", "_")
	filename = strings.ReplaceAll(filename, "\"", "_")
	filename = strings.ReplaceAll(filename, "<", "_")
	filename = strings.ReplaceAll(filename, ">", "_")
	filename = strings.ReplaceAll(filename, "|", "_")

	// Limit length
	if len(filename) > 100 {
		filename = filename[:100]
	}

	return filename
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