// build_gate_image_rule.go defines Build Gate image mapping types for stack-based image selection.
//
// These types configure how Build Gate resolves runtime images when Stack Gate
// is enabled. Image rules map stack expectations (language, release, tool) to
// container images with specificity-based resolution.
//
// ## Resolution Algorithm
//
// The resolver selects images based on specificity:
//   - Specificity 4: language + tool + release (most specific, highest priority)
//   - Specificity 3: language + tool
//   - Specificity 2: language + release
//   - Specificity 1: language-only (broad fallback)
//
// When multiple rules match, the highest specificity wins. Ties at the same
// specificity level with different images are configuration errors.
//
// ## Related Files
//
//   - build_gate_image_rule_parse.go: Parsing from map[string]any
//   - build_gate_image_rule_wire.go: Wire serialization to map[string]any
//   - build_gate_image_resolver.go: Runtime resolution implementation
package contracts

import (
	"fmt"
	"strings"
)

// BuildGateImageRule maps a stack expectation to a Build Gate runtime image.
// Each rule matches requests where the expectation fields match (or are wildcards).
type BuildGateImageRule struct {
	// Stack holds the stack expectation to match against.
	// Language is required. Release and Tool are optional wildcards.
	Stack StackExpectation `json:"stack,omitempty" yaml:"stack,omitempty"`

	// Image is the container image URL to use when this rule matches.
	// Required field.
	Image string `json:"image,omitempty" yaml:"image,omitempty"`
}

// Specificity returns the matching priority of this rule.
// Higher values indicate more specific matches:
//   - 4: language + tool + release
//   - 3: language + tool
//   - 2: language + release
//   - 1: language-only
func (r BuildGateImageRule) Specificity() int {
	if r.Stack.Tool != "" && r.Stack.Release != "" {
		return 4
	}
	if r.Stack.Tool != "" {
		return 3
	}
	if r.Stack.Release != "" {
		return 2
	}
	return 1
}

// Matches returns true if this rule matches the given expectation.
// A rule matches when:
//   - Language matches exactly (both required)
//   - Release matches exactly when rule.Release is set
//   - Tool matches exactly when rule.Tool is set
func (r BuildGateImageRule) Matches(exp StackExpectation) bool {
	// Language must match exactly.
	if r.Stack.Language != exp.Language {
		return false
	}
	// Release must match exactly when rule requires release.
	if r.Stack.Release != "" && r.Stack.Release != exp.Release {
		return false
	}
	// Tool must match exactly when rule requires tool.
	if r.Stack.Tool != "" && r.Stack.Tool != exp.Tool {
		return false
	}
	return true
}

// SelectorKey returns a unique key for duplicate detection.
// Two rules with the same selector key define the same match criteria
// and should not coexist within the same precedence level.
//
// Format: "language:release:tool" (empty release/tool become "*").
func (r BuildGateImageRule) SelectorKey() string {
	release := r.Stack.Release
	if release == "" {
		release = "*"
	}
	tool := r.Stack.Tool
	if tool == "" {
		tool = "*"
	}
	return fmt.Sprintf("%s:%s:%s", r.Stack.Language, release, tool)
}

// BuildGateImageMapping holds rules from a single source for validation.
// Each precedence level (default file, cluster inline, mig override) has
// its own mapping that is validated independently before merging.
type BuildGateImageMapping struct {
	// Images holds the image rules from this source.
	Images []BuildGateImageRule
}

// Validate checks that the mapping is well-formed.
// Validation rules:
//   - Each rule must have language (required)
//   - Each rule must have image (required)
//   - No duplicate selectors within this mapping
//
// The prefix parameter is used for error messages (e.g., "build_gate.images").
func (m BuildGateImageMapping) Validate(prefix string) error {
	seen := make(map[string]struct{}, len(m.Images))

	for i, rule := range m.Images {
		rulePrefix := fmt.Sprintf("%s[%d]", prefix, i)

		// Validate required fields.
		if strings.TrimSpace(rule.Stack.Language) == "" {
			return fmt.Errorf("%s.stack.language: required", rulePrefix)
		}
		if strings.TrimSpace(rule.Image) == "" {
			return fmt.Errorf("%s.image: required", rulePrefix)
		}

		// Check for duplicate selectors.
		key := rule.SelectorKey()
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s: duplicate selector %q", rulePrefix, key)
		}
		seen[key] = struct{}{}
	}

	return nil
}
