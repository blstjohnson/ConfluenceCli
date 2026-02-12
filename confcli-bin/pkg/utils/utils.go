package utils

import (
	"fmt"
	"regexp"
	"strings"
)

// GetExtensionForFormat returns the file extension for a given format
func GetExtensionForFormat(format string) string {
	switch strings.ToLower(format) {
	case "html", "xhtml":
		return "html"
	case "storage", "xml":
		return "xml"
	case "wiki", "wikimarkup":
		return "wiki"
	case "markdown", "md":
		return "md"
	case "plain", "plaintext":
		return "txt"
	case "json":
		return "json"
	case "pdf":
		return "pdf"
	case "word", "docx":
		return "docx"
	case "excel", "xlsx":
		return "xlsx"
	case "powerpoint", "pptx":
		return "pptx"
	default:
		return "txt" // Default to text
	}
}

// IsValidURL checks if a string is a valid URL
func IsValidURL(url string) bool {
	// Basic URL validation using regex
	regex := `^(https?://)?([a-zA-Z0-9.-]+\.[a-zA-Z]{2,})(:[0-9]+)?(/.*)?$`
	re := regexp.MustCompile(regex)
	return re.MatchString(url)
}

// SanitizeFileName sanitizes a string to be used as a filename
func SanitizeFileName(name string) string {
	// Replace invalid characters with underscores
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, "*", "_")
	name = strings.ReplaceAll(name, "?", "_")
	name = strings.ReplaceAll(name, "\"", "_")
	name = strings.ReplaceAll(name, "<", "_")
	name = strings.ReplaceAll(name, ">", "_")
	name = strings.ReplaceAll(name, "|", "_")
	
	// Limit length to 255 characters
	if len(name) > 255 {
		name = name[:255]
	}
	
	return name
}

// FormatBytes converts bytes to human-readable format
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ContainsString checks if a slice contains a string
func ContainsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}