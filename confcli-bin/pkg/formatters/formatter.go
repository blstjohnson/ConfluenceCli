package formatters

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
	"confcli/pkg/models"
)

// FormatOutput formats the output based on the specified format
func FormatOutput(data interface{}, format string) error {
	switch format {
	case "json":
		return formatJSON(data)
	case "yaml":
		return formatYAML(data)
	case "text":
		fallthrough
	default:
		return formatText(data)
	}
}

// FormatOutputWithContent formats a page with its content based on the specified format
func FormatOutputWithContent(page *models.Page, content string, contentFormat string) error {
	switch contentFormat {
	case "json":
		data := map[string]interface{}{
			"page":    page,
			"content": content,
		}
		return formatJSON(data)
	case "yaml":
		data := map[string]interface{}{
			"page":    page,
			"content": content,
		}
		return formatYAML(data)
	case "edit", "editor":
		// For edit format, output just the raw editor content
		fmt.Print(content)
		return nil
	default:
		return formatPageWithContentText(page, content)
	}
}

// formatJSON formats the data as JSON
func formatJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// formatYAML formats the data as YAML
func formatYAML(data interface{}) error {
	encoder := yaml.NewEncoder(os.Stdout)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(data)
}

// formatText formats the data as human-readable text
func formatText(data interface{}) error {
	switch v := data.(type) {
	case *models.Page:
		return formatPageText(v)
	case []models.Page:
		return formatPagesText(v)
	case []*models.SearchResult:
		return formatSearchResultsText(v)
	case []models.SearchResult:
		return formatSearchResultsText(v)
	default:
		// For unknown types, fall back to JSON
		return formatJSON(data)
	}
}

// FormatOutputToString formats the output to a string based on the specified format
func FormatOutputToString(data interface{}, format string) (string, error) {
	switch format {
	case "json":
		return formatJSONToString(data)
	case "yaml":
		return formatYAMLToString(data)
	case "text":
		fallthrough
	default:
		return formatTextToString(data)
	}
}

// formatJSONToString formats the data as JSON string
func formatJSONToString(data interface{}) (string, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// formatYAMLToString formats the data as YAML string
func formatYAMLToString(data interface{}) (string, error) {
	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

// formatTextToString formats the data as human-readable text string
func formatTextToString(data interface{}) (string, error) {
	// For now, we'll just return JSON as a fallback
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// formatPageText formats a single page as human-readable text
func formatPageText(page *models.Page) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	pageID, _ := page.ID.Int()
	fmt.Fprintf(w, "ID:\t%d\n", pageID)
	fmt.Fprintf(w, "Title:\t%s\n", page.Title)
	fmt.Fprintf(w, "Space ID:\t%d\n", page.Space.ID)
	fmt.Fprintf(w, "Status:\t%s\n", page.Status)
	createdAt := page.CreatedAt()
	if !createdAt.IsZero() {
		fmt.Fprintf(w, "Created:\t%s\n", createdAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Fprintf(w, "Created:\tN/A\n")
	}
	updatedAt := page.UpdatedAt()
	if !updatedAt.IsZero() {
		fmt.Fprintf(w, "Updated:\t%s\n", updatedAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Fprintf(w, "Updated:\tN/A\n")
	}
	fmt.Fprintf(w, "Version:\t%d\n", page.Version.Number)

	if len(page.Labels) > 0 {
		labels := make([]string, len(page.Labels))
		for i, label := range page.Labels {
			labels[i] = label.Name
		}
		fmt.Fprintf(w, "Labels:\t%s\n", fmt.Sprintf("[%s]", joinStrings(labels, ", ")))
	}

	if len(page.Ancestors) > 0 {
		ancestorTitles := make([]string, len(page.Ancestors))
		for i, ancestor := range page.Ancestors {
			ancestorTitles[i] = ancestor.Title
		}
		fmt.Fprintf(w, "Ancestors:\t%s\n", fmt.Sprintf("[%s]", joinStrings(ancestorTitles, " > ")))
	}

	if len(page.Comments) > 0 {
		fmt.Fprintf(w, "Comments:\t%d\n", len(page.Comments))
	}

	if len(page.Attachments) > 0 {
		fmt.Fprintf(w, "Attachments:\t%d\n", len(page.Attachments))
	}

	fmt.Fprintln(w, "\nContent Preview:")
	content := getPageContentPreview(page)
	if len(content) > 200 {
		content = content[:200] + "..."
	}
	fmt.Fprintln(w, content)

	return w.Flush()
}

// formatPageWithContentText formats a single page with full content as human-readable text
func formatPageWithContentText(page *models.Page, content string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	pageID, _ := page.ID.Int()
	fmt.Fprintf(w, "ID:\t%d\n", pageID)
	fmt.Fprintf(w, "Title:\t%s\n", page.Title)
	fmt.Fprintf(w, "Space ID:\t%d\n", page.Space.ID)
	fmt.Fprintf(w, "Status:\t%s\n", page.Status)
	createdAt := page.CreatedAt()
	if !createdAt.IsZero() {
		fmt.Fprintf(w, "Created:\t%s\n", createdAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Fprintf(w, "Created:\tN/A\n")
	}
	updatedAt := page.UpdatedAt()
	if !updatedAt.IsZero() {
		fmt.Fprintf(w, "Updated:\t%s\n", updatedAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Fprintf(w, "Updated:\tN/A\n")
	}
	fmt.Fprintf(w, "Version:\t%d\n", page.Version.Number)

	if len(page.Labels) > 0 {
		labels := make([]string, len(page.Labels))
		for i, label := range page.Labels {
			labels[i] = label.Name
		}
		fmt.Fprintf(w, "Labels:\t%s\n", fmt.Sprintf("[%s]", joinStrings(labels, ", ")))
	}

	if len(page.Ancestors) > 0 {
		ancestorTitles := make([]string, len(page.Ancestors))
		for i, ancestor := range page.Ancestors {
			ancestorTitles[i] = ancestor.Title
		}
		fmt.Fprintf(w, "Ancestors:\t%s\n", fmt.Sprintf("[%s]", joinStrings(ancestorTitles, " > ")))
	}

	if len(page.Comments) > 0 {
		fmt.Fprintf(w, "Comments:\t%d\n", len(page.Comments))
	}

	if len(page.Attachments) > 0 {
		fmt.Fprintf(w, "Attachments:\t%d\n", len(page.Attachments))
	}

	fmt.Fprintln(w, "\nContent:")
	fmt.Fprintln(w, content)

	return w.Flush()
}

// formatPagesText formats a slice of pages as human-readable text
func formatPagesText(pages []models.Page) error {
	if len(pages) == 0 {
		fmt.Println("No pages found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tTITLE\tSPACE ID\tVERSION\tUPDATED\n")
	fmt.Fprintf(w, "--\t-----\t--------\t-------\t-------\n")

	for _, page := range pages {
		pageID, _ := page.ID.Int()
		updatedAt := page.UpdatedAt()
		updatedStr := "N/A"
		if !updatedAt.IsZero() {
			updatedStr = updatedAt.Format("2006-01-02")
		}
		fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%s\n",
			pageID,
			page.Title,
			page.Space.ID,
			page.Version.Number,
			updatedStr)
	}

	return w.Flush()
}

// formatSearchResultsText formats search results as human-readable text
func formatSearchResultsText(results interface{}) error {
	var searchResults []models.SearchResult

	switch v := results.(type) {
	case []*models.SearchResult:
		searchResults = make([]models.SearchResult, len(v))
		for i, r := range v {
			searchResults[i] = *r
		}
	case []models.SearchResult:
		searchResults = v
	default:
		fmt.Println("No search results found.")
		return nil
	}

	if len(searchResults) == 0 {
		fmt.Println("No search results found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tTITLE\tTYPE\tSPACE\n")
	fmt.Fprintf(w, "--\t-----\t----\t-----\n")

	for _, result := range searchResults {
		spaceKey := ""
		if result.Space.Key != "" {
			spaceKey = result.Space.Key
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n",
			result.ID,
			result.Title,
			result.Type,
			spaceKey)
	}

	return w.Flush()
}

// getPageContentPreview extracts a preview of the page content
func getPageContentPreview(page *models.Page) string {
	if page.Body == nil {
		return ""
	}

	// Try different content representations in order of preference
	for _, format := range []string{"storage", "view", "export_view", "styled_view"} {
		if content, exists := page.Body[format]; exists {
			if contentMap, ok := content.(map[string]interface{}); ok {
				if value, ok := contentMap["value"].(string); ok {
					return value
				}
			}
		}
	}

	return ""
}

// joinStrings joins a slice of strings with a separator
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}