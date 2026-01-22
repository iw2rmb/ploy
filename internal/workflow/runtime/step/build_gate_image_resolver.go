// build_gate_image_resolver.go implements the Build Gate image mapping resolver.
//
// The resolver selects runtime images for Build Gate containers when Stack Gate
// is enabled. It loads rules from multiple sources with precedence ordering:
//  1. Default file (/etc/ploy/gates/build-gate-images.yaml) - lowest precedence
//  2. Cluster/global inline config - medium precedence
//  3. Mod-level overrides - highest precedence
//
// Resolution uses "most specific match wins" semantics:
//   - Tool-specific rules (specificity 3) beat tool-agnostic rules (specificity 2)
//   - Same-specificity ties with different images are configuration errors
package step

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// DefaultMappingPath is the default location for Build Gate image mapping files.
const DefaultMappingPath = "/etc/ploy/gates/build-gate-images.yaml"

// BuildGateImageResolver resolves stack expectations to container images.
// Rules are merged from multiple sources with higher-precedence sources
// appearing later in the rules slice (so they are checked first).
type BuildGateImageResolver struct {
	// rules holds merged rules from all sources.
	// Higher precedence rules appear later and are checked last to allow
	// them to override lower precedence matches.
	rules []contracts.BuildGateImageRule
}

// NewBuildGateImageResolver creates a resolver by loading and merging rules
// from multiple sources.
//
// Parameters:
//   - defaultPath: Path to the default mapping file (empty to skip file loading)
//   - clusterInline: Cluster/global inline rules (may be nil)
//   - modOverride: Mod-level override rules (may be nil)
//   - stackGateEnabled: Whether Stack Gate is enabled (controls file requirement)
//
// Merge order (lowest to highest precedence):
//   - Default file rules
//   - Cluster inline rules
//   - Mod override rules
//
// Returns an error if:
//   - stackGateEnabled is true and defaultPath is set but file doesn't exist
//   - File exists but is invalid YAML or fails validation
//   - Any source has duplicate selectors
func NewBuildGateImageResolver(
	defaultPath string,
	clusterInline []contracts.BuildGateImageRule,
	modOverride []contracts.BuildGateImageRule,
	stackGateEnabled bool,
) (*BuildGateImageResolver, error) {
	var allRules []contracts.BuildGateImageRule

	// Determine if file is required: only when Stack Gate enabled AND no inline rules provided.
	fileRequired := stackGateEnabled && len(clusterInline) == 0 && len(modOverride) == 0

	// Load default file if path is provided.
	if defaultPath != "" {
		fileRules, err := loadImageMappingFile(defaultPath, fileRequired)
		if err != nil {
			return nil, err
		}
		if len(fileRules) > 0 {
			// Validate default file rules.
			mapping := contracts.BuildGateImageMapping{Images: fileRules}
			if err := mapping.Validate("default_file"); err != nil {
				return nil, fmt.Errorf("default mapping file: %w", err)
			}
			allRules = append(allRules, fileRules...)
		}
	}

	// Add cluster inline rules.
	if len(clusterInline) > 0 {
		mapping := contracts.BuildGateImageMapping{Images: clusterInline}
		if err := mapping.Validate("cluster_inline"); err != nil {
			return nil, fmt.Errorf("cluster inline: %w", err)
		}
		allRules = append(allRules, clusterInline...)
	}

	// Add mod override rules (highest precedence).
	if len(modOverride) > 0 {
		mapping := contracts.BuildGateImageMapping{Images: modOverride}
		if err := mapping.Validate("mod_override"); err != nil {
			return nil, fmt.Errorf("mod override: %w", err)
		}
		allRules = append(allRules, modOverride...)
	}

	return &BuildGateImageResolver{rules: allRules}, nil
}

// Resolve finds the best matching image for the given stack expectation.
//
// Resolution algorithm:
//  1. Find all rules that match the expectation
//  2. Select the highest specificity match
//  3. Among same-specificity matches, the last rule wins (higher precedence)
//
// The "last rule wins" semantics enables precedence-based override: rules from
// mod-level override cluster-level, which override default file rules. Conflicts
// between different sources are allowed and resolved by precedence order.
//
// Returns an error if no matching rule is found.
func (r *BuildGateImageResolver) Resolve(exp contracts.StackExpectation) (string, error) {
	if len(r.rules) == 0 {
		return "", fmt.Errorf("no image mapping rules available for stack %s:%s:%s",
			exp.Language, exp.Release, exp.Tool)
	}

	var matches []contracts.BuildGateImageRule
	for _, rule := range r.rules {
		if rule.Matches(exp) {
			matches = append(matches, rule)
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no image rule matches stack %s:%s:%s",
			exp.Language, exp.Release, exp.Tool)
	}

	// Find highest specificity.
	maxSpecificity := 0
	for _, m := range matches {
		if s := m.Specificity(); s > maxSpecificity {
			maxSpecificity = s
		}
	}

	// Among matches at highest specificity, the last one wins (highest precedence).
	// Rules are ordered from lowest to highest precedence, so iterate in reverse
	// to find the first (highest precedence) match at max specificity.
	for i := len(matches) - 1; i >= 0; i-- {
		if matches[i].Specificity() == maxSpecificity {
			return matches[i].Image, nil
		}
	}

	// This should not happen since we verified matches is non-empty and found maxSpecificity.
	return "", fmt.Errorf("internal error: no match found at specificity %d", maxSpecificity)
}

// loadImageMappingFile loads image rules from a YAML file.
// If the file doesn't exist and required is true, returns an error.
// If the file doesn't exist and required is false, returns nil.
func loadImageMappingFile(path string, required bool) ([]contracts.BuildGateImageRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if required {
				return nil, fmt.Errorf("image mapping file required but not found: %s", path)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("read image mapping file: %w", err)
	}

	// Parse YAML into intermediate structure.
	var raw struct {
		Images []map[string]any `yaml:"images"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse image mapping file: %w", err)
	}

	if len(raw.Images) == 0 {
		return nil, nil
	}

	// Convert to typed rules.
	rules := make([]contracts.BuildGateImageRule, 0, len(raw.Images))
	for i, item := range raw.Images {
		rule, err := parseImageRuleFromYAML(item, fmt.Sprintf("images[%d]", i))
		if err != nil {
			return nil, fmt.Errorf("image mapping file %s: %w", path, err)
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// parseImageRuleFromYAML parses a single image rule from a YAML map.
func parseImageRuleFromYAML(raw map[string]any, prefix string) (contracts.BuildGateImageRule, error) {
	var rule contracts.BuildGateImageRule

	// Parse stack.
	if v, ok := raw["stack"]; ok && v != nil {
		stackMap, ok := v.(map[string]any)
		if !ok {
			return rule, fmt.Errorf("%s.stack: expected object, got %T", prefix, v)
		}
		if lang, ok := stackMap["language"]; ok {
			if s, ok := lang.(string); ok {
				rule.Stack.Language = s
			}
		}
		if release, ok := stackMap["release"]; ok {
			switch r := release.(type) {
			case string:
				rule.Stack.Release = r
			case int:
				rule.Stack.Release = fmt.Sprintf("%d", r)
			case float64:
				if r == float64(int64(r)) {
					rule.Stack.Release = fmt.Sprintf("%d", int64(r))
				} else {
					rule.Stack.Release = fmt.Sprintf("%g", r)
				}
			}
		}
		if tool, ok := stackMap["tool"]; ok {
			if s, ok := tool.(string); ok {
				rule.Stack.Tool = s
			}
		}
	}

	// Parse image.
	if v, ok := raw["image"]; ok && v != nil {
		if s, ok := v.(string); ok {
			rule.Image = s
		} else {
			return rule, fmt.Errorf("%s.image: expected string, got %T", prefix, v)
		}
	}

	return rule, nil
}
