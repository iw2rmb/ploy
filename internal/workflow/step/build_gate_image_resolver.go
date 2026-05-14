// build_gate_image_resolver.go implements the Build Gate image catalog resolver.
//
// The resolver selects runtime images for Build Gate containers when Stack Gate
// is enabled. It loads rules from multiple sources with precedence ordering:
//  1. Default gates catalog (gates/gates.yaml) - lowest precedence
//  2. Mig-level image overrides - highest precedence
//
// Resolution uses "most specific match wins" semantics:
//   - Tool-specific rules (specificity 3) beat tool-agnostic rules (specificity 2)
//   - Same-specificity ties with different images are configuration errors
package step

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// DefaultGatesCatalogPath is the repository-relative location of the default
// Build Gate gates catalog.
//
// The ploy Docker images install this file at /etc/ploy/gates/gates.yaml.
const DefaultGatesCatalogPath = "gates/gates.yaml"

var (
	errBuildGateImageMapping   = errors.New("build gate image mapping")
	errBuildGateImageRuleMatch = errors.New("build gate image rule match")
)

func resolveImageForExpectation(
	mappingPath string,
	overrides []contracts.BuildGateImageRule,
	exp contracts.StackExpectation,
	required bool,
) (string, error) {
	resolver, err := NewBuildGateImageResolver(mappingPath, overrides, required)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errBuildGateImageMapping, err)
	}
	resolved, err := resolver.Resolve(exp)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errBuildGateImageRuleMatch, err)
	}
	return resolved, nil
}

func resolveExpectedRuntimeImageForStackGate(
	mappingPath string,
	overrides []contracts.BuildGateImageRule,
	expect *contracts.StackExpectation,
) (string, error) {
	if expect == nil || strings.TrimSpace(expect.Release) == "" {
		return "", fmt.Errorf("stack gate expectation missing release")
	}
	return resolveImageForExpectation(mappingPath, overrides, *expect, true)
}

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
//   - defaultPath: Path to the default gates catalog (empty to skip file loading)
//   - migOverride: Mig-level override rules (may be nil)
//   - requireDefaultFile: Whether the default catalog file must exist
//
// Merge order (lowest to highest precedence):
//   - Default catalog rules
//   - Mig override rules
//
// Returns an error if:
//   - requireDefaultFile is true and defaultPath is set but file doesn't exist
//   - File exists but is invalid YAML or fails validation
//   - Any source has duplicate selectors
func NewBuildGateImageResolver(
	defaultPath string,
	migOverride []contracts.BuildGateImageRule,
	requireDefaultFile bool,
) (*BuildGateImageResolver, error) {
	var allRules []contracts.BuildGateImageRule

	// Load default catalog if path is provided.
	if defaultPath != "" {
		fileRules, err := loadGatesCatalogFile(defaultPath, requireDefaultFile)
		if err != nil {
			return nil, err
		}
		if len(fileRules) > 0 {
			fileRules, err = normalizeBuildGateImageRules(fileRules, "default_catalog")
			if err != nil {
				return nil, fmt.Errorf("default gates catalog: %w", err)
			}
			// Validate default catalog rules.
			mapping := contracts.BuildGateImageMapping{Images: fileRules}
			if err := mapping.Validate("default_catalog"); err != nil {
				return nil, fmt.Errorf("default gates catalog: %w", err)
			}
			allRules = append(allRules, fileRules...)
		}
	}

	// Add mig override rules (highest precedence).
	if len(migOverride) > 0 {
		normalizedOverrides, err := normalizeBuildGateImageRules(migOverride, "mig_override")
		if err != nil {
			return nil, fmt.Errorf("mig override: %w", err)
		}
		mapping := contracts.BuildGateImageMapping{Images: normalizedOverrides}
		if err := mapping.Validate("mig_override"); err != nil {
			return nil, fmt.Errorf("mig override: %w", err)
		}
		allRules = append(allRules, normalizedOverrides...)
	}

	return &BuildGateImageResolver{rules: allRules}, nil
}

func normalizeBuildGateImageRules(
	rules []contracts.BuildGateImageRule,
	prefix string,
) ([]contracts.BuildGateImageRule, error) {
	normalized := make([]contracts.BuildGateImageRule, len(rules))
	copy(normalized, rules)
	for i := range normalized {
		expanded, err := contracts.ExpandImageTemplate(normalized[i].Image, &normalized[i].Stack)
		if err != nil {
			// Allow short rules (for example language-only selectors) to keep
			// stack placeholders unresolved until Resolve() provides runtime stack.
			if strings.Contains(err.Error(), "unresolved stack placeholders:") {
				normalized[i].Image = strings.TrimSpace(normalized[i].Image)
				continue
			}
			return nil, fmt.Errorf("%s[%d].image: %w", prefix, i, err)
		}
		normalized[i].Image = strings.TrimSpace(expanded)
	}
	return normalized, nil
}

// Resolve finds the best matching image for the given stack expectation.
//
// Resolution algorithm:
//  1. Find all rules that match the expectation
//  2. Select the highest specificity match
//  3. Among same-specificity matches, the last rule wins (higher precedence)
//
// The "last rule wins" semantics enables precedence-based override: rules from
// mig-level override cluster-level, which override default catalog rules.
// Conflicts between different sources are allowed and resolved by precedence order.
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
			expanded, err := contracts.ExpandImageTemplate(matches[i].Image, &exp)
			if err != nil {
				return "", fmt.Errorf("resolve image template: %w", err)
			}
			return strings.TrimSpace(expanded), nil
		}
	}

	// This should not happen since we verified matches is non-empty and found maxSpecificity.
	return "", fmt.Errorf("internal error: no match found at specificity %d", maxSpecificity)
}

// loadGatesCatalogFile loads image rules from a YAML gates catalog.
// If the file doesn't exist and required is true, returns an error.
// If the file doesn't exist and required is false, returns nil.
func loadGatesCatalogFile(path string, required bool) ([]contracts.BuildGateImageRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if required {
				return nil, fmt.Errorf("gates catalog file required but not found: %s", path)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("read gates catalog file: %w", err)
	}

	var raw struct {
		Gates []rawCatalogEntry `yaml:"gates"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse gates catalog file: %w", err)
	}

	if len(raw.Gates) == 0 {
		return nil, nil
	}

	rules := make([]contracts.BuildGateImageRule, 0, len(raw.Gates))
	for i, item := range raw.Gates {
		prefix := fmt.Sprintf("gates[%d]", i)
		rule, err := item.toRule(prefix)
		if err != nil {
			return nil, fmt.Errorf("gates catalog file %s: %w", path, err)
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// rawCatalogEntry is the on-disk shape of a gates.yaml entry. Release accepts
// strings or numerics; yaml.v3 decodes both into `any`, which ParseReleaseValue
// coerces into a canonical string. Release is optional for language-level rules.
type rawCatalogEntry struct {
	Lang    string `yaml:"lang"`
	Tool    string `yaml:"tool"`
	Image   string `yaml:"image"`
	Release any    `yaml:"release"`
}

func (e rawCatalogEntry) toRule(prefix string) (contracts.BuildGateImageRule, error) {
	var rule contracts.BuildGateImageRule

	rule.Stack.Language = strings.TrimSpace(e.Lang)
	if rule.Stack.Language == "" {
		return rule, fmt.Errorf("%s.lang: required", prefix)
	}

	if e.Release != nil {
		releaseStr, err := contracts.ParseReleaseValue(e.Release, prefix+".release")
		if err != nil {
			return rule, err
		}
		rule.Stack.Release = strings.TrimSpace(releaseStr)
	}

	rule.Stack.Tool = strings.TrimSpace(e.Tool)

	rule.Image = strings.TrimSpace(e.Image)
	if rule.Image == "" {
		return rule, fmt.Errorf("%s.image: required", prefix)
	}

	return rule, nil
}
