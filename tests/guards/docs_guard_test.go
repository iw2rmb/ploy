//go:build experiment
// +build experiment

package guards

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestDesignDocHTCoverage parses the experiment doc and ensures every HT tag
// has a corresponding TestHT_<n> test function in the experiment package.
func TestDesignDocHTCoverage(t *testing.T) {
	docPath := filepath.Join("design", "docs", "251029_2059_role_sep_experiment.md")
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read doc: %v", err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "# EXPERIMENT") {
		t.Skip("experiment doc not present or not marked")
	}
	// Extract HT tags like HT-1, HT-2 in the How to test section
	re := regexp.MustCompile(`HT-([0-9]+)`) // simple scan
	matches := re.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		t.Fatalf("no HT tags found in doc")
	}
	wanted := map[string]bool{}
	for _, m := range matches {
		if len(m) >= 2 {
			wanted[m[1]] = false
		}
	}
	// Scan experiment tests for functions named TestHT_<n>_
	testsDir := filepath.Join("tests", "experiments", "role_sep")
	walkErr := filepath.WalkDir(testsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(b)
		for n := range wanted {
			if strings.Contains(src, "func TestHT_"+n+"_") {
				wanted[n] = true
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk tests: %v", walkErr)
	}
	missing := []string{}
	for n, ok := range wanted {
		if !ok {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing HT tests for tags: %s", strings.Join(missing, ", "))
	}
}
