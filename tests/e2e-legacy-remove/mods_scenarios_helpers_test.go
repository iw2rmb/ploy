//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func assertStageSet(t *testing.T, actual, expected []string) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("stage count mismatch: got %d want %d (%v vs %v)", len(actual), len(expected), actual, expected)
	}
	unmatched := make(map[string]int, len(actual))
	for _, name := range actual {
		unmatched[name]++
	}
	for _, name := range expected {
		if unmatched[name] == 0 {
			t.Fatalf("missing stage %s in %v", name, actual)
		}
		unmatched[name]--
		if unmatched[name] == 0 {
			delete(unmatched, name)
		}
	}
	if len(unmatched) != 0 {
		t.Fatalf("unexpected stages present: %v", unmatched)
	}
}

func containsHealing(names []string) bool {
	for _, name := range names {
		if strings.Contains(name, "#heal") {
			return true
		}
	}
	return false
}

func idx(target string, values []string) int {
	return scenarioIndex(values, target)
}

func containsValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
