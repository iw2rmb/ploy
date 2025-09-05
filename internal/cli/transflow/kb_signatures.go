package transflow

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
	NormalizePatch(patch []byte) ([]byte, string) // returns normalized patch and fingerprint
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
