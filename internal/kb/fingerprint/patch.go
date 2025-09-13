// Package fingerprint provides semantic analysis and similarity detection for patches.
//
// This package implements intelligent patch fingerprinting that goes beyond
// simple content hashing to understand semantic patterns in code changes.
// It can identify similar patches even when variable names or paths differ.
//
// Key components:
//   - PatchFingerprinter: Main analysis engine for patch patterns
//   - Pattern extraction: Identifies semantic patterns (imports, syntax fixes, etc.)
//   - Normalization: Standardizes patches for consistent comparison
//   - Similarity scoring: Calculates semantic similarity between patches
//
// Supported patterns:
//   - Java import additions (java.util.Optional, java.util.List, etc.)
//   - Optional wrapper patterns (Optional.ofNullable transformations)
//   - Syntax fixes (semicolon additions, method signature changes)
//   - Comment additions and code modifications
//
// Integration points:
//   - Used by internal/kb/learning for patch deduplication
//   - Used by internal/kb/models for patch hash generation
//
// See Also:
//   - internal/kb/learning: For learning pipeline integration
//   - internal/kb/models: For Case and patch data structures
package fingerprint

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// PatchFingerprinter generates fingerprints and analyzes patch patterns
type PatchFingerprinter struct {
	patterns map[string]*regexp.Regexp
}

// NewPatchFingerprinter creates a new patch fingerprinter
func NewPatchFingerprinter() *PatchFingerprinter {
	return &PatchFingerprinter{
		patterns: initializePatterns(),
	}
}

// GenerateFingerprint creates a fingerprint for the given patch
func (pf *PatchFingerprinter) GenerateFingerprint(patch []byte) string {
	if len(patch) == 0 {
		return "empty-patch"
	}

	// Extract patterns from the raw patch first (before normalization)
	// This ensures we can detect patterns in the original content
	patterns := pf.ExtractPatterns(patch)

	if len(patterns) == 0 {
		// Fallback to hash-based fingerprint of normalized patch
		normalized := pf.NormalizePatch(patch)
		hash := sha256.Sum256(normalized)
		return fmt.Sprintf("patch-%x", hash[:8])
	}

	// Use the most specific pattern as the fingerprint
	return patterns[0]
}

// NormalizePatch normalizes a patch for consistent fingerprinting
func (pf *PatchFingerprinter) NormalizePatch(patch []byte) []byte {
	content := string(patch)

	// Normalize diff --git lines to use only filenames
	re := regexp.MustCompile(`diff --git a/.*/([^/\s]+) b/.*/([^/\s]+)`)
	content = re.ReplaceAllString(content, "diff --git a/$1 b/$2")

	// Remove git index lines completely
	re = regexp.MustCompile(`index [a-f0-9]+\.\.[a-f0-9]+ \d+\n`)
	content = re.ReplaceAllString(content, "")

	// Normalize --- and +++ lines to use only filenames, remove timestamps
	re = regexp.MustCompile(`--- a/.*/([^/\s]+).*?\n`)
	content = re.ReplaceAllString(content, "--- a/$1\n")
	re = regexp.MustCompile(`\+\+\+ b/.*/([^/\s]+).*?\n`)
	content = re.ReplaceAllString(content, "+++ b/$1\n")

	// Normalize camelCase variable names but preserve method names
	// Replace variables in assignments and declarations
	re = regexp.MustCompile(`\b([a-z][a-zA-Z0-9]*[A-Z][a-zA-Z0-9]*)\s*=`)
	content = re.ReplaceAllString(content, "VAR_NAME =")

	return []byte(content)
}

// ExtractPatterns extracts semantic patterns from the patch
func (pf *PatchFingerprinter) ExtractPatterns(patch []byte) []string {
	content := string(patch)
	var patterns []string

	// Java import patterns - match with optional whitespace after +
	if regexp.MustCompile(`\+\s*import java\.util\.Optional`).MatchString(content) {
		patterns = append(patterns, "add-import-java.util.Optional")
	}
	if regexp.MustCompile(`\+\s*import java\.util\.List`).MatchString(content) {
		patterns = append(patterns, "add-import-java.util.List")
	}
	if regexp.MustCompile(`\+\s*import java\.util\.\w+`).MatchString(content) {
		patterns = append(patterns, "java-imports-addition")
	}

	// Optional wrapper patterns
	if strings.Contains(content, "Optional.ofNullable") {
		patterns = append(patterns, "optional-wrapper-pattern")
	}
	if regexp.MustCompile(`-\s+return .+;\n\+\s+return Optional\.`).MatchString(content) {
		patterns = append(patterns, "optional-wrapper-pattern", "return-statement-modification")
	}
	// Method signature changes to use Optional<T>
	if regexp.MustCompile(`-.*\([^)]*String[^)]*\)\s*\{\s*\n\+.*\([^)]*Optional<[^>]*>[^)]*\)\s*\{`).MatchString(content) {
		patterns = append(patterns, "optional-wrapper-pattern")
	}

	// Syntax fix patterns - semicolon addition
	// Match cases where a line is identical except for adding a semicolon
	if regexp.MustCompile(`-\s*(.+)\s*\n\+\s*(.+);`).MatchString(content) {
		patterns = append(patterns, "semicolon-addition", "syntax-fix")
	}

	// Comment additions - match with optional whitespace
	if regexp.MustCompile(`\+\s*//`).MatchString(content) {
		patterns = append(patterns, "comment-addition")
	}

	// If no specific patterns found, classify by general type
	if len(patterns) == 0 {
		if strings.Contains(content, "+") && strings.Contains(content, "import") {
			patterns = append(patterns, "import-modification")
		} else if strings.Contains(content, "+") {
			patterns = append(patterns, "code-addition")
		} else if strings.Contains(content, "-") {
			patterns = append(patterns, "code-removal")
		}
	}

	return patterns
}

// CalculateSimilarity calculates similarity between two patches (0.0 - 1.0)
func (pf *PatchFingerprinter) CalculateSimilarity(patch1, patch2 []byte) float64 {
	if len(patch1) == 0 && len(patch2) == 0 {
		return 1.0
	}
	if len(patch1) == 0 || len(patch2) == 0 {
		return 0.0
	}

	// Extract patterns from both patches
	patterns1 := pf.ExtractPatterns(pf.NormalizePatch(patch1))
	patterns2 := pf.ExtractPatterns(pf.NormalizePatch(patch2))

	if len(patterns1) == 0 && len(patterns2) == 0 {
		// Fall back to content similarity
		return pf.calculateContentSimilarity(patch1, patch2)
	}

	// Calculate pattern overlap
	common := 0
	total := len(patterns1) + len(patterns2)

	pattern1Set := make(map[string]bool)
	for _, p := range patterns1 {
		pattern1Set[p] = true
	}

	for _, p := range patterns2 {
		if pattern1Set[p] {
			common++
		}
	}

	if total == 0 {
		return 0.0
	}

	return (2.0 * float64(common)) / float64(total)
}

// calculateContentSimilarity calculates basic content similarity
func (pf *PatchFingerprinter) calculateContentSimilarity(patch1, patch2 []byte) float64 {
	// Simple implementation: check if patches are identical
	if string(patch1) == string(patch2) {
		return 1.0
	}

	// Calculate character-level similarity (simplified)
	content1 := string(pf.NormalizePatch(patch1))
	content2 := string(pf.NormalizePatch(patch2))

	if content1 == content2 {
		return 0.8 // High similarity for normalized content match
	}

	// Basic substring matching
	shorter, longer := content1, content2
	if len(content1) > len(content2) {
		shorter, longer = content2, content1
	}

	if strings.Contains(longer, shorter) {
		return 0.5
	}

	return 0.1 // Low default similarity
}

// initializePatterns initializes regex patterns for patch analysis
func initializePatterns() map[string]*regexp.Regexp {
	patterns := make(map[string]*regexp.Regexp)

	// Define common patch patterns
	patterns["java-import"] = regexp.MustCompile(`\+import java\.`)
	patterns["optional-wrapper"] = regexp.MustCompile(`Optional\.ofNullable`)
	patterns["semicolon-fix"] = regexp.MustCompile(`-(.+)\n\+(.+);`)
	patterns["method-signature"] = regexp.MustCompile(`-\s+public .+ \w+\(.*\)\s*\{\n\+\s+public .+ \w+\(.*\)\s*\{`)

	return patterns
}
