package main

import "strings"

func selectorDescription(all bool, selector string) string {
	if all {
		return "all nodes"
	}
	return selector
}

func filterNodes(nodes []nodeInfo, all bool, selector string) []nodeInfo {
	if all {
		return nodes
	}

	// Simple pattern matching: selector can be a glob-like pattern with '*'.
	var filtered []nodeInfo
	for _, n := range nodes {
		if matchesSelector(n.Name, selector) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func matchesSelector(name, pattern string) bool {
	// Simple glob matching: support '*' as wildcard.
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		// Exact match.
		return name == pattern
	}

	// Split pattern by '*' and check each part.
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 && parts[0] == "" {
		// Pattern: *suffix
		return strings.HasSuffix(name, parts[1])
	}
	if len(parts) == 2 && parts[1] == "" {
		// Pattern: prefix*
		return strings.HasPrefix(name, parts[0])
	}
	if len(parts) == 2 {
		// Pattern: prefix*suffix
		return strings.HasPrefix(name, parts[0]) && strings.HasSuffix(name, parts[1])
	}

	// For more complex patterns, fall back to basic substring matching.
	for _, part := range parts {
		if part != "" && !strings.Contains(name, part) {
			return false
		}
	}
	return true
}
