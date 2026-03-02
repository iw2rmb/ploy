// build_gate_image_resolver.go implements the Build Gate stack catalog resolver.
//
// The resolver selects runtime images for Build Gate containers when Stack Gate
// is enabled. It loads rules from multiple sources with precedence ordering:
//  1. Default stacks catalog (gates/stacks.yaml) - lowest precedence
//  2. Mod-level image overrides - highest precedence
//
// Resolution uses "most specific match wins" semantics:
//   - Tool-specific rules (specificity 3) beat tool-agnostic rules (specificity 2)
//   - Same-specificity ties with different images are configuration errors
package step

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// DefaultStacksCatalogPath is the repository-relative location of the default
// Build Gate stacks catalog.
//
// The ploy Docker images install this file at /etc/ploy/gates/stacks.yaml.
const DefaultStacksCatalogPath = "gates/stacks.yaml"

const (
	containerRegistryEnvKey    = "PLOY_CONTAINER_REGISTRY"
	defaultRegistryImagePrefix = "127.0.0.1:5000/ploy"
)

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
//   - defaultPath: Path to the default stacks catalog (empty to skip file loading)
//   - modOverride: Mod-level override rules (may be nil)
//   - requireDefaultFile: Whether the default catalog file must exist
//
// Merge order (lowest to highest precedence):
//   - Default catalog rules
//   - Mod override rules
//
// Returns an error if:
//   - requireDefaultFile is true and defaultPath is set but file doesn't exist
//   - File exists but is invalid YAML or fails validation
//   - Any source has duplicate selectors
func NewBuildGateImageResolver(
	defaultPath string,
	modOverride []contracts.BuildGateImageRule,
	requireDefaultFile bool,
) (*BuildGateImageResolver, error) {
	var allRules []contracts.BuildGateImageRule

	// Load default catalog if path is provided.
	if defaultPath != "" {
		fileRules, err := loadStacksCatalogFile(defaultPath, requireDefaultFile)
		if err != nil {
			return nil, err
		}
		if len(fileRules) > 0 {
			fileRules = normalizeBuildGateImageRules(fileRules)
			// Validate default catalog rules.
			mapping := contracts.BuildGateImageMapping{Images: fileRules}
			if err := mapping.Validate("default_catalog"); err != nil {
				return nil, fmt.Errorf("default stacks catalog: %w", err)
			}
			allRules = append(allRules, fileRules...)
		}
	}

	// Add mig override rules (highest precedence).
	if len(modOverride) > 0 {
		normalizedOverrides := normalizeBuildGateImageRules(modOverride)
		mapping := contracts.BuildGateImageMapping{Images: normalizedOverrides}
		if err := mapping.Validate("mod_override"); err != nil {
			return nil, fmt.Errorf("mig override: %w", err)
		}
		allRules = append(allRules, normalizedOverrides...)
	}

	return &BuildGateImageResolver{rules: allRules}, nil
}

func normalizeBuildGateImageRules(rules []contracts.BuildGateImageRule) []contracts.BuildGateImageRule {
	normalized := make([]contracts.BuildGateImageRule, len(rules))
	copy(normalized, rules)
	for i := range normalized {
		normalized[i].Image = expandContainerRegistryPrefix(normalized[i].Image)
	}
	return normalized
}

func expandContainerRegistryPrefix(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return image
	}
	prefix := resolveContainerRegistryPrefix()
	expanded := strings.ReplaceAll(image, "${"+containerRegistryEnvKey+"}", prefix)
	expanded = strings.ReplaceAll(expanded, "$"+containerRegistryEnvKey, prefix)
	return strings.TrimSpace(expanded)
}

func resolveContainerRegistryPrefix() string {
	prefix := strings.TrimSpace(os.Getenv(containerRegistryEnvKey))
	if prefix == "" {
		return defaultRegistryImagePrefix
	}
	return strings.TrimRight(prefix, "/")
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
			return matches[i].Image, nil
		}
	}

	// This should not happen since we verified matches is non-empty and found maxSpecificity.
	return "", fmt.Errorf("internal error: no match found at specificity %d", maxSpecificity)
}

// loadStacksCatalogFile loads image rules from a YAML stacks catalog.
// If the file doesn't exist and required is true, returns an error.
// If the file doesn't exist and required is false, returns nil.
func loadStacksCatalogFile(path string, required bool) ([]contracts.BuildGateImageRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if required {
				return nil, fmt.Errorf("stacks catalog file required but not found: %s", path)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("read stacks catalog file: %w", err)
	}

	// Parse YAML into intermediate structure.
	var raw struct {
		Stacks []map[string]any `yaml:"stacks"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse stacks catalog file: %w", err)
	}

	if len(raw.Stacks) == 0 {
		return nil, nil
	}

	// Convert to typed rules and validate referenced profile files exist.
	rules := make([]contracts.BuildGateImageRule, 0, len(raw.Stacks))
	for i, item := range raw.Stacks {
		rule, profilePath, err := parseStackCatalogEntry(item, fmt.Sprintf("stacks[%d]", i))
		if err != nil {
			return nil, fmt.Errorf("stacks catalog file %s: %w", path, err)
		}
		if err := ensureCatalogProfileExists(path, profilePath, fmt.Sprintf("stacks[%d].profile", i)); err != nil {
			return nil, fmt.Errorf("stacks catalog file %s: %w", path, err)
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// parseStackCatalogEntry parses a single stack catalog entry from a YAML map.
func parseStackCatalogEntry(raw map[string]any, prefix string) (contracts.BuildGateImageRule, string, error) {
	var rule contracts.BuildGateImageRule
	var profilePath string

	lang, ok := raw["lang"]
	if !ok || lang == nil {
		return rule, "", fmt.Errorf("%s.lang: required", prefix)
	}
	langStr, ok := lang.(string)
	if !ok {
		return rule, "", fmt.Errorf("%s.lang: expected string, got %T", prefix, lang)
	}
	rule.Stack.Language = strings.TrimSpace(langStr)
	if rule.Stack.Language == "" {
		return rule, "", fmt.Errorf("%s.lang: required", prefix)
	}

	release, ok := raw["release"]
	if !ok || release == nil {
		return rule, "", fmt.Errorf("%s.release: required", prefix)
	}
	releaseStr, err := contracts.ParseReleaseValue(release, prefix+".release")
	if err != nil {
		return rule, "", err
	}
	rule.Stack.Release = strings.TrimSpace(releaseStr)
	if rule.Stack.Release == "" {
		return rule, "", fmt.Errorf("%s.release: required", prefix)
	}

	if v, ok := raw["tool"]; ok && v != nil {
		toolStr, ok := v.(string)
		if !ok {
			return rule, "", fmt.Errorf("%s.tool: expected string, got %T", prefix, v)
		}
		rule.Stack.Tool = strings.TrimSpace(toolStr)
	}

	image, ok := raw["image"]
	if !ok || image == nil {
		return rule, "", fmt.Errorf("%s.image: required", prefix)
	}
	imageStr, ok := image.(string)
	if !ok {
		return rule, "", fmt.Errorf("%s.image: expected string, got %T", prefix, image)
	}
	rule.Image = strings.TrimSpace(imageStr)
	if rule.Image == "" {
		return rule, "", fmt.Errorf("%s.image: required", prefix)
	}

	profile, ok := raw["profile"]
	if !ok || profile == nil {
		return rule, "", fmt.Errorf("%s.profile: required", prefix)
	}
	profileStr, ok := profile.(string)
	if !ok {
		return rule, "", fmt.Errorf("%s.profile: expected string, got %T", prefix, profile)
	}
	profilePath = strings.TrimSpace(profileStr)
	if profilePath == "" {
		return rule, "", fmt.Errorf("%s.profile: required", prefix)
	}

	return rule, profilePath, nil
}

func ensureCatalogProfileExists(catalogPath, profileRef, field string) error {
	resolved := resolveCatalogAssetPath(catalogPath, profileRef)
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s: referenced file does not exist %q (resolved to %q)", field, profileRef, resolved)
		}
		return fmt.Errorf("%s: stat referenced file %q: %w", field, profileRef, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s: referenced file is a directory %q (resolved to %q)", field, profileRef, resolved)
	}
	return nil
}

func resolveCatalogAssetPath(catalogPath, assetRef string) string {
	cleanRef := path.Clean(strings.TrimSpace(assetRef))
	if filepath.IsAbs(cleanRef) {
		return cleanRef
	}
	catalogDir := filepath.Dir(catalogPath)
	if strings.HasPrefix(cleanRef, "gates/") {
		return filepath.Join(filepath.Dir(catalogDir), filepath.FromSlash(cleanRef))
	}
	return filepath.Join(catalogDir, filepath.FromSlash(cleanRef))
}
