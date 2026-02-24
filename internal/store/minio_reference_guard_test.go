package store

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const goModuleFile = "go." + "mo" + "d"

func TestNoMinioReferencesOutsideHistoricalDocs(t *testing.T) {
	root := repoRoot(t)

	allow := map[string]struct{}{
		"internal/store/minio_reference_guard_test.go": {},
		"roadmap/garage.md":                            {},
	}

	targets := []string{
		"cmd",
		"internal",
		"local",
		"docs",
		"design",
		"roadmap",
		"scripts",
		goModuleFile,
		"go.sum",
	}

	var offenders []string
	for _, rel := range targets {
		abs := filepath.Join(root, rel)
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("stat %s: %v", rel, err)
		}
		if !info.IsDir() {
			checkFileForMinio(t, root, abs, allow, &offenders)
			continue
		}

		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				name := d.Name()
				if name == ".git" || name == "dist" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			checkFileForMinio(t, root, path, allow, &offenders)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", rel, err)
		}
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Fatalf("found forbidden MinIO references:\n%s", strings.Join(offenders, "\n"))
	}
}

func checkFileForMinio(t *testing.T, root, absPath string, allow map[string]struct{}, offenders *[]string) {
	t.Helper()

	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		t.Fatalf("rel %s: %v", absPath, err)
	}
	rel = filepath.ToSlash(rel)

	if _, ok := allow[rel]; ok {
		return
	}
	if !isScanTarget(rel) {
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if strings.Contains(strings.ToLower(string(data)), "minio") {
		*offenders = append(*offenders, rel)
	}
}

func isScanTarget(path string) bool {
	switch path {
	case goModuleFile, "go.sum":
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".sql", ".md", ".txt", ".yaml", ".yml", ".toml", ".sh":
		return true
	default:
		return false
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("repo root not found from %s: %v", wd, err)
	}
	return root
}
