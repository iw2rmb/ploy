package goenv

import (
	"os"
	"path/filepath"
	"regexp"
)

// DetectVersion extracts Go version from go.mod (go 1.22)
func DetectVersion(srcDir string) string {
	b, err := os.ReadFile(filepath.Join(srcDir, "go.mod"))
	if err != nil {
		return ""
	}
	if re := regexp.MustCompile(`\ngo\s+(\d+\.\d+)`); re.Match(b) {
		return re.FindStringSubmatch(string(b))[1]
	}
	return ""
}
