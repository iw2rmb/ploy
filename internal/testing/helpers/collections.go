package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MergeStringMaps merges multiple string maps into one
func MergeStringMaps(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// CopyStringMap creates a copy of a string map
func CopyStringMap(original map[string]string) map[string]string {
	copy := make(map[string]string)
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// StringMapEquals checks if two string maps are equal
func StringMapEquals(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// AssertStringMapEquals asserts that two string maps are equal
func AssertStringMapEquals(t testing.TB, expected, actual map[string]string) {
	t.Helper()
	assert.Equal(t, expected, actual, "String maps should be equal")
}

// AssertStringMapContains asserts that a map contains specific key-value pairs
func AssertStringMapContains(t testing.TB, actual map[string]string, expected map[string]string) {
	t.Helper()
	for key, expectedValue := range expected {
		actualValue, exists := actual[key]
		assert.True(t, exists, "Map should contain key: %s", key)
		assert.Equal(t, expectedValue, actualValue, "Value for key %s should match", key)
	}
}

// StringSliceContains checks if a string slice contains a value
func StringSliceContains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

// StringSliceContainsAll checks if a slice contains all specified values
func StringSliceContainsAll(slice []string, values ...string) bool {
	for _, value := range values {
		if !StringSliceContains(slice, value) {
			return false
		}
	}
	return true
}

// AssertSliceContains asserts that a slice contains a specific value
func AssertSliceContains(t testing.TB, slice []string, value string) {
	t.Helper()
	assert.Contains(t, slice, value, "Slice should contain value: %s", value)
}

// AssertSliceContainsAll asserts that a slice contains all specified values
func AssertSliceContainsAll(t testing.TB, slice []string, values ...string) {
	t.Helper()
	for _, value := range values {
		assert.Contains(t, slice, value, "Slice should contain value: %s", value)
	}
}
