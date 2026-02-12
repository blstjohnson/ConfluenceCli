package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"confcli/pkg/models"
	"confcli/pkg/api"
	"confcli/internal/formatter"
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
	default:
		return "txt"
	}
}

// ExportPageToFile exports a single page to a file
func ExportPageToFile(apiClient api.Client, page models.Page, baseDir, format string, skipContent bool) error {
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
			for _, format := range []string{"markdown", "storage"} {
				content, err := apiClient.GetPageContent(ctx, page.ID, format)
				if err != nil {
					// Try alternative format if primary fails
					content, err = apiClient.GetPageContent(ctx, page.ID, "storage")
					if err != nil {
						return fmt.Errorf("failed to get page content: %w", err)
					}
				}

				ext := GetExtensionForFormat(format)
				contentFile := filepath.Join(pageDir, fmt.Sprintf("index.%s", ext))
				if err := os.WriteFile(contentFile, []byte(content), 0644); err != nil {
					return fmt.Errorf("failed to write content file: %w", err)
				}
			}
		} else {
			// Export in single format
			content, err := apiClient.GetPageContent(ctx, page.ID, format)
			if err != nil {
				// Try storage as fallback
				content, err = apiClient.GetPageContent(ctx, page.ID, "storage")
				if err != nil {
					return fmt.Errorf("failed to get page content: %w", err)
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
		"spaceId":   page.SpaceID,
		"status":    page.Status,
		"createdAt": page.CreatedAt,
		"updatedAt": page.UpdatedAt,
		"version":   page.Version,
		"authorId":  page.AuthorID,
	}

	metadataBytes, err := formatter.FormatOutputToString(metadata, "json")
	if err != nil {
		return fmt.Errorf("failed to format page metadata: %w", err)
	}

	metadataFile := filepath.Join(pageDir, "metadata.json")
	if err := os.WriteFile(metadataFile, []byte(metadataBytes), 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}