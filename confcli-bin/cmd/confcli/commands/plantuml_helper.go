package commands

import (
	"context"
	"fmt"
	"os"

	"confcli/pkg/api"
	"confcli/pkg/converters"
)

// newPlantUMLIncludeFetcher returns a converters.IncludeFetcher that resolves
// (space, title) to that page's storage content via the Confluence API.
// Results are cached for the lifetime of the closure so that exports
// transcluding the same template page pay the resolution cost once.
func newPlantUMLIncludeFetcher(ctx context.Context, apiClient api.Client) converters.IncludeFetcher {
	type cacheKey struct{ space, title string }
	cache := map[cacheKey]string{}
	spaceOf := map[cacheKey]string{}
	return func(t converters.IncludeTarget, defaultSpace string) (string, string, error) {
		space := t.SpaceKey
		if space == "" {
			space = defaultSpace
		}
		key := cacheKey{space, t.Title}
		if s, ok := cache[key]; ok {
			return s, spaceOf[key], nil
		}
		page, err := apiClient.GetPageByTitle(ctx, space, t.Title)
		if err != nil || page == nil {
			cache[key] = ""
			spaceOf[key] = space
			return "", space, err
		}
		pageID, ok := page.ID.Int()
		if !ok {
			cache[key] = ""
			spaceOf[key] = space
			return "", space, fmt.Errorf("invalid page id for include target %q", t.Title)
		}
		storage, err := apiClient.GetPageContent(ctx, pageID, "storage", 0)
		if err != nil {
			cache[key] = ""
			spaceOf[key] = space
			return "", space, err
		}
		resolvedSpace := space
		if page.Space.Key != "" {
			resolvedSpace = page.Space.Key
		}
		cache[key] = storage
		spaceOf[key] = resolvedSpace
		return storage, resolvedSpace, nil
	}
}

// renderExportWithPlantUML converts export_view HTML to markdown, replacing
// rendered PlantUML images with their source code blocks pulled from the
// page's storage format (and recursively from any transcluded pages).
//
// storageContent may be empty — when needed and not already supplied it is
// fetched on demand. defaultSpaceKey seeds include-macro resolution for
// targets that omit ri:space-key. A non-zero block/image count mismatch
// is logged to stderr but does not fail the conversion; the markdown is
// still returned with whatever blocks were found.
func renderExportWithPlantUML(
	ctx context.Context,
	apiClient api.Client,
	pageID int,
	exportView string,
	baseURL string,
	storageContent string,
	storageVersion int,
	defaultSpaceKey string,
	fetcher converters.IncludeFetcher,
) (string, error) {
	if !converters.HasPlantUMLImages(exportView) {
		return converters.ExportViewToMarkdown(exportView, baseURL)
	}

	if storageContent == "" {
		sc, err := apiClient.GetPageContent(ctx, pageID, "storage", storageVersion)
		if err == nil {
			storageContent = sc
		}
	}

	blocks := converters.ExtractPlantUMLBlocksWithIncludes(
		storageContent, defaultSpaceKey, fetcher,
		converters.DefaultIncludeMaxDepth, map[string]bool{},
	)

	imgCount := converters.CountPlantUMLImages(exportView)
	if len(blocks) != imgCount {
		fmt.Fprintf(os.Stderr,
			"Warning: page %d has %d PlantUML images but resolved %d source blocks (some diagrams may render as broken images)\n",
			pageID, imgCount, len(blocks))
	}

	if len(blocks) == 0 {
		return converters.ExportViewToMarkdown(exportView, baseURL)
	}

	md, err := converters.ExportViewToMarkdownKeepImages(exportView, baseURL)
	if err != nil {
		return "", err
	}
	md = converters.ReplacePlantUMLImages(md, blocks)
	return converters.StripJunkImages(md), nil
}
