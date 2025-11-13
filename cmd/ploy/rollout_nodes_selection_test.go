package main

import "testing"

func TestFilterNodesAll(t *testing.T) {
	nodes := []nodeInfo{
		{ID: "1", Name: "worker-1", IPAddress: "10.0.0.1"},
		{ID: "2", Name: "worker-2", IPAddress: "10.0.0.2"},
		{ID: "3", Name: "server-1", IPAddress: "10.0.0.3"},
	}

	filtered := filterNodes(nodes, true, "")
	if len(filtered) != 3 {
		t.Fatalf("expected 3 nodes with --all, got %d", len(filtered))
	}
}

func TestFilterNodesSelector(t *testing.T) {
	nodes := []nodeInfo{
		{ID: "1", Name: "worker-1", IPAddress: "10.0.0.1"},
		{ID: "2", Name: "worker-2", IPAddress: "10.0.0.2"},
		{ID: "3", Name: "server-1", IPAddress: "10.0.0.3"},
	}

	tests := []struct {
		name     string
		selector string
		expected int
	}{
		{"prefix wildcard", "worker-*", 2},
		{"suffix wildcard", "*-1", 2},
		{"exact match", "worker-1", 1},
		{"no match", "non-existent", 0},
		{"match all wildcard", "*", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterNodes(nodes, false, tt.selector)
			if len(filtered) != tt.expected {
				t.Fatalf("selector %q: expected %d nodes, got %d", tt.selector, tt.expected, len(filtered))
			}
		})
	}
}

func TestMatchesSelector(t *testing.T) {
	tests := []struct {
		name     string
		nodeName string
		pattern  string
		expected bool
	}{
		{"exact match", "worker-1", "worker-1", true},
		{"prefix wildcard match", "worker-1", "worker-*", true},
		{"prefix wildcard no match", "server-1", "worker-*", false},
		{"suffix wildcard match", "worker-1", "*-1", true},
		{"suffix wildcard no match", "worker-2", "*-1", false},
		{"wildcard match", "anything", "*", true},
		{"prefix-suffix match", "worker-1-prod", "worker-*-prod", true},
		{"prefix-suffix no match", "worker-1-dev", "worker-*-prod", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesSelector(tt.nodeName, tt.pattern)
			if result != tt.expected {
				t.Fatalf("matchesSelector(%q, %q): expected %v, got %v", tt.nodeName, tt.pattern, tt.expected, result)
			}
		})
	}
}
