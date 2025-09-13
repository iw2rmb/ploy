package mods

import (
	"os"
	"path/filepath"
	"testing"
)

// helper to write a minimal unified diff touching provided paths
func writeMinimalDiff(t *testing.T, dir string, paths []string) string {
	t.Helper()
	diffPath := filepath.Join(dir, "test.diff")
	f, err := os.Create(diffPath)
	if err != nil {
		t.Fatalf("failed to create diff file: %v", err)
	}
	defer f.Close()
	for _, p := range paths {
		// write only headers; ValidateDiffPaths only parses +++ lines
		_, _ = f.WriteString("--- a/" + p + "\n")
		_, _ = f.WriteString("+++ b/" + p + "\n")
	}
	_ = f.Sync()
	return diffPath
}

func TestValidateDiffPaths_DoublestarRecursiveJavaAllowed(t *testing.T) {
	tmp := t.TempDir()
	diff := writeMinimalDiff(t, tmp, []string{
		"src/main/java/com/example/App.java",
		"src/test/java/com/example/AppTest.java",
	})

	// recursive pattern with doublestar should match nested java files
	allowed := []string{"src/**/*.java"}
	if err := ValidateDiffPaths(diff, allowed); err != nil {
		t.Fatalf("expected diff to be allowed, got error: %v", err)
	}
}

func TestValidateDiffPaths_DirectoryRecursiveAllowed(t *testing.T) {
	tmp := t.TempDir()
	diff := writeMinimalDiff(t, tmp, []string{
		"src/main/resources/application.yml",
		"src/main/java/App.java",
	})

	allowed := []string{"src/**"}
	if err := ValidateDiffPaths(diff, allowed); err != nil {
		t.Fatalf("expected diff under src to be allowed, got error: %v", err)
	}
}

func TestValidateDiffPaths_MixedAllowedAndBlocked(t *testing.T) {
	tmp := t.TempDir()
	diff := writeMinimalDiff(t, tmp, []string{
		"pom.xml",           // allowed
		"build.gradle",      // not allowed
		"src/main/App.java", // allowed by src/**
	})

	allowed := []string{"src/**", "pom.xml"}
	if err := ValidateDiffPaths(diff, allowed); err == nil {
		t.Fatalf("expected error due to build.gradle being disallowed, got nil")
	}
}
