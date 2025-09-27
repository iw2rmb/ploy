package main

import "strings"

// splitToggles normalises comma separated toggle strings into a slice.
func splitToggles(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if candidate := strings.TrimSpace(part); candidate != "" {
			result = append(result, candidate)
		}
	}
	return result
}
