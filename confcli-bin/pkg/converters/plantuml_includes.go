package converters

import (
	"html"
	"regexp"
	"strings"
)

// IncludeTarget identifies a page transcluded via the Confluence include
// or excerpt-include macro.
type IncludeTarget struct {
	Title    string
	SpaceKey string // empty when the macro omitted ri:space-key
}

// IncludeFetcher resolves an include target to the included page's storage
// content. defaultSpace is the space of the page that *contains* the include
// macro — fetchers should fall back to it when target.SpaceKey is empty.
// The returned storageSpaceKey is the space the storage actually came from
// and is used as defaultSpace for any nested include macros inside it.
type IncludeFetcher func(target IncludeTarget, defaultSpace string) (storage string, storageSpaceKey string, err error)

// DefaultIncludeMaxDepth caps recursion through include chains to keep
// pathological transclusion structures from blowing up.
const DefaultIncludeMaxDepth = 3

// plantumlOrIncludeRe matches plantuml, include, and excerpt-include macros
// in one pass so that the walker can preserve document order across mixed
// inline-and-transcluded diagrams.
var plantumlOrIncludeRe = regexp.MustCompile(
	`(?si)<ac:structured-macro\b[^>]*\bac:name\s*=\s*"(plantuml|include|excerpt-include)"[^>]*>(.*?)</ac:structured-macro>`)

var riPageRe = regexp.MustCompile(`(?si)<ri:page\b[^/>]*/?>`)
var riContentTitleRe = regexp.MustCompile(`(?si)\bri:content-title\s*=\s*"([^"]*)"`)
var riSpaceKeyRe = regexp.MustCompile(`(?si)\bri:space-key\s*=\s*"([^"]*)"`)
var plantumlBodyRe = regexp.MustCompile(
	`(?si)<ac:plain-text-body>\s*(?:<!\[CDATA\[)?(.*?)(?:\]\]>)?\s*</ac:plain-text-body>`)

// ExtractIncludeTargets returns every include / excerpt-include target in
// the given storage HTML, in document order.
func ExtractIncludeTargets(storage string) []IncludeTarget {
	var out []IncludeTarget
	for _, m := range plantumlOrIncludeRe.FindAllStringSubmatch(storage, -1) {
		if strings.EqualFold(m[1], "plantuml") {
			continue
		}
		if t, ok := parseIncludeTarget(m[2]); ok {
			out = append(out, t)
		}
	}
	return out
}

// ExtractPlantUMLBlocksWithIncludes walks the storage in document order and
// returns plantuml source blocks, recursing into include / excerpt-include
// macros via fetch. The first call should pass the containing page's space
// as defaultSpace; subsequent recursions use the space reported by fetch.
//
// visited prevents cycles, keyed by "<space>|<title>". maxDepth caps
// recursion. If fetch is nil or maxDepth <= 0, only top-level plantuml
// macros are returned (matching ExtractPlantUMLBlocks).
func ExtractPlantUMLBlocksWithIncludes(
	storage string,
	defaultSpace string,
	fetch IncludeFetcher,
	maxDepth int,
	visited map[string]bool,
) []string {
	if visited == nil {
		visited = map[string]bool{}
	}
	var out []string
	for _, m := range plantumlOrIncludeRe.FindAllStringSubmatch(storage, -1) {
		name, inner := strings.ToLower(m[1]), m[2]
		if name == "plantuml" {
			if body := extractPlantumlBody(inner); body != "" {
				out = append(out, body)
			}
			continue
		}
		if fetch == nil || maxDepth <= 0 {
			continue
		}
		t, ok := parseIncludeTarget(inner)
		if !ok {
			continue
		}
		key := visitKey(t, defaultSpace)
		if visited[key] {
			continue
		}
		visited[key] = true
		sub, subSpace, err := fetch(t, defaultSpace)
		if err != nil || sub == "" {
			continue
		}
		out = append(out, ExtractPlantUMLBlocksWithIncludes(sub, subSpace, fetch, maxDepth-1, visited)...)
	}
	return out
}

// CountPlantUMLImages reports how many PlantUML image references appear in
// the export_view HTML, used for diagnosing block/image count mismatches.
func CountPlantUMLImages(htmlContent string) int {
	return len(plantumlImgRe.FindAllString(htmlContent, -1))
}

func parseIncludeTarget(macroInner string) (IncludeTarget, bool) {
	pm := riPageRe.FindString(macroInner)
	if pm == "" {
		return IncludeTarget{}, false
	}
	tm := riContentTitleRe.FindStringSubmatch(pm)
	if tm == nil {
		return IncludeTarget{}, false
	}
	title := html.UnescapeString(tm[1])
	if title == "" {
		return IncludeTarget{}, false
	}
	t := IncludeTarget{Title: title}
	if sm := riSpaceKeyRe.FindStringSubmatch(pm); sm != nil {
		t.SpaceKey = sm[1]
	}
	return t, true
}

func extractPlantumlBody(macroInner string) string {
	m := plantumlBodyRe.FindStringSubmatch(macroInner)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func visitKey(t IncludeTarget, defaultSpace string) string {
	space := t.SpaceKey
	if space == "" {
		space = defaultSpace
	}
	return space + "|" + t.Title
}
