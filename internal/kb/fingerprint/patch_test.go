package fingerprint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchFingerprinter_GenerateFingerprint(t *testing.T) {
	tests := []struct {
		name        string
		patch       []byte
		expected    string
		description string
	}{
		{
			name: "java import addition",
			patch: []byte(`diff --git a/Main.java b/Main.java
index 1234567..abcdefg 100644
--- a/Main.java
+++ b/Main.java
@@ -1,3 +1,4 @@
 package com.example;
 
+import java.util.Optional;
 public class Main {`),
			expected:    "add-import-java.util.Optional",
			description: "Should recognize Java import addition pattern",
		},
		{
			name: "method signature change",
			patch: []byte(`diff --git a/Service.java b/Service.java
--- a/Service.java
+++ b/Service.java
@@ -10,7 +10,7 @@
-    public void process(String input) {
+    public void process(Optional<String> input) {
         // method body`),
			expected:    "optional-wrapper-pattern",
			description: "Should recognize method signature modernization",
		},
		{
			name: "semicolon fix",
			patch: []byte(`diff --git a/App.java b/App.java
--- a/App.java
+++ b/App.java
@@ -5,7 +5,7 @@
-        System.out.println("Hello")
+        System.out.println("Hello");
         return result;`),
			expected:    "semicolon-addition",
			description: "Should recognize syntax error fixes",
		},
		{
			name:        "empty patch",
			patch:       []byte(""),
			expected:    "empty-patch",
			description: "Should handle empty patches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - PatchFingerprinter doesn't exist yet
			fingerprinter := NewPatchFingerprinter()
			require.NotNil(t, fingerprinter)

			fingerprint := fingerprinter.GenerateFingerprint(tt.patch)
			assert.NotEmpty(t, fingerprint, "Fingerprint should not be empty")
			assert.Equal(t, tt.expected, fingerprint, tt.description)

			// Same patch should generate same fingerprint
			fingerprint2 := fingerprinter.GenerateFingerprint(tt.patch)
			assert.Equal(t, fingerprint, fingerprint2, "Fingerprints should be deterministic")
		})
	}
}

func TestPatchFingerprinter_NormalizePatch(t *testing.T) {
	tests := []struct {
		name     string
		patch    []byte
		expected []byte
	}{
		{
			name: "normalize file paths and timestamps",
			patch: []byte(`diff --git a/src/main/java/com/company/App.java b/src/main/java/com/company/App.java
index 1234567..abcdefg 100644
--- a/src/main/java/com/company/App.java    2024-01-01 10:30:00
+++ b/src/main/java/com/company/App.java    2024-01-01 10:31:00
@@ -1,3 +1,4 @@
+import java.util.Optional;
 public class App {`),
			expected: []byte(`diff --git a/App.java b/App.java
--- a/App.java
+++ b/App.java
@@ -1,3 +1,4 @@
+import java.util.Optional;
 public class App {`),
		},
		{
			name: "normalize variable names in patches",
			patch: []byte(`@@ -10,7 +10,7 @@
-    String userNameFromRequest = request.getName();
+    Optional<String> userNameFromRequest = Optional.ofNullable(request.getName());`),
			expected: []byte(`@@ -10,7 +10,7 @@
-    String VAR_NAME = request.getName();
+    Optional<String> VAR_NAME = Optional.ofNullable(request.getName());`),
		},
		{
			name:     "preserve structural changes",
			patch:    []byte(`+import java.util.Optional;\n-import java.util.List;`),
			expected: []byte(`+import java.util.Optional;\n-import java.util.List;`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - NormalizePatch doesn't exist yet
			fingerprinter := NewPatchFingerprinter()

			normalized := fingerprinter.NormalizePatch(tt.patch)
			assert.Equal(t, tt.expected, normalized)
		})
	}
}

func TestPatchFingerprinter_ExtractPatterns(t *testing.T) {
	tests := []struct {
		name             string
		patch            []byte
		expectedPatterns []string
	}{
		{
			name: "java import pattern",
			patch: []byte(`+import java.util.Optional;
+import java.util.List;`),
			expectedPatterns: []string{
				"add-import-java.util.Optional",
				"add-import-java.util.List",
				"java-imports-addition",
			},
		},
		{
			name: "method wrapper pattern",
			patch: []byte(`-    return value;
+    return Optional.ofNullable(value);`),
			expectedPatterns: []string{
				"optional-wrapper-pattern",
				"return-statement-modification",
			},
		},
		{
			name: "syntax fix pattern",
			patch: []byte(`-        System.out.println("test")
+        System.out.println("test");`),
			expectedPatterns: []string{
				"semicolon-addition",
				"syntax-fix",
			},
		},
		{
			name:             "unrecognized pattern",
			patch:            []byte(`+// This is a comment`),
			expectedPatterns: []string{"comment-addition"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - ExtractPatterns doesn't exist yet
			fingerprinter := NewPatchFingerprinter()

			patterns := fingerprinter.ExtractPatterns(tt.patch)
			assert.NotEmpty(t, patterns, "Should extract at least one pattern")

			for _, expected := range tt.expectedPatterns {
				assert.Contains(t, patterns, expected, "Should contain expected pattern: %s", expected)
			}
		})
	}
}

func TestPatchFingerprinter_SimilarityScore(t *testing.T) {
	fingerprinter := NewPatchFingerprinter()

	// Base patch for comparison
	basePatch := []byte(`+import java.util.Optional;
-    String value = getValue();
+    Optional<String> value = Optional.ofNullable(getValue());`)

	tests := []struct {
		name            string
		comparePatch    []byte
		expectedScore   float64
		description     string
	}{
		{
			name:          "identical patches",
			comparePatch:  basePatch,
			expectedScore: 1.0,
			description:   "Identical patches should have similarity score of 1.0",
		},
		{
			name: "similar optional wrapper pattern",
			comparePatch: []byte(`+import java.util.Optional;
-    String name = getName();
+    Optional<String> name = Optional.ofNullable(getName());`),
			expectedScore: 0.85,
			description:   "Similar Optional wrapper pattern should have high similarity",
		},
		{
			name: "different pattern",
			comparePatch: []byte(`+import java.util.List;
+    List<String> items = new ArrayList<>();`),
			expectedScore: 0.2,
			description:   "Different patterns should have low similarity",
		},
		{
			name:          "empty patch",
			comparePatch:  []byte(""),
			expectedScore: 0.0,
			description:   "Empty patch should have zero similarity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - CalculateSimilarity doesn't exist yet
			score := fingerprinter.CalculateSimilarity(basePatch, tt.comparePatch)
			
			assert.True(t, score >= 0.0 && score <= 1.0, "Similarity score should be between 0.0 and 1.0")
			assert.InDelta(t, tt.expectedScore, score, 0.1, tt.description)
		})
	}
}