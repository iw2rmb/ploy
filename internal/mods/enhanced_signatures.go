package mods

import (
	"bytes"
	"regexp"
	"sort"
	"strings"
)

// EnhancedSignatureGenerator extends SignatureGenerator with similarity detection capabilities
type EnhancedSignatureGenerator interface {
	SignatureGenerator
	ComputeSignatureSimilarity(sig1, sig2 string, lang, compiler string) float64
	FindSimilarSignatures(targetSig string, candidateSigs []string, lang, compiler string, threshold float64) []SimilarSignature
	ComputePatchSimilarity(patch1, patch2 []byte) float64
	FindSimilarPatches(targetFingerprint string, candidateFingerprints []string, patches map[string][]byte, threshold float64) []SimilarPatch
}

// DeduplicationConfig contains configuration for deduplication algorithms
type DeduplicationConfig struct {
	ErrorSimilarityThreshold   float64 `json:"error_similarity_threshold"`
	PatchSimilarityThreshold   float64 `json:"patch_similarity_threshold"`
	ContextSimilarityThreshold float64 `json:"context_similarity_threshold"`
	LexicalSimilarityWeight    float64 `json:"lexical_similarity_weight"`
	StructuralSimilarityWeight float64 `json:"structural_similarity_weight"`
	SemanticSimilarityWeight   float64 `json:"semantic_similarity_weight"`
	MaxCandidatesForSimilarity int     `json:"max_candidates_for_similarity"`
	MaxSimilarResults          int     `json:"max_similar_results"`
}

// DefaultEnhancedSignatureGenerator implements both basic and enhanced signature generation
type DefaultEnhancedSignatureGenerator struct {
	*DefaultSignatureGenerator
	config *DeduplicationConfig
}

// NewEnhancedSignatureGenerator creates a new enhanced signature generator
func NewEnhancedSignatureGenerator(config *DeduplicationConfig) *DefaultEnhancedSignatureGenerator {
	if config == nil {
		config = DefaultDeduplicationConfig()
	}
	return &DefaultEnhancedSignatureGenerator{
		DefaultSignatureGenerator: NewDefaultSignatureGenerator(),
		config:                    config,
	}
}

// DefaultDeduplicationConfig returns reasonable defaults for deduplication
func DefaultDeduplicationConfig() *DeduplicationConfig {
	return &DeduplicationConfig{
		ErrorSimilarityThreshold:   0.8,
		PatchSimilarityThreshold:   0.85,
		ContextSimilarityThreshold: 0.9,
		LexicalSimilarityWeight:    0.4,
		StructuralSimilarityWeight: 0.3,
		SemanticSimilarityWeight:   0.3,
		MaxCandidatesForSimilarity: 1000,
		MaxSimilarResults:          50,
	}
}

// ComputeSignatureSimilarity calculates similarity between two error signatures
func (esg *DefaultEnhancedSignatureGenerator) ComputeSignatureSimilarity(sig1, sig2 string, lang, compiler string) float64 {
	if sig1 == sig2 {
		return 1.0
	}
	return esg.computeHammingSimilarity(sig1, sig2)
}

func (esg *DefaultEnhancedSignatureGenerator) computeHammingSimilarity(sig1, sig2 string) float64 {
	// Compare up to a fixed length of 16 hex chars to be robust to minor length differences
	const n = 16
	matches := 0
	for i := 0; i < n; i++ {
		if i < len(sig1) && i < len(sig2) && sig1[i] == sig2[i] {
			matches++
		}
	}
	return float64(matches) / float64(n)
}

// FindSimilarSignatures finds signatures similar to the target
func (esg *DefaultEnhancedSignatureGenerator) FindSimilarSignatures(targetSig string, candidateSigs []string, lang, compiler string, threshold float64) []SimilarSignature {
	var results []SimilarSignature
	for _, candidateSig := range candidateSigs {
		if len(results) >= esg.config.MaxSimilarResults {
			break
		}
		similarity := esg.ComputeSignatureSimilarity(targetSig, candidateSig, lang, compiler)
		if similarity >= threshold && candidateSig != targetSig {
			results = append(results, SimilarSignature{Signature: candidateSig, Similarity: similarity, Language: lang, Compiler: compiler})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Similarity > results[j].Similarity })
	return results
}

// ComputePatchSimilarity calculates similarity between two patches
func (esg *DefaultEnhancedSignatureGenerator) ComputePatchSimilarity(patch1, patch2 []byte) float64 {
	if bytes.Equal(patch1, patch2) {
		return 1.0
	}
	norm1, _ := esg.NormalizePatch(patch1)
	norm2, _ := esg.NormalizePatch(patch2)
	if bytes.Equal(norm1, norm2) {
		return 1.0
	}
	lexicalSim := esg.computeLexicalSimilarity(norm1, norm2)
	structuralSim := esg.computeStructuralSimilarity(norm1, norm2)
	semanticSim := esg.computeSemanticSimilarity(norm1, norm2)
	totalSim := esg.config.LexicalSimilarityWeight*lexicalSim + esg.config.StructuralSimilarityWeight*structuralSim + esg.config.SemanticSimilarityWeight*semanticSim
	return totalSim
}

// computeLexicalSimilarity uses LCS ratio as a proxy
func (esg *DefaultEnhancedSignatureGenerator) computeLexicalSimilarity(patch1, patch2 []byte) float64 {
	s1, s2 := string(patch1), string(patch2)
	lcs := esg.longestCommonSubsequence(s1, s2)
	maxLen := len(s1)
	if len(s2) > maxLen {
		maxLen = len(s2)
	}
	if maxLen == 0 {
		return 1.0
	}
	return float64(lcs) / float64(maxLen)
}

func (esg *DefaultEnhancedSignatureGenerator) computeStructuralSimilarity(patch1, patch2 []byte) float64 {
	lines1 := strings.Split(string(patch1), "\n")
	lines2 := strings.Split(string(patch2), "\n")
	struct1 := esg.analyzePatchStructure(lines1)
	struct2 := esg.analyzePatchStructure(lines2)
	addSim := 1.0 - float64(abs(struct1.additions-struct2.additions))/float64(max(struct1.additions, struct2.additions, 1))
	delSim := 1.0 - float64(abs(struct1.deletions-struct2.deletions))/float64(max(struct1.deletions, struct2.deletions, 1))
	ctxSim := 1.0 - float64(abs(struct1.context-struct2.context))/float64(max(struct1.context, struct2.context, 1))
	return (addSim + delSim + ctxSim) / 3.0
}

func (esg *DefaultEnhancedSignatureGenerator) computeSemanticSimilarity(patch1, patch2 []byte) float64 {
	tokens1 := esg.extractChangeTokens(patch1)
	tokens2 := esg.extractChangeTokens(patch2)
	return esg.computeTokenOverlap(tokens1, tokens2)
}

// FindSimilarPatches finds patches similar to the target fingerprint
func (esg *DefaultEnhancedSignatureGenerator) FindSimilarPatches(targetFingerprint string, candidateFingerprints []string, patches map[string][]byte, threshold float64) []SimilarPatch {
	targetPatch, exists := patches[targetFingerprint]
	if !exists {
		return nil
	}
	var results []SimilarPatch
	for _, candidate := range candidateFingerprints {
		if len(results) >= esg.config.MaxSimilarResults {
			break
		}
		if candidate == targetFingerprint {
			continue
		}
		candPatch, ok := patches[candidate]
		if !ok {
			continue
		}
		sim := esg.ComputePatchSimilarity(targetPatch, candPatch)
		if sim >= threshold {
			results = append(results, SimilarPatch{Fingerprint: candidate, Similarity: sim, PatchSize: len(candPatch)})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Similarity > results[j].Similarity })
	return results
}

type patchStructure struct{ additions, deletions, context int }

func (esg *DefaultEnhancedSignatureGenerator) analyzePatchStructure(lines []string) patchStructure {
	var s patchStructure
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+':
			s.additions++
		case '-':
			s.deletions++
		case ' ':
			s.context++
		}
	}
	return s
}

func (esg *DefaultEnhancedSignatureGenerator) extractChangeTokens(patch []byte) []string {
	lines := strings.Split(string(patch), "\n")
	var tokens []string
	tokenPattern := regexp.MustCompile(`\b\w+\b`)
	for _, line := range lines {
		if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
			content := line[1:]
			lineTokens := tokenPattern.FindAllString(content, -1)
			tokens = append(tokens, lineTokens...)
		}
	}
	return tokens
}

func (esg *DefaultEnhancedSignatureGenerator) computeTokenOverlap(tokens1, tokens2 []string) float64 {
	if len(tokens1) == 0 && len(tokens2) == 0 {
		return 1.0
	}
	set1 := make(map[string]bool)
	for _, t := range tokens1 {
		set1[t] = true
	}
	set2 := make(map[string]bool)
	for _, t := range tokens2 {
		set2[t] = true
	}
	intersection := 0
	for t := range set1 {
		if set2[t] {
			intersection++
		}
	}
	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func (esg *DefaultEnhancedSignatureGenerator) longestCommonSubsequence(s1, s2 string) int {
	m, n := len(s1), len(s2)
	if m == 0 || n == 0 {
		return 0
	}
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if s1[i-1] == s2[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] > dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	return dp[m][n]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func max(a, b, c int) int {
	r := a
	if b > r {
		r = b
	}
	if c > r {
		r = c
	}
	return r
}
