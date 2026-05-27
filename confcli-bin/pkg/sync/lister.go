package sync

import (
	"context"
	"fmt"
	"log"
	"strings"

	"confcli/pkg/api"
	"confcli/pkg/sync/identity"
)

// APILister is the production ManagedPageLister, backed by api.Client.
//
// Implementation: walk the entire descendant tree under rootPageID, then
// expand each page's labels and keep only those carrying a confcli-id-
// label. This is N+1 in the size of the subtree — acceptable for the
// first cut of the sync command. A future optimization is a CQL query
// scoped to the confcli-managed anchor label (which would require the
// executor to set such a label on every synced page).
type APILister struct {
	client api.Client
	logger *log.Logger
}

// NewAPILister constructs an APILister. If logger is nil, warnings about
// per-page label fetch failures are silently dropped.
func NewAPILister(client api.Client, logger *log.Logger) *APILister {
	return &APILister{client: client, logger: logger}
}

// ListManaged returns every page under rootPageID in spaceKey that carries
// a confcli-id- label. Pages whose label fetch fails are skipped with a
// warning rather than failing the whole call — a transient fetch error
// shouldn't turn the entire subtree into a sea of "orphans".
func (l *APILister) ListManaged(ctx context.Context, spaceKey string, rootPageID int) ([]ManagedPage, error) {
	if rootPageID == 0 {
		return nil, fmt.Errorf("APILister.ListManaged: rootPageID is required")
	}

	descendants, err := l.client.GetDescendants(ctx, rootPageID, 0)
	if err != nil {
		return nil, fmt.Errorf("get descendants of %d: %w", rootPageID, err)
	}

	var managed []ManagedPage
	for _, page := range descendants {
		pageID, ok := page.ID.Int()
		if !ok {
			continue
		}

		// metadata.labels expansion doesn't deserialize into Page.Labels
		// (response nests under metadata.labels.results; the JSON tag is
		// just "labels"). Always use the dedicated label endpoint, which
		// the API client correctly maps.
		labels := page.Labels
		if len(labels) == 0 {
			fetched, err := l.client.GetLabels(ctx, pageID)
			if err != nil {
				if l.logger != nil {
					l.logger.Printf("lister: fetch labels for page %d (%q): %v — skipping", pageID, page.Title, err)
				}
				continue
			}
			labels = fetched
		}

		idLabel := identity.ExtractIDLabel(labels)
		if idLabel == "" || !strings.HasPrefix(idLabel, identity.IDLabelPrefix) {
			continue
		}

		managed = append(managed, ManagedPage{
			PageID:  pageID,
			Title:   page.Title,
			IDLabel: idLabel,
		})
	}

	return managed, nil
}
