package models

// Space represents a Confluence space
type Space struct {
	ID          int                    `json:"id,omitempty"`
	Key         string                 `json:"key,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Description map[string]interface{} `json:"description,omitempty"`
	HomepageID  int                    `json:"homepageId,omitempty"`
	Status      string                 `json:"status,omitempty"`
	Links       map[string]string      `json:"_links,omitempty"`
}

// SearchResult represents a search result
type SearchResult struct {
	ID        int    `json:"id,omitempty"`
	Title     string `json:"title,omitempty"`
	Type      string `json:"type,omitempty"`
	Space     Space  `json:"space,omitempty"`
	Content   Page   `json:"content,omitempty"`
	Excerpt   string `json:"excerpt,omitempty"`
	URL       string `json:"url,omitempty"`
	FriendlyURL string `json:"friendlyUrl,omitempty"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs,omitempty"`
}

// Breadcrumb represents a breadcrumb in search results
type Breadcrumb struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}