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
	default:
		return "txt"
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
			storageContent, err := apiClient.GetPageContent(ctx, page.ID, "storage")
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
			var content string
			var err error

			if format == "markdown" {
				// Get storage format and convert to markdown
				storageContent, err := apiClient.GetPageContent(ctx, page.ID, "storage")
				if err != nil {
					return fmt.Errorf("failed to get page content: %w", err)
				}
				content, err = converters.StorageToMarkdown(storageContent, baseURL)
				if err != nil {
					return fmt.Errorf("failed to convert to markdown: %w", err)
				}
			} else {
				content, err = apiClient.GetPageContent(ctx, page.ID, format)
				if err != nil {
					// Try storage as fallback
					content, err = apiClient.GetPageContent(ctx, page.ID, "storage")
					if err != nil {
						return fmt.Errorf("failed to get page content: %w", err)
					}
				}
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