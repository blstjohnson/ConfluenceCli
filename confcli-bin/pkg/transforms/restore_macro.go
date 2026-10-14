package transforms

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// RestoreMacro restores paired Markdown comments after the body has been
// rendered to storage. Keeping the boundaries opaque during conversion lets
// nested includes, tables, code and rich formatting render normally.
type RestoreMacro struct {
	start, end *regexp.Regexp
	mapping    map[string]string
	autoParams bool
}

func (r *RestoreMacro) Name() string { return "restore/macro" }

func buildRestoreMacro(params map[string]interface{}) (Transform, error) {
	start, _ := params["start_pattern"].(string)
	end, _ := params["end_pattern"].(string)
	if start == "" {
		start = "<!-- {name}-start: {params} -->"
	}
	if end == "" {
		end = "<!-- {name}-end -->"
	}
	compile := func(s string, withParams bool) (*regexp.Regexp, error) {
		if strings.Count(s, "{name}") != 1 || (withParams && strings.Count(s, "{params}") != 1) {
			return nil, fmt.Errorf("restore_macro patterns require {name} and start_pattern requires {params}")
		}
		s = regexp.QuoteMeta(s)
		s = strings.ReplaceAll(s, `\{name\}`, `(?P<name>[A-Za-z][A-Za-z0-9_-]*)`)
		s = strings.ReplaceAll(s, `\{params\}`, `(?P<params>[^\r\n]*?)`)
		return regexp.Compile(s)
	}
	s, err := compile(start, true)
	if err != nil {
		return nil, err
	}
	e, err := compile(end, false)
	if err != nil {
		return nil, err
	}
	r := &RestoreMacro{start: s, end: e, mapping: map[string]string{"expand": "expand", "panel": "panel"}, autoParams: true}
	if v, ok := params["auto_params"].(bool); ok {
		r.autoParams = v
	}
	if raw, ok := params["macro_mapping"]; ok {
		m, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("restore_macro.macro_mapping must be a map")
		}
		r.mapping = map[string]string{}
		for name, cfg := range m {
			c, ok := cfg.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("restore_macro.macro_mapping.%s must be a map", name)
			}
			macro, _ := c["confluence_macro"].(string)
			if macro == "" {
				return nil, fmt.Errorf("restore_macro.macro_mapping.%s.confluence_macro is required", name)
			}
			r.mapping[name] = macro
		}
	}
	return r, nil
}

func (r *RestoreMacro) Apply(ctx *TransformContext) error {
	if ctx.PostContent != "" {
		for token, xml := range ctx.macroMarkers {
			ctx.PostContent = strings.ReplaceAll(ctx.PostContent, token, xml)
		}
		return nil
	}
	if ctx.PreContent == "" {
		return nil
	}
	if ctx.macroMarkers == nil {
		ctx.macroMarkers = map[string]string{}
	}
	prefix := fmt.Sprintf("confcli-restore-%x", sha256.Sum256([]byte(ctx.PreContent)))
	for strings.Contains(ctx.PreContent, prefix) {
		prefix += "x"
	}
	var stack []string
	var failure error
	combined := regexp.MustCompile("(?:" + r.start.String() + ")|(?:" + r.end.String() + ")")
	ctx.PreContent = MarkdownOutsideCode(ctx.PreContent, func(s string) string {
		return combined.ReplaceAllStringFunc(s, func(marker string) string {
			if failure != nil {
				return marker
			}
			match := r.start.FindStringSubmatch(marker)
			opening := match != nil
			re := r.start
			if !opening {
				re = r.end
				match = re.FindStringSubmatch(marker)
			}
			name := match[re.SubexpIndex("name")]
			macro, known := r.mapping[name]
			if !known {
				return marker
			}
			xml := ""
			if opening {
				stack = append(stack, name)
				payload := strings.TrimSpace(match[re.SubexpIndex("params")])
				values := map[string]string{}
				if r.autoParams && strings.HasPrefix(payload, "{") {
					if err := json.Unmarshal([]byte(payload), &values); err != nil {
						failure = fmt.Errorf("restore_macro %s: invalid JSON parameters: %w", name, err)
						return marker
					}
				} else if payload != "" {
					values["title"] = payload
				}
				xml = `<ac:structured-macro ac:name="` + xmlAttrEscape(macro) + `">`
				keys := make([]string, 0, len(values))
				for k := range values {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					xml += `<ac:parameter ac:name="` + xmlAttrEscape(k) + `">` + xmlAttrEscape(values[k]) + `</ac:parameter>`
				}
				xml += `<ac:rich-text-body>`
			} else {
				if len(stack) == 0 || stack[len(stack)-1] != name {
					failure = fmt.Errorf("restore_macro: unmatched %s end marker", name)
					return marker
				}
				stack = stack[:len(stack)-1]
				xml = `</ac:rich-text-body></ac:structured-macro>`
			}
			token := fmt.Sprintf("<!--%s-%d-->", prefix, len(ctx.macroMarkers))
			ctx.macroMarkers[token] = xml
			return token
		})
	})
	if failure != nil {
		return failure
	}
	if len(stack) != 0 {
		return fmt.Errorf("restore_macro: unclosed %s start marker", stack[len(stack)-1])
	}
	return nil
}
