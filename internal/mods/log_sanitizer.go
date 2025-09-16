package mods

import "regexp"

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
		tokenPattern:    regexp.MustCompile(`(?i)\b(gh[ps]_[a-zA-Z0-9]{36}|glpat-[a-zA-Z0-9_\-]{20}|gho_[a-zA-Z0-9]{36}|ghu_[a-zA-Z0-9]{36})\b`),
		keyPattern:      regexp.MustCompile(`(?i)\b(api[_-]?key|secret[_-]?key|access[_-]?token|bearer[_-]?token)["\s]*[:=]["\s]*[a-zA-Z0-9+/=]{16,}\b`),
		passwordPattern: regexp.MustCompile(`(?i)(password|passwd|pwd)["\s]*[:=]["\s]*[^\s"']+`),
		urlAuthPattern:  regexp.MustCompile(`https?://[^@\s]+:[^@\s]+@[^\s]+`),
		jwtPattern:      regexp.MustCompile(`\beyJ[a-zA-Z0-9+/=]+\.[a-zA-Z0-9+/=]+\.[a-zA-Z0-9+/=_-]+\b`),
		maxLength:       50000,
	}
}

// Sanitize removes sensitive information from logs
func (s *LogSanitizer) Sanitize(text string) string {
	sanitized := s.tokenPattern.ReplaceAllString(text, "[REDACTED_TOKEN]")
	sanitized = s.keyPattern.ReplaceAllString(sanitized, "[REDACTED_KEY]")
	sanitized = s.passwordPattern.ReplaceAllString(sanitized, "[REDACTED_PASSWORD]")
	sanitized = s.urlAuthPattern.ReplaceAllString(sanitized, "[REDACTED_AUTH_URL]")
	sanitized = s.jwtPattern.ReplaceAllString(sanitized, "[REDACTED_JWT]")
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
