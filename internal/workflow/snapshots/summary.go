package snapshots

import (
	"fmt"
	"sort"
)

// summariseStrip aggregates strip rules by table for reporting.
func summariseStrip(rules []StripRule) RuleSummary {
	summary := RuleSummary{Tables: make(map[string]int)}
	summary.Total = len(rules)
	for _, rule := range rules {
		summary.Tables[rule.Table]++
	}
	return summary
}

// summariseMask aggregates mask rules by table for reporting.
func summariseMask(rules []MaskRule) RuleSummary {
	summary := RuleSummary{Tables: make(map[string]int)}
	summary.Total = len(rules)
	for _, rule := range rules {
		summary.Tables[rule.Table]++
	}
	return summary
}

// summariseSynthetic aggregates synthetic rules by table for reporting.
func summariseSynthetic(rules []SyntheticRule) RuleSummary {
	summary := RuleSummary{Tables: make(map[string]int)}
	summary.Total = len(rules)
	for _, rule := range rules {
		summary.Tables[rule.Table]++
	}
	return summary
}

// buildHighlights renders a sorted list of operations the snapshot will perform.
func buildHighlights(spec Spec) []string {
	highlights := make([]string, 0, len(spec.Strip)+len(spec.Mask)+len(spec.Synthetic))
	for _, rule := range spec.Strip {
		for _, column := range rule.Columns {
			highlights = append(highlights, fmt.Sprintf("strip %s.%s", rule.Table, column))
		}
	}
	for _, rule := range spec.Mask {
		highlights = append(highlights, fmt.Sprintf("mask %s.%s -> %s", rule.Table, rule.Column, rule.Strategy))
	}
	for _, rule := range spec.Synthetic {
		highlights = append(highlights, fmt.Sprintf("synth %s.%s -> %s", rule.Table, rule.Column, rule.Strategy))
	}
	sort.Strings(highlights)
	return highlights
}
