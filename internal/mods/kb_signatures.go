package transflow

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SignatureGenerator provides error signature normalization and patch fingerprinting
type SignatureGenerator interface {
	GenerateSignature(lang, compiler string, stdout, stderr []byte) string
	NormalizePatch(patch []byte) ([]byte, string) // returns normalized patch and fingerprint
}

// EnhancedSignatureGenerator extends SignatureGenerator with similarity detection capabilities
type EnhancedSignatureGenerator interface {
	SignatureGenerator

	// Similarity detection for error signatures
	ComputeSignatureSimilarity(sig1, sig2 string, lang, compiler string) float64
	FindSimilarSignatures(targetSig string, candidateSigs []string, lang, compiler string, threshold float64) []SimilarSignature

	// Enhanced patch similarity
	ComputePatchSimilarity(patch1, patch2 []byte) float64
	FindSimilarPatches(targetFingerprint string, candidateFingerprints []string, patches map[string][]byte, threshold float64) []SimilarPatch
}

// SimilarSignature represents a signature with similarity score
type SimilarSignature struct {
	Signature  string  `json:"signature"`
	Similarity float64 `json:"similarity"`
	Language   string  `json:"language"`
	Compiler   string  `json:"compiler"`
}

// SimilarPatch represents a patch with similarity score
type SimilarPatch struct {
	Fingerprint string  `json:"fingerprint"`
	Similarity  float64 `json:"similarity"`
	PatchSize   int     `json:"patch_size"`
}

// DeduplicationConfig contains configuration for deduplication algorithms
type DeduplicationConfig struct {
	// Error signature similarity thresholds
	ErrorSimilarityThreshold   float64 `json:"error_similarity_threshold"`   // 0.8
	PatchSimilarityThreshold   float64 `json:"patch_similarity_threshold"`   // 0.85
	ContextSimilarityThreshold float64 `json:"context_similarity_threshold"` // 0.9

	// Similarity algorithm weights
	LexicalSimilarityWeight    float64 `json:"lexical_similarity_weight"`    // 0.4
	StructuralSimilarityWeight float64 `json:"structural_similarity_weight"` // 0.3
	SemanticSimilarityWeight   float64 `json:"semantic_similarity_weight"`   // 0.3

	// Performance limits
	MaxCandidatesForSimilarity int `json:"max_candidates_for_similarity"` // 1000
	MaxSimilarResults          int `json:"max_similar_results"`           // 50
}

// DefaultSignatureGenerator implements SignatureGenerator with content-addressed normalization
type DefaultSignatureGenerator struct {
	// Regex patterns for normalizing error output
	timestampPattern  *regexp.Regexp
	pathPattern       *regexp.Regexp
	lineNumberPattern *regexp.Regexp
	memoryAddrPattern *regexp.Regexp
	tempFilePattern   *regexp.Regexp
	userHomePattern   *regexp.Regexp
	buildIdPattern    *regexp.Regexp
	threadIdPattern   *regexp.Regexp
}

// NewDefaultSignatureGenerator creates a new signature generator with default patterns
func NewDefaultSignatureGenerator() *DefaultSignatureGenerator {
	return &DefaultSignatureGenerator{
		// Match various timestamp formats
		timestampPattern: regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?([+-]\d{4}|Z)?\b|\b\d{2}:\d{2}:\d{2}\b`),

		// Match absolute paths and Windows drive letters
		pathPattern: regexp.MustCompile(`(?:/[^\s]+/|[A-Z]:\\[^\s\\]+\\|\.{1,2}/[^\s]*)`),

		// Match line numbers and column numbers
		lineNumberPattern: regexp.MustCompile(`:\d+:\d+\b|:\d+\b|\bline \d+\b|\bat line \d+\b`),

		// Match memory addresses
		memoryAddrPattern: regexp.MustCompile(`0x[a-fA-F0-9]+\b`),

		// Match temporary file patterns
		tempFilePattern: regexp.MustCompile(`/tmp/[^\s]+|C:\\Users\\[^\\]+\\AppData\\Local\\Temp\\[^\s]+|\.tmp\.\w+`),

		// Match user home directories
		userHomePattern: regexp.MustCompile(`/home/[^/\s]+/|/Users/[^/\s]+/|C:\\Users\\[^\\]+\\`),

		// Match build IDs, job IDs, and other generated identifiers
		buildIdPattern: regexp.MustCompile(`\b[a-fA-F0-9]{8,}\b|\bbuild-\d+\b|\bjob-\w+\b`),

		// Match thread/process IDs
		threadIdPattern: regexp.MustCompile(`\b(thread|process|pid)\s+\d+\b|\[\d+\]`),
	}
}

// GenerateSignature creates a normalized signature for an error
func (sg *DefaultSignatureGenerator) GenerateSignature(lang, compiler string, stdout, stderr []byte) string {
	// Combine stdout and stderr for comprehensive error analysis
	combined := string(stdout) + "\n" + string(stderr)

	// Normalize the error text
	normalized := sg.normalizeErrorText(combined)

	// Create signature components
	components := []string{
		"lang=" + lang,
		"compiler=" + compiler,
		"error=" + normalized,
	}

	// Hash the combined components for a stable signature
	content := strings.Join(components, "|")
	hash := sha256.Sum256([]byte(content))

	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes for compact signature
}

// normalizeErrorText removes environment-specific details from error text
func (sg *DefaultSignatureGenerator) normalizeErrorText(text string) string {
	// Start with the original text
	normalized := text

	// Remove timestamps
	normalized = sg.timestampPattern.ReplaceAllString(normalized, "[TIMESTAMP]")

	// Normalize paths to generic placeholders
	normalized = sg.pathPattern.ReplaceAllString(normalized, "[PATH]")

	// Remove specific line numbers (keep pattern but normalize numbers)
	normalized = sg.lineNumberPattern.ReplaceAllString(normalized, ":[LINE]")

	// Remove memory addresses
	normalized = sg.memoryAddrPattern.ReplaceAllString(normalized, "[ADDR]")

	// Remove temporary files
	normalized = sg.tempFilePattern.ReplaceAllString(normalized, "[TMPFILE]")

	// Remove user-specific home directory paths
	normalized = sg.userHomePattern.ReplaceAllString(normalized, "[HOME]/")

	// Remove build-specific IDs
	normalized = sg.buildIdPattern.ReplaceAllString(normalized, "[BUILD_ID]")

	// Remove thread/process IDs
	normalized = sg.threadIdPattern.ReplaceAllString(normalized, "[THREAD_ID]")

	// Clean up whitespace and collapse multiple spaces
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, " ")

	// Trim leading/trailing whitespace
	normalized = strings.TrimSpace(normalized)

	// Extract key error patterns (compiler-specific)
	normalized = sg.extractKeyErrorPatterns(normalized)

	return normalized
}

// extractKeyErrorPatterns identifies and prioritizes important error information
func (sg *DefaultSignatureGenerator) extractKeyErrorPatterns(text string) string {
	lines := strings.Split(text, "\n")
	var keyLines []string

	// Common error indicators across languages
	errorIndicators := []string{
		"error:", "Error:", "ERROR:",
		"fatal:", "Fatal:", "FATAL:",
		"exception:", "Exception:", "EXCEPTION:",
		"panic:", "Panic:", "PANIC:",
		"failed:", "Failed:", "FAILED:",
		"cannot", "Cannot", "CANNOT",
		"undefined", "Undefined", "UNDEFINED",
		"not found", "Not found", "NOT FOUND",
		"syntax error", "Syntax error", "SYNTAX ERROR",
		"compilation error", "Compilation error",
		"build error", "Build error",
	}

	// Collect lines containing error indicators
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		for _, indicator := range errorIndicators {
			if strings.Contains(line, indicator) {
				keyLines = append(keyLines, line)
				break
			}
		}
	}

	// If no key lines found, use first few non-empty lines
	if len(keyLines) == 0 {
		maxLines := 3
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				keyLines = append(keyLines, line)
				if len(keyLines) >= maxLines {
					break
				}
			}
		}
	}

	// Sort for consistency (order shouldn't matter for same logical error)
	sort.Strings(keyLines)

	return strings.Join(keyLines, "\n")
}

// NormalizePatch normalizes a patch by removing timestamps and file-specific metadata
func (sg *DefaultSignatureGenerator) NormalizePatch(patch []byte) ([]byte, string) {
	patchText := string(patch)

	// Normalize patch headers (remove timestamps)
	lines := strings.Split(patchText, "\n")
	var normalizedLines []string

	for _, line := range lines {
		// Skip diff headers with timestamps
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			// Keep the file path structure but normalize it
			if strings.HasPrefix(line, "---") {
				normalizedLines = append(normalizedLines, "--- [FILE_A]")
			} else {
				normalizedLines = append(normalizedLines, "+++ [FILE_B]")
			}
			continue
		}

		// Skip index lines (git-specific)
		if strings.HasPrefix(line, "index ") {
			continue
		}

		// Skip diff command lines
		if strings.HasPrefix(line, "diff --git") {
			continue
		}

		// Keep actual diff content
		normalizedLines = append(normalizedLines, line)
	}

	normalizedPatch := strings.Join(normalizedLines, "\n")

	// Remove trailing whitespace variations
	normalizedPatch = strings.TrimSpace(normalizedPatch)

	// Normalize whitespace within lines (but preserve leading spaces for diff context)
	lines = strings.Split(normalizedPatch, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			// Preserve diff markers and leading space, normalize trailing
			lines[i] = strings.TrimRight(line, " \t")
		}
	}
	normalizedPatch = strings.Join(lines, "\n")

	// Generate fingerprint from normalized content
	fingerprint := sg.generatePatchFingerprint([]byte(normalizedPatch))

	return []byte(normalizedPatch), fingerprint
}

// generatePatchFingerprint creates a content-addressed fingerprint for a patch
func (sg *DefaultSignatureGenerator) generatePatchFingerprint(normalizedPatch []byte) string {
	hash := sha256.Sum256(normalizedPatch)
	return fmt.Sprintf("%x", hash)
}

// SanitizeForStorage removes sensitive information from text for safe storage
func SanitizeForStorage(text string) string {
	sanitizer := NewLogSanitizer()
	return sanitizer.Sanitize(text)
}

// LogSanitizer removes sensitive information from logs and build output
type LogSanitizer struct {
	tokenPattern    *regexp.Regexp
	keyPattern      *regexp.Regexp
	passwordPattern *regexp.Regexp
	urlAuthPattern  *regexp.Regexp
	jwtPattern      *regexp.Regexp
	maxLength       int
}

// NewLogSanitizer creates a new log sanitizer
func NewLogSanitizer() *LogSanitizer {
	return &LogSanitizer{
		// GitHub/GitLab tokens (ghp_, glpat_, etc.)
		tokenPattern: regexp.MustCompile(`(?i)\b(gh[ps]_[a-zA-Z0-9]{36}|glpat-[a-zA-Z0-9_\-]{20}|gho_[a-zA-Z0-9]{36}|ghu_[a-zA-Z0-9]{36})\b`),

		// API keys and secrets (various patterns)
		keyPattern: regexp.MustCompile(`(?i)\b(api[_-]?key|secret[_-]?key|access[_-]?token|bearer[_-]?token)["\s]*[:=]["\s]*[a-zA-Z0-9+/=]{16,}\b`),

		// Passwords in command lines
		passwordPattern: regexp.MustCompile(`(?i)(password|passwd|pwd)["\s]*[:=]["\s]*[^\s"']+`),

		// URLs with embedded authentication
		urlAuthPattern: regexp.MustCompile(`https?://[^@\s]+:[^@\s]+@[^\s]+`),

		// JWT tokens
		jwtPattern: regexp.MustCompile(`\beyJ[a-zA-Z0-9+/=]+\.[a-zA-Z0-9+/=]+\.[a-zA-Z0-9+/=_-]+\b`),

		// Reasonable limit for log storage
		maxLength: 50000, // 50KB max per log
	}
}

// Sanitize removes sensitive information from logs
func (s *LogSanitizer) Sanitize(text string) string {
	// Remove tokens
	sanitized := s.tokenPattern.ReplaceAllString(text, "[REDACTED_TOKEN]")

	// Remove API keys
	sanitized = s.keyPattern.ReplaceAllString(sanitized, "[REDACTED_KEY]")

	// Remove passwords
	sanitized = s.passwordPattern.ReplaceAllString(sanitized, "[REDACTED_PASSWORD]")

	// Remove URLs with auth
	sanitized = s.urlAuthPattern.ReplaceAllString(sanitized, "[REDACTED_AUTH_URL]")

	// Remove JWT tokens
	sanitized = s.jwtPattern.ReplaceAllString(sanitized, "[REDACTED_JWT]")

	// Truncate if too long
	if len(sanitized) > s.maxLength {
		sanitized = sanitized[:s.maxLength-20] + "\n...[TRUNCATED]"
	}

	return sanitized
}

// CreateSanitizedLogs creates sanitized log records for KB storage
func CreateSanitizedLogs(stdout, stderr string) *SanitizedLogs {
	sanitizer := NewLogSanitizer()

	sanitizedStdout := sanitizer.Sanitize(stdout)
	sanitizedStderr := sanitizer.Sanitize(stderr)

	return &SanitizedLogs{
		Stdout:    sanitizedStdout,
		Stderr:    sanitizedStderr,
		Truncated: len(sanitizedStdout) < len(stdout) || len(sanitizedStderr) < len(stderr),
		MaxLength: sanitizer.maxLength,
	}
}

// ValidateSignature checks if a signature appears valid
func ValidateSignature(signature string) bool {
	// Signature should be a hex string of 16 characters (8 bytes)
	if len(signature) != 16 {
		return false
	}

	// Check if all characters are valid hex
	for _, char := range signature {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}

	return true
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

	// Reconstruct normalized error text from stored signatures for comparison
	// Since signatures are hashes, we can't reverse them, so this would need to work
	// with the original error text. For now, we'll implement based on signature patterns.

	// Simple Hamming distance for hex signatures as a baseline
	return esg.computeHammingSimilarity(sig1, sig2)
}

// computeHammingSimilarity computes similarity based on Hamming distance of hex signatures
func (esg *DefaultEnhancedSignatureGenerator) computeHammingSimilarity(sig1, sig2 string) float64 {
	if len(sig1) != len(sig2) {
		return 0.0
	}

	matches := 0
	for i := 0; i < len(sig1); i++ {
		if sig1[i] == sig2[i] {
			matches++
		}
	}

	return float64(matches) / float64(len(sig1))
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
			results = append(results, SimilarSignature{
				Signature:  candidateSig,
				Similarity: similarity,
				Language:   lang,
				Compiler:   compiler,
			})
		}
	}

	// Sort by similarity descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	return results
}

// ComputePatchSimilarity calculates similarity between two patches
func (esg *DefaultEnhancedSignatureGenerator) ComputePatchSimilarity(patch1, patch2 []byte) float64 {
	if bytes.Equal(patch1, patch2) {
		return 1.0
	}

	// Normalize both patches first
	norm1, _ := esg.NormalizePatch(patch1)
	norm2, _ := esg.NormalizePatch(patch2)

	if bytes.Equal(norm1, norm2) {
		return 1.0
	}

	// Compute similarity based on multiple factors
	lexicalSim := esg.computeLexicalSimilarity(norm1, norm2)
	structuralSim := esg.computeStructuralSimilarity(norm1, norm2)
	semanticSim := esg.computeSemanticSimilarity(norm1, norm2)

	// Weighted combination
	totalSim := esg.config.LexicalSimilarityWeight*lexicalSim +
		esg.config.StructuralSimilarityWeight*structuralSim +
		esg.config.SemanticSimilarityWeight*semanticSim

	return totalSim
}

// computeLexicalSimilarity uses Levenshtein-based similarity
func (esg *DefaultEnhancedSignatureGenerator) computeLexicalSimilarity(patch1, patch2 []byte) float64 {
	s1, s2 := string(patch1), string(patch2)

	// Simple implementation using longest common subsequence ratio
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

// computeStructuralSimilarity analyzes diff structure patterns
func (esg *DefaultEnhancedSignatureGenerator) computeStructuralSimilarity(patch1, patch2 []byte) float64 {
	lines1 := strings.Split(string(patch1), "\n")
	lines2 := strings.Split(string(patch2), "\n")

	// Count additions, deletions, and context lines
	struct1 := esg.analyzePatchStructure(lines1)
	struct2 := esg.analyzePatchStructure(lines2)

	// Compare structural patterns
	addSim := 1.0 - float64(abs(struct1.additions-struct2.additions))/float64(max(struct1.additions, struct2.additions, 1))
	delSim := 1.0 - float64(abs(struct1.deletions-struct2.deletions))/float64(max(struct1.deletions, struct2.deletions, 1))
	ctxSim := 1.0 - float64(abs(struct1.context-struct2.context))/float64(max(struct1.context, struct2.context, 1))

	return (addSim + delSim + ctxSim) / 3.0
}

// computeSemanticSimilarity analyzes the semantic content of changes
func (esg *DefaultEnhancedSignatureGenerator) computeSemanticSimilarity(patch1, patch2 []byte) float64 {
	// Extract added/modified code tokens from patches
	tokens1 := esg.extractChangeTokens(patch1)
	tokens2 := esg.extractChangeTokens(patch2)

	// Compute token overlap
	return esg.computeTokenOverlap(tokens1, tokens2)
}

// patchStructure represents the structure of a patch
type patchStructure struct {
	additions int
	deletions int
	context   int
}

// analyzePatchStructure extracts structural information from patch lines
func (esg *DefaultEnhancedSignatureGenerator) analyzePatchStructure(lines []string) patchStructure {
	var struct_ patchStructure

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		switch line[0] {
		case '+':
			struct_.additions++
		case '-':
			struct_.deletions++
		case ' ':
			struct_.context++
		}
	}

	return struct_
}

// extractChangeTokens extracts meaningful tokens from patch changes
func (esg *DefaultEnhancedSignatureGenerator) extractChangeTokens(patch []byte) []string {
	lines := strings.Split(string(patch), "\n")
	var tokens []string

	// Simple tokenization of added/removed lines
	tokenPattern := regexp.MustCompile(`\b\w+\b`)

	for _, line := range lines {
		if len(line) > 0 && (line[0] == '+' || line[0] == '-') {
			content := line[1:] // Remove +/- prefix
			lineTokens := tokenPattern.FindAllString(content, -1)
			tokens = append(tokens, lineTokens...)
		}
	}

	return tokens
}

// computeTokenOverlap computes overlap between two token sets
func (esg *DefaultEnhancedSignatureGenerator) computeTokenOverlap(tokens1, tokens2 []string) float64 {
	if len(tokens1) == 0 && len(tokens2) == 0 {
		return 1.0
	}

	set1 := make(map[string]bool)
	for _, token := range tokens1 {
		set1[token] = true
	}

	set2 := make(map[string]bool)
	for _, token := range tokens2 {
		set2[token] = true
	}

	// Compute Jaccard similarity
	intersection := 0
	for token := range set1 {
		if set2[token] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 1.0
	}

	return float64(intersection) / float64(union)
}

// FindSimilarPatches finds patches similar to the target
func (esg *DefaultEnhancedSignatureGenerator) FindSimilarPatches(targetFingerprint string, candidateFingerprints []string, patches map[string][]byte, threshold float64) []SimilarPatch {
	targetPatch, exists := patches[targetFingerprint]
	if !exists {
		return nil
	}

	var results []SimilarPatch

	for _, candidateFingerprint := range candidateFingerprints {
		if len(results) >= esg.config.MaxSimilarResults {
			break
		}

		if candidateFingerprint == targetFingerprint {
			continue
		}

		candidatePatch, exists := patches[candidateFingerprint]
		if !exists {
			continue
		}

		similarity := esg.ComputePatchSimilarity(targetPatch, candidatePatch)
		if similarity >= threshold {
			results = append(results, SimilarPatch{
				Fingerprint: candidateFingerprint,
				Similarity:  similarity,
				PatchSize:   len(candidatePatch),
			})
		}
	}

	// Sort by similarity descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	return results
}

// Helper functions

func (esg *DefaultEnhancedSignatureGenerator) longestCommonSubsequence(s1, s2 string) int {
	m, n := len(s1), len(s2)
	if m == 0 || n == 0 {
		return 0
	}

	// Dynamic programming approach
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if s1[i-1] == s2[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
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
	result := a
	if b > result {
		result = b
	}
	if c > result {
		result = c
	}
	return result
}
