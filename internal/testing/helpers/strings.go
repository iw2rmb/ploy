package helpers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ContainsAll checks if a string contains all specified substrings
func ContainsAll(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if !strings.Contains(s, substring) {
			return false
		}
	}
	return true
}

// ContainsAny checks if a string contains any of the specified substrings
func ContainsAny(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if strings.Contains(s, substring) {
			return true
		}
	}
	return false
}

// AssertContainsAll asserts that a string contains all specified substrings
func AssertContainsAll(t testing.TB, s string, substrings ...string) {
	t.Helper()
	for _, substring := range substrings {
		assert.Contains(t, s, substring, "String should contain: %s", substring)
	}
}

// AssertContainsAny asserts that a string contains at least one of the specified substrings
func AssertContainsAny(t testing.TB, s string, substrings ...string) {
	t.Helper()
	assert.True(t, ContainsAny(s, substrings...), "String should contain at least one of: %v", substrings)
}
