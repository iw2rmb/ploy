package mods

import "testing"

func TestDetectLanguageFromBuildError(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"[ERROR] javac: cannot find symbol", "java"},
		{"go build ./...\npackage main", "go"},
		{"tsc --noEmit\nnode_modules warning", "typescript"},
		{"Traceback (most recent call last):\nImportError: foo\npytest", "python"},
		{"error[E0425]: cannot find value `x` in this scope\nrustc 1.79", "rust"},
		{"some unknown tool output", "unknown"},
	}

	for _, c := range cases {
		if got := DetectLanguageFromBuildError(c.in); got != c.want {
			t.Fatalf("DetectLanguageFromBuildError(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("hello world", "nope", "world") {
		t.Fatal("expected true when one substring matches")
	}
	if containsAny("abcdef", "gh", "ij") {
		t.Fatal("expected false when no substrings match")
	}
	if !containsAny("maven build failed", "gradle", "maven") {
		t.Fatal("expected true for second match")
	}
}
