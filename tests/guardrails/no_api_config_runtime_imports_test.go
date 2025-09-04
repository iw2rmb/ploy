package guardrails

import (
    "io/fs"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// findRepoRoot walks up from the current dir to locate go.mod as an anchor
func findRepoRoot(start string) string {
    dir := start
    for i := 0; i < 6; i++ { // walk up a few levels at most
        if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
            return dir
        }
        parent := filepath.Dir(dir)
        if parent == dir { break }
        dir = parent
    }
    return start
}

// Test_NoApiConfigImports ensures runtime code does not import api/config
func Test_NoApiConfigImports(t *testing.T) {
    root := findRepoRoot(".")
    forbidden := "\"github.com/iw2rmb/ploy/api/config\""
    var offenders []string
    _ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if err != nil { return nil }
        if d.IsDir() {
            // Skip api/config package itself and VCS/build/test dirs
            parts := strings.Split(path, string(os.PathSeparator))
            if len(parts) >= 2 && parts[len(parts)-2] == "api" && parts[len(parts)-1] == "config" { return filepath.SkipDir }
            base := d.Name()
            if strings.HasPrefix(base, ".") || base == "vendor" || base == "coverage" || base == "test-results" { return filepath.SkipDir }
            if base == "roadmap" || base == "docs" || base == "research" { return filepath.SkipDir }
            return nil
        }
        if !strings.HasSuffix(path, ".go") { return nil }
        if strings.HasSuffix(path, "_test.go") { return nil }
        b, err := os.ReadFile(path)
        if err != nil { return nil }
        if strings.Contains(string(b), forbidden) {
            offenders = append(offenders, path)
        }
        return nil
    })
    if len(offenders) > 0 {
        t.Fatalf("runtime files must not import api/config; found in:\n%s", strings.Join(offenders, "\n"))
    }
}

