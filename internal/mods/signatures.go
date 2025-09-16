package mods

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SignatureGenerator provides error signature normalization and patch fingerprinting
type SignatureGenerator interface {
	GenerateSignature(lang, compiler string, stdout, stderr []byte) string
	NormalizePatch(patch []byte) ([]byte, string)
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

// DefaultSignatureGenerator implements SignatureGenerator with content-addressed normalization
type DefaultSignatureGenerator struct {
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
		timestampPattern:  regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?([+-]\d{4}|Z)?\b|\b\d{2}:\d{2}:\d{2}\b`),
		pathPattern:       regexp.MustCompile(`(?:/[^\s]+/|[A-Z]:\\[^\s\\]+\\|\.{1,2}/[^\s]*)`),
		lineNumberPattern: regexp.MustCompile(`:\d+:\d+\b|:\d+\b|\bline \d+\b|\bat line \d+\b`),
		memoryAddrPattern: regexp.MustCompile(`0x[a-fA-F0-9]+\b`),
		tempFilePattern:   regexp.MustCompile(`/tmp/[^\s]+|C:\\Users\\[^\\]+\\AppData\\Local\\Temp\\[^\s]+|\.tmp\.\w+`),
		userHomePattern:   regexp.MustCompile(`/home/[^/\s]+/|/Users/[^/\s]+/|C:\\Users\\[^\\]+\\`),
		buildIdPattern:    regexp.MustCompile(`\b[a-fA-F0-9]{8,}\b|\bbuild-\d+\b|\bjob-\w+\b`),
		threadIdPattern:   regexp.MustCompile(`\b(thread|process|pid)\s+\d+\b|\[\d+\]`),
	}
}

// GenerateSignature creates a normalized signature for an error
func (sg *DefaultSignatureGenerator) GenerateSignature(lang, compiler string, stdout, stderr []byte) string {
	combined := string(stdout) + "\n" + string(stderr)
	normalized := sg.normalizeErrorText(combined)
	components := []string{"lang=" + lang, "compiler=" + compiler, "error=" + normalized}
	content := strings.Join(components, "|")
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash[:8])
}

// normalizeErrorText removes environment-specific details from error text
func (sg *DefaultSignatureGenerator) normalizeErrorText(text string) string {
	normalized := text
	normalized = sg.timestampPattern.ReplaceAllString(normalized, "[TIMESTAMP]")
	normalized = sg.pathPattern.ReplaceAllString(normalized, "[PATH]")
	normalized = sg.lineNumberPattern.ReplaceAllString(normalized, ":[LINE]")
	normalized = sg.memoryAddrPattern.ReplaceAllString(normalized, "[ADDR]")
	normalized = sg.tempFilePattern.ReplaceAllString(normalized, "[TMPFILE]")
	normalized = sg.userHomePattern.ReplaceAllString(normalized, "[HOME]/")
	normalized = sg.buildIdPattern.ReplaceAllString(normalized, "[BUILD_ID]")
	normalized = sg.threadIdPattern.ReplaceAllString(normalized, "[THREAD_ID]")
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, " ")
	normalized = strings.TrimSpace(normalized)
	normalized = sg.extractKeyErrorPatterns(normalized)
	return normalized
}

// extractKeyErrorPatterns identifies and prioritizes important error information
func (sg *DefaultSignatureGenerator) extractKeyErrorPatterns(text string) string {
	lines := strings.Split(text, "\n")
	var keyLines []string
	indicators := []string{"error:", "Error:", "ERROR:", "fatal:", "Fatal:", "FATAL:", "exception:", "Exception:", "EXCEPTION:", "panic:", "Panic:", "PANIC:", "failed:", "Failed:", "FAILED:", "cannot", "Cannot", "CANNOT", "undefined", "Undefined", "UNDEFINED", "not found", "Not found", "NOT FOUND", "syntax error", "Syntax error", "SYNTAX ERROR", "compilation error", "Compilation error", "build error", "Build error"}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, ind := range indicators {
			if strings.Contains(line, ind) {
				keyLines = append(keyLines, line)
				break
			}
		}
	}
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
	sort.Strings(keyLines)
	return strings.Join(keyLines, "\n")
}

// ValidateSignature checks if a signature appears valid
func ValidateSignature(signature string) bool {
	if len(signature) != 16 {
		return false
	}
	for _, char := range signature {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
