package guards

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoLegacyModNamingInKeySurfaces enforces the phase-0 contract for the
// mod->mig migration by blocking legacy naming in key repository surfaces.
func TestNoLegacyModNamingInKeySurfaces(t *testing.T) {
	tokens := []string{
		"/v1/" + "mods",
		"internal/" + "mods",
		"ploy " + "mod",
		"deploy/images/" + "mods",
		"mods" + "-",
	}

	roots := []string{"cmd", "internal", "deploy", "docs", "tests"}

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
			if base == "go.mod" || base == "go.sum" {
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
