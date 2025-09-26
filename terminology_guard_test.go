package ploy_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var forbiddenTerm = regexp.MustCompile(`(?i)\barf\b`)

func TestTerminologyDoesNotUseArf(t *testing.T) {
	roots := []string{
		"README.md",
		"docs",
		"roadmap",
		"internal",
		"cmd",
		"configs",
	}

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !isTextFile(path) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if loc := forbiddenTerm.FindIndex(data); loc != nil {
				t.Errorf("forbidden term %q found in %s", "arf", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

func isTextFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".go", ".toml", ".txt", ".yaml", ".yml", ".json":
		return true
	}
	return path == "README.md"
}
