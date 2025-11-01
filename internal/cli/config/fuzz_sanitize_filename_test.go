package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// FuzzSanitizeFilename ensures sanitizeFilename never returns path separators
// and remains stable under repeated application.
func FuzzSanitizeFilename(f *testing.F) {
	seeds := []string{
		"simple",
		" a/b ",
		"..\\..",
		"name with spaces",
		"中文/漢字\\mix",
		"tricky/../../etc/passwd",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := sanitizeFilename(in)
		if strings.Contains(out, string(filepath.Separator)) || strings.Contains(out, "/") || strings.Contains(out, "\\") {
			t.Fatalf("sanitizeFilename returned path separator: %q -> %q", in, out)
		}
		// Idempotent application property.
		if again := sanitizeFilename(out); again != out {
			t.Fatalf("sanitizeFilename not idempotent: %q -> %q -> %q", in, out, again)
		}
		// Trimmed property.
		if strings.TrimSpace(out) != out {
			t.Fatalf("sanitizeFilename should trim spaces: %q", out)
		}
	})
}
