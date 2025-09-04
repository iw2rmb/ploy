package server

import (
    "io/fs"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// TestNoLegacyStorageFactory ensures server does not use legacy api/config factory helpers
func TestNoLegacyStorageFactory(t *testing.T) {
    forbidden := []string{
        "CreateStorageClientFromConfig(",
        "CreateStorageFromFactory(",
    }
    var offenders []string
    _ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
        if err != nil { return nil }
        if d.IsDir() { return nil }
        if !strings.HasSuffix(path, ".go") { return nil }
        b, err := os.ReadFile(path)
        if err != nil { return nil }
        s := string(b)
        for _, f := range forbidden {
            if strings.Contains(s, f) {
                offenders = append(offenders, path+": contains "+f)
            }
        }
        return nil
    })
    if len(offenders) > 0 {
        t.Fatalf("legacy storage factory usage detected:\n%s", strings.Join(offenders, "\n"))
    }
}

