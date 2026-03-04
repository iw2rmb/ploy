package assertx

import (
	"strings"
	"testing"
)

func Contains(t testing.TB, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q, got: %q", needle, haystack)
	}
}

func NotContains(t testing.TB, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("expected output not to contain %q, got: %q", needle, haystack)
	}
}
