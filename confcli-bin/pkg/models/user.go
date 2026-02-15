package models

// User represents a Confluence user
type User struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Email       string `json:"emailAddress"` // Confluence Cloud uses this field
	AccountID   string `json:"accountId,omitempty"` // Confluence Cloud uses this field
}