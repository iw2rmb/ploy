package main

import "testing"

func TestAsterEnabledReadsEnvironment(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		expected bool
	}{
		{name: "unset", value: "", expected: false},
		{name: "zero", value: "0", expected: false},
		{name: "false", value: "false", expected: false},
		{name: "no", value: "no", expected: false},
		{name: "one", value: "1", expected: true},
		{name: "true", value: "true", expected: true},
		{name: "yes", value: "yes", expected: true},
		{name: "whitespace", value: "   true  ", expected: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.value == "" {
				t.Setenv("PLOY_ASTER_ENABLE", "")
			} else {
				t.Setenv("PLOY_ASTER_ENABLE", tc.value)
			}
			if got := asterEnabled(); got != tc.expected {
				t.Fatalf("asterEnabled() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestAsterEnabledClearsEnvironment(t *testing.T) {
	t.Setenv("PLOY_ASTER_ENABLE", "")
	if asterEnabled() {
		t.Fatal("expected asterEnabled to be false when env unset")
	}
}
