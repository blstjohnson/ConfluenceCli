package transforms

import (
	"fmt"
	"regexp"
)

// ModifyContent applies regex find/replace rules on raw page content.
// It operates on the specified phase (PreContent or PostContent).
type ModifyContent struct {
	// Rules is a list of find/replace pairs applied in order.
	Rules []ContentRule
	// TargetPhase determines which content field to modify.
	TargetPhase Phase

	compiled []compiledContentRule
}

// ContentRule is a single find/replace rule for content.
type ContentRule struct {
	// Find is a regex pattern to match in the content.
	Find string
	// Replace is the replacement string (supports $1, $2, etc.).
	Replace string
}

type compiledContentRule struct {
	re      *regexp.Regexp
	replace string
}

// NewModifyContent creates a ModifyContent transform with the given rules and target phase.
func NewModifyContent(phase Phase, rules ...ContentRule) (*ModifyContent, error) {
	compiled := make([]compiledContentRule, len(rules))
	for i, rule := range rules {
		re, err := regexp.Compile(rule.Find)
		if err != nil {
			return nil, fmt.Errorf("invalid content rule pattern %q: %w", rule.Find, err)
		}
		compiled[i] = compiledContentRule{re: re, replace: rule.Replace}
	}
	return &ModifyContent{Rules: rules, TargetPhase: phase, compiled: compiled}, nil
}

func (m *ModifyContent) Name() string {
	return "modify/content"
}

func (m *ModifyContent) Apply(ctx *TransformContext) error {
	var content string
	switch m.TargetPhase {
	case PhasePre:
		content = ctx.PreContent
	case PhasePost:
		content = ctx.PostContent
	}

	for _, rule := range m.compiled {
		content = rule.re.ReplaceAllString(content, rule.replace)
	}

	switch m.TargetPhase {
	case PhasePre:
		ctx.PreContent = content
	case PhasePost:
		ctx.PostContent = content
	}

	return nil
}
