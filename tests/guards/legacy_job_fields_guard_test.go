package guards

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoLegacyJobFieldTokens enforces repository-wide removal of legacy job
// field/type tokens after the next_id + job_* migration.
func TestNoLegacyJobFieldTokens(t *testing.T) {
	tokens := []string{
		"step" + "_index",
		"mig" + "_type",
		"mig" + "_image",
		"Mig" + "Type",
		"Mig" + "Image",
	}

	roots := []string{"internal", "cmd", "docs", "tests"}

	for _, root := range roots {
		rootPath := resolveRepoPath(root)
		err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			// Skip binary-like files.
			if bytes.IndexByte(content, 0) >= 0 {
				return nil
			}

			text := string(content)
			for _, token := range tokens {
				if strings.Contains(text, token) {
					rel, relErr := filepath.Rel(resolveRepoPath("."), path)
					if relErr != nil {
						rel = path
					}
					t.Fatalf("legacy token %q found in %s", token, rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

func resolveRepoPath(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return filepath.Join("..", "..", path)
}
