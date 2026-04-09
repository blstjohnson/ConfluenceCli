package transforms

import "fmt"

// BuildFunc creates a Transform from a TransformSpec's params map.
type BuildFunc func(params map[string]interface{}) (Transform, error)

// Registry maps transform type names to their builder functions.
type Registry struct {
	builders map[string]BuildFunc
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{builders: make(map[string]BuildFunc)}
}

// Register adds a builder for the given transform type name.
func (r *Registry) Register(typeName string, fn BuildFunc) {
	r.builders[typeName] = fn
}

// Build instantiates a Transform from a TransformSpec.
func (r *Registry) Build(spec TransformSpec) (Transform, error) {
	fn, ok := r.builders[spec.Type]
	if !ok {
		return nil, fmt.Errorf("unknown transform type %q", spec.Type)
	}
	return fn(spec.Params)
}

// DefaultRegistry returns a Registry pre-populated with all built-in transforms.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	r.Register("remove_macro", buildRemoveMacro)
	r.Register("remove_element", buildRemoveElement)
	r.Register("modify_links", buildModifyLinks)
	r.Register("modify_content", buildModifyContent)
	r.Register("rewrite_tfs_links", buildRewriteTFSLinks)
	r.Register("rewrite_internal_links", buildRewriteInternalLinks)
	r.Register("expand_tiny_urls", buildExpandTinyURLs)

	return r
}

// --- Built-in builders ---

func buildRemoveMacro(params map[string]interface{}) (Transform, error) {
	names, err := getStringSlice(params, "macro_names")
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("remove_macro requires at least one macro_names entry")
	}
	
	// Check if preserve_content is set (useful for expand macros)
	preserveContent := false
	if pc, ok := params["preserve_content"]; ok {
		if pb, ok := pc.(bool); ok {
			preserveContent = pb
		}
	}
	
	if preserveContent {
		return NewRemoveMacroWithContentPreserve(names...)
	}
	return NewRemoveMacro(names...)
}

func buildRemoveElement(params map[string]interface{}) (Transform, error) {
	selectors, err := getStringSlice(params, "selectors")
	if err != nil {
		return nil, err
	}
	if len(selectors) == 0 {
		return nil, fmt.Errorf("remove_element requires at least one selectors entry")
	}
	return NewRemoveElement(selectors...)
}

func buildModifyLinks(params map[string]interface{}) (Transform, error) {
	rawRules, ok := params["rules"]
	if !ok {
		return nil, fmt.Errorf("modify_links requires 'rules' param")
	}
	rules, err := toLinkRules(rawRules)
	if err != nil {
		return nil, err
	}
	return NewModifyLinks(rules...)
}

func buildModifyContent(params map[string]interface{}) (Transform, error) {
	rawRules, ok := params["rules"]
	if !ok {
		return nil, fmt.Errorf("modify_content requires 'rules' param")
	}
	rules, err := toContentRules(rawRules)
	if err != nil {
		return nil, err
	}
	phase := PhasePre
	if p, ok := params["phase"]; ok {
		if ps, ok := p.(string); ok && ps == "post" {
			phase = PhasePost
		}
	}
	return NewModifyContent(phase, rules...)
}

func buildRewriteTFSLinks(params map[string]interface{}) (Transform, error) {
	tfsBase, _ := params["tfs_base_url"].(string)
	localPath, _ := params["local_repo_path"].(string)
	currentFile, _ := params["current_file_path"].(string)
	if tfsBase == "" {
		return nil, fmt.Errorf("rewrite_tfs_links requires 'tfs_base_url' param")
	}
	return NewRewriteTFSLinks(tfsBase, localPath, currentFile), nil
}

func buildRewriteInternalLinks(params map[string]interface{}) (Transform, error) {
	confBase, _ := params["conf_base_url"].(string)
	currentDir, _ := params["current_page_dir"].(string)
	if confBase == "" {
		return nil, fmt.Errorf("rewrite_internal_links requires 'conf_base_url' param")
	}
	// page_map is typically injected at runtime, not from YAML
	return NewRewriteInternalLinks(nil, confBase, currentDir), nil
}

func buildExpandTinyURLs(params map[string]interface{}) (Transform, error) {
	confBase, _ := params["conf_base_url"].(string)
	if confBase == "" {
		return nil, fmt.Errorf("expand_tiny_urls requires 'conf_base_url' param")
	}
	// Default to client-side decoding resolver when used from YAML profiles.
	// The hierarchy export injects a smarter resolver at runtime.
	return NewExpandTinyURLs(confBase, DecodingResolver()), nil
}

// --- param helpers ---

func getStringSlice(params map[string]interface{}, key string) ([]string, error) {
	v, ok := params[key]
	if !ok {
		return nil, nil
	}
	switch val := v.(type) {
	case []interface{}:
		result := make([]string, len(val))
		for i, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d]: expected string, got %T", key, i, item)
			}
			result[i] = s
		}
		return result, nil
	case []string:
		return val, nil
	default:
		return nil, fmt.Errorf("%s: expected string array, got %T", key, v)
	}
}

func toLinkRules(raw interface{}) ([]LinkRule, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("rules: expected array, got %T", raw)
	}
	rules := make([]LinkRule, len(items))
	for i, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("rules[%d]: expected map, got %T", i, item)
		}
		find, _ := m["find"].(string)
		replace, _ := m["replace"].(string)
		if find == "" {
			return nil, fmt.Errorf("rules[%d]: 'find' is required", i)
		}
		rules[i] = LinkRule{Find: find, Replace: replace}
	}
	return rules, nil
}

func toContentRules(raw interface{}) ([]ContentRule, error) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("rules: expected array, got %T", raw)
	}
	rules := make([]ContentRule, len(items))
	for i, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("rules[%d]: expected map, got %T", i, item)
		}
		find, _ := m["find"].(string)
		replace, _ := m["replace"].(string)
		if find == "" {
			return nil, fmt.Errorf("rules[%d]: 'find' is required", i)
		}
		rules[i] = ContentRule{Find: find, Replace: replace}
	}
	return rules, nil
}
