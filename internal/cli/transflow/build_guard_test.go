package transflow

import (
    "os"
    "path/filepath"
    "testing"
)

func TestCheckBuildFilesAndEnsure(t *testing.T) {
    dir := t.TempDir()
    // No files -> error
    if err := ensureBuildFile(dir); err == nil {
        t.Fatalf("expected error for missing build files")
    }
    // Add pom.xml -> ok
    if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0644); err != nil {
        t.Fatal(err)
    }
    if err := ensureBuildFile(dir); err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
    hasPom, hasGradle, hasKts := checkBuildFiles(dir)
    if !hasPom || hasGradle || hasKts {
        t.Fatalf("unexpected flags: pom=%v gradle=%v kts=%v", hasPom, hasGradle, hasKts)
    }
}

