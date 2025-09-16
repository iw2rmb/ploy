package mods

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// buildFirstErrorSnippet extracts a small 1-3 line snippet around the first compiler error
// from buildError and returns a formatted message for event emission.
func buildFirstErrorSnippet(repoPath, buildError string) string {
	errs := ParseBuildErrors("java", "maven", buildError)
	if len(errs) == 0 {
		return ""
	}
	file := errs[0].File
	line := errs[0].Line
	// Normalize to repo-relative path when possible
	rel := file
	if repoPath != "" {
		prefix := strings.TrimRight(repoPath, string(os.PathSeparator)) + string(os.PathSeparator)
		if strings.HasPrefix(file, prefix) {
			rel = strings.TrimPrefix(file, prefix)
		}
	}
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoPath, rel)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	// Split into lines (support both \n and \r\n)
	content := string(b)
	// Avoid huge messages: cap to first 2000 bytes before splitting to keep memory small
	if len(content) > 20000 {
		content = content[:20000]
	}
	lines := strings.Split(content, "\n")
	if line <= 0 {
		line = 1
	}
	start := line - 1
	if start < 1 {
		start = 1
	}
	end := line + 1
	if end > len(lines) {
		end = len(lines)
	}
	// Extract and trim snippet
	var parts []string
	for i := start; i <= end && i <= len(lines); i++ {
		idx := i
		if idx <= 0 || idx > len(lines) { // guard
			continue
		}
		parts = append(parts, lines[idx-1])
		if len(parts) >= 3 { // at most 3 lines
			break
		}
	}
	snippet := strings.Join(parts, "\n")
	// Truncate snippet for safety
	if len(snippet) > 500 {
		snippet = snippet[:500] + "…"
	}
	return fmt.Sprintf("post-replay snippet file=%s lines %d-%d:\n%s", rel, start, end, snippet)
}
