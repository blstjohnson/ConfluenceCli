package transforms

import (
	"fmt"
	"regexp"
)

// ModifyLinks applies regex find/replace on link URLs in PostContent (markdown).
// It matches markdown links [text](url) and applies the pattern to the URL portion.
type ModifyLinks struct {
	// Rules is a list of find/replace pairs applied in order.
	Rules []LinkRule

	compiled []compiledLinkRule
}

// LinkRule is a single find/replace rule for link URLs.
type LinkRule struct {
	// Find is a regex pattern to match against the link URL.
	Find string
	// Replace is the replacement string (supports $1, $2, etc. for capture groups).
	Replace string
}

type compiledLinkRule struct {
	re      *regexp.Regexp
	replace string
}

// markdownLinkPattern matches markdown links: [text](url)
var markdownLinkPattern = regexp.MustCompile(`\[([^\]]*)\]\(([^()\s]*(?:\([^()]*\)[^()\s]*)*)\)`)

// NewModifyLinks creates a ModifyLinks transform with the given rules.
func NewModifyLinks(rules ...LinkRule) (*ModifyLinks, error) {
	compiled := make([]compiledLinkRule, len(rules))
	for i, rule := range rules {
		re, err := regexp.Compile(rule.Find)
		if err != nil {
			return nil, fmt.Errorf("invalid link rule pattern %q: %w", rule.Find, err)
		}
		compiled[i] = compiledLinkRule{re: re, replace: rule.Replace}
	}
	return &ModifyLinks{Rules: rules, compiled: compiled}, nil
}

func (m *ModifyLinks) Name() string {
	return "modify/links"
}

func (m *ModifyLinks) Apply(ctx *TransformContext) error {
	ctx.PostContent = markdownLinkPattern.ReplaceAllStringFunc(ctx.PostContent, func(match string) string {
		sub := markdownLinkPattern.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		text := sub[1]
		linkURL := sub[2]

		for _, rule := range m.compiled {
			linkURL = rule.re.ReplaceAllString(linkURL, rule.replace)
		}

		return fmt.Sprintf("[%s](%s)", text, linkURL)
	})
	return nil
}
