package mods

import (
	"testing"
)

func TestDefaultEnhancedSignatureGenerator_ComputeSignatureSimilarity(t *testing.T) {
	generator := NewEnhancedSignatureGenerator(nil)

	tests := []struct {
		name             string
		sig1             string
		sig2             string
		lang             string
		compiler         string
		expectSimilarity float64
	}{
		{
			name:             "identical signatures",
			sig1:             "abcdef1234567890",
			sig2:             "abcdef1234567890",
			lang:             "java",
			compiler:         "javac",
			expectSimilarity: 1.0,
		},
		{
			name:             "completely different signatures",
			sig1:             "0000000000000000",
			sig2:             "ffffffffffffffff",
			lang:             "java",
			compiler:         "javac",
			expectSimilarity: 0.0,
		},
		{
			name:             "partially similar signatures",
			sig1:             "abcd00000000000",
			sig2:             "abcd11111111111",
			lang:             "java",
			compiler:         "javac",
			expectSimilarity: 0.25, // 4 out of 16 characters match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := generator.ComputeSignatureSimilarity(tt.sig1, tt.sig2, tt.lang, tt.compiler)
			if similarity != tt.expectSimilarity {
				t.Errorf("ComputeSignatureSimilarity() = %v, want %v", similarity, tt.expectSimilarity)
			}
		})
	}
}

func TestDefaultEnhancedSignatureGenerator_ComputePatchSimilarity(t *testing.T) {
	generator := NewEnhancedSignatureGenerator(nil)

	tests := []struct {
		name      string
		patch1    []byte
		patch2    []byte
		expectSim float64
		expectGT  float64 // expect greater than this value
	}{
		{
			name:      "identical patches",
			patch1:    []byte("--- [FILE_A]\n+++ [FILE_B]\n+import java.util.List;"),
			patch2:    []byte("--- [FILE_A]\n+++ [FILE_B]\n+import java.util.List;"),
			expectSim: 1.0,
		},
		{
			name:     "similar imports",
			patch1:   []byte("--- [FILE_A]\n+++ [FILE_B]\n+import java.util.List;"),
			patch2:   []byte("--- [FILE_A]\n+++ [FILE_B]\n+import java.util.Set;"),
			expectGT: 0.5, // Should have reasonable similarity due to structure
		},
		{
			name:     "completely different patches",
			patch1:   []byte("--- [FILE_A]\n+++ [FILE_B]\n+public class A {}"),
			patch2:   []byte("--- [FILE_A]\n+++ [FILE_B]\n-private int x;"),
			expectGT: 0.0, // Should be low but not zero due to structure similarity
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := generator.ComputePatchSimilarity(tt.patch1, tt.patch2)

			if tt.expectSim > 0 {
				if similarity != tt.expectSim {
					t.Errorf("ComputePatchSimilarity() = %v, want %v", similarity, tt.expectSim)
				}
			} else {
				if similarity <= tt.expectGT {
					t.Errorf("ComputePatchSimilarity() = %v, want > %v", similarity, tt.expectGT)
				}
			}
		})
	}
}

func TestDefaultEnhancedSignatureGenerator_FindSimilarSignatures(t *testing.T) {
	generator := NewEnhancedSignatureGenerator(nil)

	targetSig := "abcd123456789000"
	candidateSigs := []string{
		"abcd123456789111", // High similarity
		"abcd000000000000", // Medium similarity
		"0000000000000000", // Low similarity
		"abcd123456789000", // Exact match (should be excluded)
		"ffffffffffffffff", // No similarity
	}

	results := generator.FindSimilarSignatures(targetSig, candidateSigs, "java", "javac", 0.5)

	// Should find signatures above threshold (0.5) but exclude exact match
	if len(results) == 0 {
		t.Error("Expected to find similar signatures, got none")
	}

	// Results should be sorted by similarity descending
	for i := 1; i < len(results); i++ {
		if results[i-1].Similarity < results[i].Similarity {
			t.Errorf("Results not sorted properly: %v < %v", results[i-1].Similarity, results[i].Similarity)
		}
	}

	// Should not include exact match
	for _, result := range results {
		if result.Signature == targetSig {
			t.Error("Results should not include exact match of target signature")
		}
	}
}

func TestDefaultEnhancedSignatureGenerator_FindSimilarPatches(t *testing.T) {
	generator := NewEnhancedSignatureGenerator(nil)

	targetFingerprint := "target123"
	patch1 := []byte("--- [FILE_A]\n+++ [FILE_B]\n+import java.util.List;")
	patch2 := []byte("--- [FILE_A]\n+++ [FILE_B]\n+import java.util.Set;")
	patch3 := []byte("--- [FILE_A]\n+++ [FILE_B]\n+public class Test {}")

	patches := map[string][]byte{
		"target123": patch1,
		"similar1":  patch2, // Similar import
		"different": patch3, // Different content
	}

	candidateFingerprints := []string{"similar1", "different"}

	results := generator.FindSimilarPatches(targetFingerprint, candidateFingerprints, patches, 0.3)

	if len(results) == 0 {
		t.Error("Expected to find similar patches, got none")
	}

	// Results should be sorted by similarity descending
	for i := 1; i < len(results); i++ {
		if results[i-1].Similarity < results[i].Similarity {
			t.Errorf("Results not sorted properly: %v < %v", results[i-1].Similarity, results[i].Similarity)
		}
	}

	// Should include patch size information
	for _, result := range results {
		if result.PatchSize <= 0 {
			t.Errorf("Expected positive patch size, got %v", result.PatchSize)
		}
	}
}

func TestDeduplicationConfig_Defaults(t *testing.T) {
	config := DefaultDeduplicationConfig()

	if config.ErrorSimilarityThreshold != 0.8 {
		t.Errorf("Expected ErrorSimilarityThreshold 0.8, got %v", config.ErrorSimilarityThreshold)
	}

	if config.PatchSimilarityThreshold != 0.85 {
		t.Errorf("Expected PatchSimilarityThreshold 0.85, got %v", config.PatchSimilarityThreshold)
	}

	// Verify weights sum close to 1.0
	totalWeight := config.LexicalSimilarityWeight + config.StructuralSimilarityWeight + config.SemanticSimilarityWeight
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("Expected similarity weights to sum to ~1.0, got %v", totalWeight)
	}
}

func TestDefaultEnhancedSignatureGenerator_TokenExtraction(t *testing.T) {
	generator := NewEnhancedSignatureGenerator(nil)

	patch := []byte(`--- [FILE_A]
+++ [FILE_B]  
+import java.util.List;
+public class Test {
-    private int oldVar;
+    private String newVar;
 }`)

	tokens := generator.extractChangeTokens(patch)

	expectedTokens := []string{"import", "java", "util", "List", "public", "class", "Test", "private", "int", "oldVar", "private", "String", "newVar"}

	// Check that expected tokens are present
	tokenMap := make(map[string]bool)
	for _, token := range tokens {
		tokenMap[token] = true
	}

	for _, expected := range expectedTokens {
		if !tokenMap[expected] {
			t.Errorf("Expected token %q not found in extracted tokens: %v", expected, tokens)
		}
	}
}

func TestDefaultEnhancedSignatureGenerator_PatchStructureAnalysis(t *testing.T) {
	generator := NewEnhancedSignatureGenerator(nil)

	lines := []string{
		"--- [FILE_A]",
		"+++ [FILE_B]",
		" context line 1",
		"+added line 1",
		"+added line 2",
		"-removed line 1",
		" context line 2",
	}

	structure := generator.analyzePatchStructure(lines)

	if structure.additions != 2 {
		t.Errorf("Expected 2 additions, got %d", structure.additions)
	}

	if structure.deletions != 1 {
		t.Errorf("Expected 1 deletion, got %d", structure.deletions)
	}

	if structure.context != 2 {
		t.Errorf("Expected 2 context lines, got %d", structure.context)
	}
}
