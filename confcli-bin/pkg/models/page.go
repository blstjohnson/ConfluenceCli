package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// PageID represents a Confluence page ID that can be either string or int
type PageID struct {
	Value interface{}
}

func (p *PageID) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as integer first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		p.Value = i
		return nil
	}

	// If that fails, try to unmarshal as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		p.Value = s
		return nil
	}

	// If both fail, assign the raw data as interface{}
	p.Value = data
	return nil
}

func (p PageID) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Value)
}

func (p PageID) String() string {
	switch v := p.Value.(type) {
	case int:
		return fmt.Sprintf("%d", v)
	case string:
		return v
	default:
		return ""
	}
}

func (p PageID) Int() (int, bool) {
	switch v := p.Value.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	case string:
		var i int
		_, err := fmt.Sscanf(v, "%d", &i)
		if err == nil {
			return i, true
		}
		return 0, false
	}
	return 0, false
}

// IntOrString returns the ID as an integer if it's stored as an integer,
// otherwise returns it as a string representation
func (p PageID) IntOrString() interface{} {
	if v, ok := p.Value.(int); ok {
		return v
	}
	return p.String()
}

// Page represents a Confluence page
type Page struct {
	ID        PageID                 `json:"id,omitempty"`
	Type      string                 `json:"type,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Title     string                 `json:"title,omitempty"`
	Space     Space                  `json:"space,omitempty"`
	Body      map[string]interface{} `json:"body,omitempty"`
	Version   Version                `json:"version,omitempty"`
	History   History                `json:"history,omitempty"`
	Links     map[string]string      `json:"_links,omitempty"`
	Ancestors []Page                 `json:"ancestors,omitempty"`
	Children  Children               `json:"children,omitempty"`
	Descendants []Page               `json:"descendants,omitempty"`
	Attachments []Attachment         `json:"attachments,omitempty"`
	Labels    []Label                `json:"labels,omitempty"`
	Comments  []Comment              `json:"comments,omitempty"`
}

// SpaceID returns the space ID from the space object
func (p *Page) SpaceID() int {
	return p.Space.ID
}

// CreatedAt returns the created time from history
func (p *Page) CreatedAt() time.Time {
	if !p.History.CreatedDate.IsZero() {
		return p.History.CreatedDate
	}
	return time.Time{}
}

// UpdatedAt returns the updated time from version.when (more reliable than history)
func (p *Page) UpdatedAt() time.Time {
	if !p.Version.UpdatedAt.IsZero() {
		return p.Version.UpdatedAt
	}
	if !p.History.LastUpdated.When.IsZero() {
		return p.History.LastUpdated.When
	}
	return time.Time{}
}

// History represents the history of a page
type History struct {
	CreatedDate   time.Time `json:"createdDate,omitempty"`
	LastUpdated   LastUpdated `json:"lastUpdated,omitempty"`
}

// LastUpdated represents the last updated information
type LastUpdated struct {
	When time.Time `json:"when,omitempty"`
}

// Children represents the children of a page
type Children struct {
	Page []Page `json:"page,omitempty"`
}

// Version represents the version of a page
type Version struct {
	Number    int       `json:"number,omitempty"`
	Message   string    `json:"message,omitempty"`
	MinorEdit bool      `json:"minorEdit,omitempty"`
	AuthorID  string    `json:"authorId,omitempty"`
	UpdatedAt time.Time `json:"when,omitempty"`
}

// Attachment represents a Confluence attachment
type Attachment struct {
	ID        string            `json:"id,omitempty"`
	Title     string            `json:"title,omitempty"`
	Filename  string            `json:"filename,omitempty"`
	FileSize  int64             `json:"fileSize,omitempty"`
	MediaType string            `json:"mediaType,omitempty"`
	CreatedAt time.Time         `json:"createdAt,omitempty"`
	UpdatedAt time.Time         `json:"updatedAt,omitempty"`
	Links     map[string]string `json:"_links,omitempty"`
}

// Label represents a Confluence label
type Label struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// Comment represents a Confluence comment
type Comment struct {
	ID        string                 `json:"id,omitempty"`
	Body      map[string]interface{} `json:"body,omitempty"`
	AuthorID  string                 `json:"authorId,omitempty"`
	CreatedAt time.Time              `json:"createdAt,omitempty"`
	UpdatedAt time.Time              `json:"updatedAt,omitempty"`
	ParentID  string                 `json:"parentId,omitempty"`
	PageID    string                 `json:"pageId,omitempty"`
}