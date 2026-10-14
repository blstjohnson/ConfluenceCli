package commands

import (
	"context"
	"errors"
	"fmt"

	"confcli/pkg/api"
	"confcli/pkg/converters"
	"confcli/pkg/models"
)

func newRoundTripPageFetcher(ctx context.Context, client api.Client) func(int) (converters.RoundTripPage, error) {
	cache := map[int]converters.RoundTripPage{}
	return func(id int) (converters.RoundTripPage, error) {
		if page, ok := cache[id]; ok {
			return page, nil
		}
		p, err := client.GetPage(ctx, id)
		if err != nil {
			return converters.RoundTripPage{}, err
		}
		storage, err := client.GetPageContent(ctx, id, "storage", p.Version.Number)
		if err != nil {
			return converters.RoundTripPage{}, err
		}
		view, err := client.GetPageContent(ctx, id, "export_view", p.Version.Number)
		if err != nil {
			return converters.RoundTripPage{}, err
		}
		page := converters.RoundTripPage{PageID: id, SpaceKey: p.Space.Key, Title: p.Title, Storage: storage, ExportView: view}
		cache[id] = page
		return page, nil
	}
}

// Prefer pages already collected for this export, then cache API resolutions
// for references outside the exported subtree (including cross-space includes).
func newRoundTripPageResolver(ctx context.Context, client api.Client, pages map[int]*models.Page, defaultSpace string) func(string, string) (int, error) {
	type key struct{ space, title string }
	cache := map[key]int{}
	for id, p := range pages {
		space := p.Space.Key
		if space == "" {
			space = defaultSpace
		}
		cache[key{space, p.Title}] = id
	}
	return func(space, title string) (int, error) {
		if space == "" {
			space = defaultSpace
		}
		k := key{space, title}
		if id, ok := cache[k]; ok {
			return id, nil
		}
		p, err := client.GetPageByTitle(ctx, space, title)
		if err != nil {
			var missing *api.PageNotFoundError
			if errors.As(err, &missing) {
				cache[k] = 0
				return 0, nil
			}
			return 0, fmt.Errorf("resolve page %q in %q: %w", title, space, err)
		}
		if p == nil {
			return 0, fmt.Errorf("page %q in %q not found", title, space)
		}
		id, ok := p.ID.Int()
		if !ok {
			return 0, fmt.Errorf("invalid ID for page %q in %q", title, space)
		}
		cache[k] = id
		return id, nil
	}
}
