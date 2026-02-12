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
	if v, ok := p.Value.(int); ok {
		return v, true
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
	ID          PageID                 `json:"id,omitempty"`
	Title       string                 `json:"title,omitempty"`
	SpaceID     int                    `json:"spaceId,omitempty"`
	Status      string                 `json:"status,omitempty"`
	CreatedAt   time.Time              `json:"createdAt,omitempty"`
	UpdatedAt   time.Time              `json:"updatedAt,omitempty"`
	Version     Version                `json:"version,omitempty"`
	AuthorID    string                 `json:"authorId,omitempty"`
	Body        map[string]interface{} `json:"body,omitempty"`
	Links       map[string]string      `json:"_links,omitempty"`
	Ancestors   []Page                 `json:"ancestors,omitempty"`
	Descendants []Page                 `json:"descendants,omitempty"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	Labels      []Label                `json:"labels,omitempty"`
	Comments    []Comment              `json:"comments,omitempty"`
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