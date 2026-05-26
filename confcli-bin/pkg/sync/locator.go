// Package sync owns the engine and supporting types for `confcli sync`. It
// reuses pkg/api, pkg/converters and pkg/transforms; this file contains the
// identity-anchored page locator used to resolve "does this markdown file
// already exist on the server?" via the confcli-id label.
package sync

import (
	"context"
	"fmt"
	"log"

	"confcli/pkg/api"
	"confcli/pkg/models"
	"confcli/pkg/sync/identity"
)

// Locator resolves a sync-root-relative markdown path to a Confluence page by
// querying the confcli-id label within a space. It returns the page with
// labels and version expanded so callers can immediately read the
// confcli-hash and decide between skip/update.
type Locator struct {
	client api.Client
	logger *log.Logger
}

// NewLocator returns a Locator backed by the given client. If logger is nil,
// warnings are silenced (use log.New(os.Stderr, ...) at the call site to
// surface them).
func NewLocator(client api.Client, logger *log.Logger) *Locator {
	return &Locator{client: client, logger: logger}
}

// FindByPath returns the page identified by relPath within spaceKey, or nil
// if no such page exists. When the server returns multiple matches (an
// inconsistent state — manual reconciliation needed) a warning is logged and
// the first match is returned.
func (l *Locator) FindByPath(ctx context.Context, spaceKey, relPath string) (*models.Page, error) {
	cql := identity.CQLFilter(spaceKey, relPath)

	// limit=2 is enough to detect the >1 case while keeping the response small.
	results, err := l.client.Search(ctx, cql, 2)
	if err != nil {
		return nil, fmt.Errorf("locator: CQL search failed: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	if len(results) > 1 && l.logger != nil {
		l.logger.Printf("locator: multiple pages share confcli-id for %q in space %q; using first match — manual reconciliation needed", relPath, spaceKey)
	}

	id := results[0].Content.ID.IntOrString()
	if id == nil || id == "" {
		return nil, fmt.Errorf("locator: search result for %q has empty page ID", relPath)
	}

	page, err := l.client.GetPageWithExpansions(ctx, id, []string{"metadata.labels", "version"})
	if err != nil {
		return nil, fmt.Errorf("locator: fetch page %v with labels: %w", id, err)
	}
	return page, nil
}
