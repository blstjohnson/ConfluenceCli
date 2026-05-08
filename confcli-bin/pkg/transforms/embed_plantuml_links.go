package transforms

import (
	"fmt"
	"regexp"
	"strings"
)

// embedPlantUMLLinkRe matches a markdown link whose target ends in .puml or
// .plantuml (case-insensitive). It also captures an optional leading "!" (to
// detect existing image embeds) and optional surrounding "**" markers.
var embedPlantUMLLinkRe = regexp.MustCompile(
	`(!?)(\*\*)?\[([^\]]*)\]\(([^()\s]*\.(?i:puml|plantuml))\)(\*\*)?`,
)

// EmbedPlantUMLLinks rewrites markdown links targeting .puml/.plantuml files
// into image embeds so renderers that support PlantUML preview can display
// the diagram inline. Bold markers around the link are stripped, since a
// PlantUML link should not be bold. Links inside fenced code blocks are
// left alone.
type EmbedPlantUMLLinks struct{}

func NewEmbedPlantUMLLinks() *EmbedPlantUMLLinks {
	return &EmbedPlantUMLLinks{}
}

func (t *EmbedPlantUMLLinks) Name() string {
	return "embed/plantuml_links"
}

func (t *EmbedPlantUMLLinks) Apply(ctx *TransformContext) error {
	lines := strings.Split(ctx.PostContent, "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		lines[i] = embedPlantUMLLinkRe.ReplaceAllStringFunc(line, rewritePlantUMLLink)
	}
	ctx.PostContent = strings.Join(lines, "\n")
	return nil
}

func rewritePlantUMLLink(match string) string {
	sub := embedPlantUMLLinkRe.FindStringSubmatch(match)
	if len(sub) < 6 {
		return match
	}
	if sub[1] == "!" {
		// Already an image embed; leave alone.
		return match
	}
	leading := sub[2]
	text := sub[3]
	url := sub[4]
	trailing := sub[5]

	// Only strip the surrounding ** when they bracket *only* the link.
	// Otherwise the link is part of a wider bold run and removing one side
	// would unbalance it.
	stripOuter := leading != "" && trailing != ""

	// Strip ** inside the link text when the text is exactly **...**.
	if len(text) >= 4 && strings.HasPrefix(text, "**") && strings.HasSuffix(text, "**") {
		text = text[2 : len(text)-2]
	}

	rebuilt := fmt.Sprintf("![%s](%s)", text, url)
	if !stripOuter {
		rebuilt = leading + rebuilt + trailing
	}
	return rebuilt
}
