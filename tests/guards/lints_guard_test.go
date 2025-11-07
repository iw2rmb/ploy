package guards

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestForbidHardCodedLabelKeys ensures label keys are not hard-coded as strings.
// Allowed source of truth: internal/domain/types/labels.go
func TestForbidHardCodedLabelKeys(t *testing.T) {
	allowExact := map[string]bool{
		filepath.Join("internal", "domain", "types", "labels.go"): true,
	}

	// scan only Go sources under internal/ and cmd/, excluding *_test.go
	roots := []string{"internal", "cmd"}
	rx := regexp.MustCompile(`"com\.ploy\.[a-z_]+"`)

	var offenders []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if allowExact[path] {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if rx.Match(b) {
				offenders = append(offenders, path)
			}
			return nil
		})
	}
	if len(offenders) > 0 {
		t.Fatalf("hard-coded label keys found in: %s", strings.Join(offenders, ", "))
	}
}

// TestForbidFreeFormProtocols ensures protocol literals ("tcp", "udp")
// are not used directly in manifest-related code. Protocols should use
// the typed enum in internal/domain/types/network.go and validation helpers.
func TestForbidFreeFormProtocols(t *testing.T) {
	// Scan only manifest compilation/CLI manifest code paths.
	roots := []string{
		filepath.Join("internal", "workflow", "manifests"),
		filepath.Join("internal", "cli", "manifest"),
	}
	rx := regexp.MustCompile(`"(?:tcp|udp)"`)

	var offenders []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if rx.Match(b) {
				offenders = append(offenders, path)
			}
			return nil
		})
	}
	if len(offenders) > 0 {
		t.Fatalf("free-form protocol literals found in: %s", strings.Join(offenders, ", "))
	}
}
