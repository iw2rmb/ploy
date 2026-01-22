// build_gate_image_rule_parse.go provides parsing functions for Build Gate image rules.
//
// These functions parse BuildGateImageRule from map[string]any intermediate
// representations (from JSON/YAML input). They use the existing expect*
// helpers from parse_helpers.go for consistent error handling.
package contracts

import (
	"fmt"
	"strings"
)

// parseBuildGateImageRule parses a single BuildGateImageRule from a raw map.
// Expected format:
//
//	{
//	  "stack": { "language": "java", "release": "17", "tool": "maven" },
//	  "image": "maven:3-eclipse-temurin-17"
//	}
func parseBuildGateImageRule(raw map[string]any, prefix string) (BuildGateImageRule, error) {
	var rule BuildGateImageRule

	// Parse stack expectation.
	if v, ok := raw["stack"]; ok && v != nil {
		stackRaw, err := expectMap(v, prefix+".stack")
		if err != nil {
			return rule, err
		}
		exp, err := parseStackExpectation(stackRaw, prefix+".stack")
		if err != nil {
			return rule, err
		}
		if exp != nil {
			rule.Stack = *exp
		}
	}

	// Parse image.
	if v, ok := raw["image"]; ok && v != nil {
		s, err := expectString(v, prefix+".image")
		if err != nil {
			return rule, err
		}
		rule.Image = strings.TrimSpace(s)
	}

	return rule, nil
}

// parseBuildGateImageRules parses an array of BuildGateImageRule from raw input.
// Expected format:
//
//	[
//	  { "stack": { "language": "java", "release": "17", "tool": "maven" }, "image": "maven:3-eclipse-temurin-17" },
//	  { "stack": { "language": "java", "release": "17" }, "image": "eclipse-temurin:17-jdk" }
//	]
func parseBuildGateImageRules(raw []any, prefix string) ([]BuildGateImageRule, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	rules := make([]BuildGateImageRule, 0, len(raw))
	for i, item := range raw {
		itemMap, err := expectMap(item, fmt.Sprintf("%s[%d]", prefix, i))
		if err != nil {
			return nil, err
		}
		rule, err := parseBuildGateImageRule(itemMap, fmt.Sprintf("%s[%d]", prefix, i))
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}
