package models

// ScrollVersionsConfig represents the Scroll Versions configuration for a space.
// Returned by GET /rest/scroll-versions/1.0/config/{spaceKey}.
type ScrollVersionsConfig struct {
	EnableVersionManagement bool   `json:"enableVersionManagement"`
	EnableTranslation       bool   `json:"enableTranslation"`
	EnableVariants          bool   `json:"enableVariants"`
	EnablePermalinks        bool   `json:"enablePermalinks"`
	EnableSeo               bool   `json:"enableSeo"`
	IsTargetSpace           bool   `json:"isTargetSpace"`
	RestrictEditInReaderView bool  `json:"restrictEditInReaderView"`
	WorkflowType            string `json:"workflowType"`
	EnableSearch            bool   `json:"enableSearch"`
}

// ScrollVersion represents a version in a Scroll Versions-managed space.
// Returned by GET /rest/scroll-versions/1.0/versions/{spaceKey}.
type ScrollVersion struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	ReleaseDateMillis  int64  `json:"releaseDateMillis"`
	ReleaseDate        string `json:"releaseDate"`
	PrecedingVersionID string `json:"precedingVersionId"`
	Archived           bool   `json:"archived"`
	Color              string `json:"color"`
	RuntimeAccessible  bool   `json:"runtimeAccessible"`
}

// ScrollPageTreeNode represents a node in the Scroll Versions page tree.
// Returned by GET /rest/scroll-versions/1.0/pagetree/{spaceKey}.
type ScrollPageTreeNode struct {
	Type            string               `json:"type"`
	Title           string               `json:"title"`
	ChangePageID    int64                `json:"changePageId"`
	ID              int64                `json:"id"`
	MasterPageID    int64                `json:"masterPageId"`
	URLPath         string               `json:"urlPath"`
	VersionName     string               `json:"versionName"`
	ScrollPageID    string               `json:"scrollPageId"`
	ScrollPageKey   string               `json:"scrollPageKey"`
	ScrollPageTitle string               `json:"scrollPageTitle"`
	IsManaged       bool                 `json:"isManaged"`
	IsMasterPage    bool                 `json:"isMasterPage"`
	HasChildren     bool                 `json:"hasChildren"`
	IsCurrentPage   bool                 `json:"isCurrentPage"`
	IsDeleted       bool                 `json:"isDeleted"`
	IsEnabled       bool                 `json:"isEnabled"`
	Children        []ScrollPageTreeNode `json:"children"`
}

// ScrollPage represents a resolved Scroll page for a version.
// Returned by GET /rest/scroll-versions/1.0/page/{spaceKey}/{scrollPageId}/{versionId}.
type ScrollPage struct {
	ScrollPageTitle string          `json:"scrollPageTitle"`
	ScrollPageID    string          `json:"scrollPageId"`
	ScrollPageKey   string          `json:"scrollPageKey"`
	SpaceKey        string          `json:"spaceKey"`
	PageType        string          `json:"pageType"`
	Unversioned     bool            `json:"unversioned"`
	MasterPage      bool            `json:"masterPage"`
	ChangeType      string          `json:"changeType"`
	Available       bool            `json:"available"`
	Fallback        bool            `json:"fallback"`
	Unresolved      bool            `json:"unresolved"`
	ConfluencePage  *ConfluencePage `json:"confluencePage"`
}

// ConfluencePage is the Confluence page reference embedded in a ScrollPage.
type ConfluencePage struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}
