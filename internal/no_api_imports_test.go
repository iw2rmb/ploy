package internal_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoInternalImportsAPI ensures no package under internal/ imports any path under api/
func TestNoInternalImportsAPI(t *testing.T) {
	root := "internal"
	forbidden := "\"github.com/iw2rmb/ploy/api/"
	var offenders []string

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip hidden and testdata
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(b), forbidden) {
			offenders = append(offenders, path)
		}
		return nil
	})

	if len(offenders) > 0 {
		t.Fatalf("internal packages must not import api/*; found in:\n%s", strings.Join(offenders, "\n"))
	}
}
