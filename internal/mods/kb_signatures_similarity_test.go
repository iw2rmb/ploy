package mods

import "testing"

func TestEnhancedSignature_SignatureSimilarity(t *testing.T) {
	esg := NewEnhancedSignatureGenerator(nil)

	// Identical strings -> 1.0
	if s := esg.ComputeSignatureSimilarity("abcdef12", "abcdef12", "java", "javac"); s != 1.0 {
		t.Fatalf("expected 1.0, got %v", s)
	}

	// Same length, small difference -> between 0 and 1
	s := esg.ComputeSignatureSimilarity("abcdef12", "abcxef12", "java", "javac")
	if !(s > 0 && s < 1) {
		t.Fatalf("expected 0<s<1, got %v", s)
	}

	// Different length -> 0.0
	if s := esg.ComputeSignatureSimilarity("abcd", "abcdef", "java", "javac"); s != 0.0 {
		t.Fatalf("expected 0.0, got %v", s)
	}
}

func TestEnhancedSignature_PatchSimilarityNormalization(t *testing.T) {
	esg := NewEnhancedSignatureGenerator(nil)

	// Two patches that differ only by timestamps and headers should normalize equal
	p1 := []byte(`--- Test.java\t2023-12-01 10:30:45\n+++ Test.java\t2023-12-01 10:31:12\n@@ -1,2 +1,2 @@\n-old\n+new`)
	p2 := []byte(`--- Test.java\t2024-01-02 01:02:03\n+++ Test.java\t2024-01-02 01:03:04\n@@ -1,2 +1,2 @@\n-old\n+new`)

	sim := esg.ComputePatchSimilarity(p1, p2)
	if sim != 1.0 {
		t.Fatalf("expected 1.0 similarity for normalized-equal patches, got %v", sim)
	}
}

func TestEnhancedSignature_FindSimilarSignatures(t *testing.T) {
	esg := NewEnhancedSignatureGenerator(nil)
	target := "abcdef12"
	candidates := []string{"zzzzzzzz", "abcxef12", "abcdef12"}

	// Threshold high enough to include very similar but not identical; identical is excluded by function
	res := esg.FindSimilarSignatures(target, candidates, "go", "go", 0.85)
	if len(res) != 1 || res[0].Signature != "abcxef12" {
		t.Fatalf("expected one similar result 'abcxef12', got %#v", res)
	}
}
