package buildgate

import "testing"

// TestNormalizeLanguageHandlesJavaAliases ensures java-oriented identifiers map to the canonical key.
func TestNormalizeLanguageHandlesJavaAliases(t *testing.T) {
	if got := normalizeLanguage("Java"); got != "java" {
		t.Fatalf("expected java alias, got %q", got)
	}
	if got := normalizeLanguage("javac"); got != "java" {
		t.Fatalf("expected javac to normalise to java, got %q", got)
	}
}

// TestNormalizeLanguageHandlesJavaScriptAliases ensures javascript aliases resolve to the canonical key.
func TestNormalizeLanguageHandlesJavaScriptAliases(t *testing.T) {
	cases := []string{"js", "node", "nodejs", "JavaScript"}
	for _, input := range cases {
		if got := normalizeLanguage(input); got != "javascript" {
			t.Fatalf("expected %q to normalise to javascript, got %q", input, got)
		}
	}
}
