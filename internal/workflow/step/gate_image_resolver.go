// gate_image_resolver.go resolves Build Gate runtime images from image rules.
//
// Rules come from two sources, lowest precedence first:
//  1. Default gates catalog (gates/gates.yaml)
//  2. Mig-level image overrides
//
// Resolution uses "most specific match wins" semantics over the four
// specificity levels that contracts.BuildGateImageRule defines. Among matches
// at the top level, the rule with the higher precedence wins.
package step

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// defaultCatalogPath is the repository-relative location of the default
// Build Gate gates catalog.
//
// The ploy Docker images install this file at /etc/ploy/gates/gates.yaml.
const defaultCatalogPath = "gates/gates.yaml"

// defaultCatalogFilePath resolves the active gates catalog file:
// the installed copy at /etc/ploy/gates/gates.yaml when present, otherwise
// the repo-relative defaultCatalogPath discovered by walking up to a
// go.mod ancestor.
func defaultCatalogFilePath() string {
	installed := "/etc/ploy/gates/gates.yaml"
	if info, err := os.Stat(installed); err == nil && !info.IsDir() {
		return installed
	}
	wd, err := os.Getwd()
	if err == nil {
		dir := wd
		for {
			if info, serr := os.Stat(filepath.Join(dir, "go.mod")); serr == nil && !info.IsDir() {
				candidate := filepath.Join(dir, defaultCatalogPath)
				if info, serr := os.Stat(candidate); serr == nil && !info.IsDir() {
					return candidate
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return defaultCatalogPath
}

var (
	errImageMapping   = errors.New("build gate image mapping")
	errImageRuleMatch = errors.New("build gate image rule match")
)

// resolveImageForExpectation returns the image of the best rule for exp from
// the catalog at catalogPath and the mig overrides.
func resolveImageForExpectation(
	catalogPath string,
	overrides []contracts.BuildGateImageRule,
	exp contracts.StackExpectation,
) (string, error) {
	rules, err := loadImageRules(catalogPath, overrides)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errImageMapping, err)
	}
	image, err := selectImage(rules, exp)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errImageRuleMatch, err)
	}
	return image, nil
}

// loadImageRules returns the catalog rules followed by the override rules, so
// later rules have higher precedence. An empty catalogPath skips the catalog.
func loadImageRules(
	catalogPath string,
	overrides []contracts.BuildGateImageRule,
) ([]contracts.BuildGateImageRule, error) {
	var rules []contracts.BuildGateImageRule
	if catalogPath != "" {
		catalogRules, err := loadCatalogFile(catalogPath)
		if err != nil {
			return nil, err
		}
		mapping := contracts.BuildGateImageMapping{Images: catalogRules}
		if err := mapping.Validate("default_catalog"); err != nil {
			return nil, fmt.Errorf("default gates catalog: %w", err)
		}
		rules = append(rules, catalogRules...)
	}
	mapping := contracts.BuildGateImageMapping{Images: overrides}
	if err := mapping.Validate("mig_override"); err != nil {
		return nil, fmt.Errorf("mig override: %w", err)
	}
	return append(rules, overrides...), nil
}

// selectImage returns the expanded image of the highest-specificity rule that
// matches exp. Among rules at that specificity, the last rule wins.
//
// The image template expands only for the selected rule and with the runtime
// stack: short rules (for example language-only selectors) cannot fill stack
// placeholders from their own selector.
func selectImage(rules []contracts.BuildGateImageRule, exp contracts.StackExpectation) (string, error) {
	if len(rules) == 0 {
		return "", fmt.Errorf("no image mapping rules available for stack %s:%s:%s",
			exp.Language, exp.Release, exp.Tool)
	}

	var best *contracts.BuildGateImageRule
	for i := range rules {
		rule := &rules[i]
		if !rule.Matches(exp) {
			continue
		}
		if best == nil || rule.Specificity() >= best.Specificity() {
			best = rule
		}
	}
	if best == nil {
		return "", fmt.Errorf("no image rule matches stack %s:%s:%s",
			exp.Language, exp.Release, exp.Tool)
	}

	expanded, err := contracts.ExpandImageTemplate(best.Image, &exp)
	if err != nil {
		return "", fmt.Errorf("resolve image template: %w", err)
	}
	return strings.TrimSpace(expanded), nil
}

// loadCatalogFile loads image rules from a YAML gates catalog.
func loadCatalogFile(path string) ([]contracts.BuildGateImageRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("gates catalog file required but not found: %s", path)
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
