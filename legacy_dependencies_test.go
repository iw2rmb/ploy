package ploy_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoLegacyInfraImports(t *testing.T) {
	forbidden := []string{
		"\"github.com/hashicorp/nomad",
		"\"github.com/hashicorp/consul",
		"\"github.com/seaweedfs/seaweedfs",
		"\"github.com/chrislusf/seaweedfs",
		"\"github.com/iw2rmb/ploy/api/",
		"\"github.com/iw2rmb/ploy/internal/orchestration",
		"\"github.com/iw2rmb/ploy/internal/storage/providers/seaweedfs",
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || strings.HasPrefix(name, ".") || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		for _, disallowed := range forbidden {
			if strings.Contains(content, disallowed) {
				offenders = append(offenders, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}

	if len(offenders) > 0 {
		t.Fatalf("legacy infrastructure imports must be removed; found in:\n%s", strings.Join(offenders, "\n"))
	}
}
