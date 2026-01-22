// build_gate_image_rule_wire.go provides wire serialization for Build Gate image rules.
//
// These functions convert typed BuildGateImageRule back to map[string]any for
// wire transmission. Only non-empty fields are included (omitempty semantics).
package contracts

// buildGateImageRuleToMap converts a BuildGateImageRule to map[string]any for wire serialization.
func buildGateImageRuleToMap(rule BuildGateImageRule) map[string]any {
	result := make(map[string]any)

	// Add stack if non-empty.
	if !rule.Stack.IsEmpty() {
		result["stack"] = stackExpectationToMap(&rule.Stack)
	}

	// Add image if non-empty.
	if rule.Image != "" {
		result["image"] = rule.Image
	}

	return result
}

// buildGateImageRulesToAny converts a slice of BuildGateImageRule to []any for wire serialization.
// Returns nil if the slice is empty.
func buildGateImageRulesToAny(rules []BuildGateImageRule) []any {
	if len(rules) == 0 {
		return nil
	}

	result := make([]any, 0, len(rules))
	for _, rule := range rules {
		result = append(result, buildGateImageRuleToMap(rule))
	}

	return result
}
