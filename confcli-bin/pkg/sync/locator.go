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
	client      api.Client
	logger      *log.Logger
	rootID      int
	rootPages   map[string]*models.Page
	pinnedPages map[int]*models.Page
	pinnedSpace string
}

type RootSnapshotClient interface {
	GetRootSnapshot(ctx context.Context, spaceKey string, rootID int) ([]models.Page, error)
}

// NewLocator returns a Locator backed by the given client. If logger is nil,
// warnings are silenced (use log.New(os.Stderr, ...) at the call site to
// surface them).
func NewLocator(client api.Client, logger *log.Logger, rootID ...int) *Locator {
	l := &Locator{client: client, logger: logger}
	if len(rootID) > 0 {
		l.rootID = rootID[0]
	}
	return l
}

// FindByPath returns the page identified by relPath within spaceKey, or nil
// if no such page exists. When the server returns multiple matches (an
// inconsistent state — manual reconciliation needed) a warning is logged and
// the first match is returned.
func (l *Locator) FindByPath(ctx context.Context, spaceKey, relPath string) (*models.Page, error) {
	if _, ok := l.client.(RootSnapshotClient); ok && l.rootID != 0 {
		return l.findInRoot(ctx, spaceKey, relPath)
	}
	cql := identity.CQLFilter(spaceKey, relPath)

	// limit=2 is enough to detect the >1 case while keeping the response small.
	results, err := l.client.Search(ctx, cql, 2)
	if err != nil {
		return nil, fmt.Errorf("locator: CQL search failed: %w", err)
	}
	if len(results) == 0 {
		if l.rootID != 0 {
			return l.findInRoot(ctx, spaceKey, relPath)
		}
		return nil, nil
	}
	if len(results) > 1 && l.logger != nil {
		l.logger.Printf("locator: multiple pages share confcli-id for %q in space %q; using first match — manual reconciliation needed", relPath, spaceKey)
	}

	id := results[0].Content.ID.IntOrString()
	if id == nil || id == "" {
		return nil, fmt.Errorf("locator: search result for %q has empty page ID", relPath)
	}

	expansions := []string{"version"}
	if l.rootID != 0 {
		expansions = append(expansions, "space", "ancestors")
	}
	page, err := l.client.GetPageWithExpansions(ctx, id, expansions)
	if err != nil {
		return nil, fmt.Errorf("locator: fetch page %v: %w", id, err)
	}
	if l.rootID != 0 {
		if err := l.validateDestination(page, spaceKey); err != nil {
			return nil, err
		}
	}
	// The metadata.labels expansion returns labels under metadata.labels.results,
	// which doesn't match the top-level "labels" JSON tag on models.Page — the
	// field stays empty. Use the dedicated label endpoint instead, which the API
	// client correctly deserializes.
	if pageIDInt, ok := page.ID.Int(); ok {
		labels, err := l.client.GetLabels(ctx, pageIDInt)
		if err != nil {
			return nil, fmt.Errorf("locator: fetch labels for page %d: %w", pageIDInt, err)
		}
		page.Labels = labels
	}
	return page, nil
}

// FindByID adopts an explicitly selected copy without relying on its labels.
// Validate the space and ancestor chain before exposing it to the executor.
func (l *Locator) FindByID(ctx context.Context, spaceKey string, pageID int) (*models.Page, error) {
	if l.rootID == 0 {
		return nil, fmt.Errorf("locator: page_id overrides require a selected root")
	}
	if bulk, ok := l.client.(RootSnapshotClient); ok {
		if l.pinnedPages == nil || l.pinnedSpace != spaceKey {
			pages, err := bulk.GetRootSnapshot(ctx, spaceKey, l.rootID)
			if err != nil {
				return nil, fmt.Errorf("locator: pinned root snapshot: %w", err)
			}
			cache := make(map[int]*models.Page, len(pages))
			for _, candidate := range pages {
				if err := l.validateDestination(&candidate, spaceKey); err != nil {
					return nil, err
				}
				id, valid := candidate.ID.Int()
				if !valid || cache[id] != nil {
					return nil, fmt.Errorf("locator: invalid or duplicate pinned page ID %v", candidate.ID)
				}
				p := candidate
				cache[id] = &p
			}
			l.pinnedPages = cache
			l.pinnedSpace = spaceKey
		}
		if page := l.pinnedPages[pageID]; page != nil {
			return page, nil
		}
	}
	page, err := l.client.GetPageWithExpansions(ctx, pageID, []string{"space", "ancestors", "version"})
	if err != nil {
		return nil, fmt.Errorf("locator: pinned page %d: %w", pageID, err)
	}
	if err := l.validateDestination(page, spaceKey); err != nil {
		return nil, err
	}
	labels, err := l.client.GetLabels(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("locator: pinned page %d labels: %w", pageID, err)
	}
	page.Labels = labels
	return page, nil
}

func (l *Locator) validateDestination(page *models.Page, spaceKey string) error {
	if page == nil || page.Space.Key != spaceKey {
		return fmt.Errorf("locator: destination page does not belong to space %q", spaceKey)
	}
	for _, ancestor := range page.Ancestors {
		if id, ok := ancestor.ID.Int(); ok && id == l.rootID {
			return nil
		}
	}
	return fmt.Errorf("locator: page %v is outside selected root %d", page.ID, l.rootID)
}

// CQL indexing is asynchronous. A fresh page's labels are already available
// through the content/label endpoints even when label search returns nothing.
// Scan the selected root once per plan and verify its space before using it.
func (l *Locator) findInRoot(ctx context.Context, spaceKey, relPath string) (*models.Page, error) {
	if l.rootPages == nil {
		root, err := l.client.GetPageWithExpansions(ctx, l.rootID, []string{"space"})
		if err != nil {
			return nil, fmt.Errorf("locator: fetch root: %w", err)
		}
		if root == nil || root.Space.Key != spaceKey {
			return nil, fmt.Errorf("locator: root %d does not belong to space %q", l.rootID, spaceKey)
		}
		var pages []models.Page
		bulk, hasSnapshot := l.client.(RootSnapshotClient)
		if hasSnapshot {
			pages, err = bulk.GetRootSnapshot(ctx, spaceKey, l.rootID)
		} else {
			pages, err = l.client.GetDescendants(ctx, l.rootID, 0)
		}
		if err != nil {
			return nil, fmt.Errorf("locator: list root descendants: %w", err)
		}
		cache := map[string]*models.Page{}
		for _, candidate := range pages {
			id, ok := candidate.ID.Int()
			if !ok {
				return nil, fmt.Errorf("locator: descendant has invalid page ID")
			}
			labels := candidate.Labels
			if !hasSnapshot {
				labels, err = l.client.GetLabels(ctx, id)
			}
			if err != nil {
				return nil, fmt.Errorf("locator: labels for descendant %d: %w", id, err)
			}
			label := identity.ExtractIDLabel(labels)
			if label == "" {
				continue
			}
			if cache[label] != nil {
				return nil, fmt.Errorf("locator: multiple pages share identity label %q under root %d", label, l.rootID)
			}
			p := &candidate
			if !hasSnapshot {
				p, err = l.client.GetPageWithExpansions(ctx, id, []string{"space", "version"})
			}
			if err != nil {
				return nil, fmt.Errorf("locator: fetch descendant %d: %w", id, err)
			}
			if p == nil || p.Space.Key != spaceKey {
				return nil, fmt.Errorf("locator: descendant %d does not belong to space %q", id, spaceKey)
			}
			p.Labels = labels
			cache[label] = p
		}
		l.rootPages = cache
	}
	page := l.rootPages[identity.BuildIDLabel(relPath)]
	if page != nil && l.logger != nil {
		l.logger.Printf("locator: found %q directly under root %d", relPath, l.rootID)
	}
	return page, nil
}
