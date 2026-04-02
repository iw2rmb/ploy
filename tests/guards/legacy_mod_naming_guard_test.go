package guards

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoLegacyMigNamingInKeySurfaces enforces the phase-0 contract for the
// mig rename by blocking legacy naming in key repository surfaces.
func TestNoLegacyMigNamingInKeySurfaces(t *testing.T) {
	legacyUnit := "m" + "o" + "d"
	legacyPlural := legacyUnit + "s"
	tokens := []string{
		"/v1/" + legacyPlural,
		"internal/" + legacyPlural,
		"ploy " + legacyUnit,
		"images/" + legacyPlural,
		legacyPlural + "-",
	}

	roots := []string{"cmd", "internal", "images", "docs", "tests"}

	for _, root := range roots {
		rootPath := resolveRepoPath(root)
		err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			base := filepath.Base(path)
			if base == "go."+legacyUnit || base == "go.sum" {
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
