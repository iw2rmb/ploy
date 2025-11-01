package main

import (
    "errors"
    "flag"
    "fmt"
    "os"
    "path/filepath"
    "runtime"
)

// stringValue implements flag.Value with a marker for whether it was set.
type stringValue struct{ set bool; value string }

func (v *stringValue) String() string { return v.value }
func (v *stringValue) Set(s string) error { v.value = s; v.set = true; return nil }

// intValue implements flag.Value for integers.
type intValue struct{ set bool; value int }

func (v *intValue) String() string { return fmt.Sprintf("%d", v.value) }
func (v *intValue) Set(s string) error {
    // Let the FlagSet parse ints instead of re-implementing; keep minimal here.
    var tmp flag.FlagSet
    var parsed int
    tmp.IntVar(&parsed, "v", 0, "")
    if err := tmp.Parse([]string{"-v", s}); err != nil { return err }
    v.value = parsed; v.set = true; return nil
}

// resolveIdentityPath chooses a default SSH identity when not explicitly set.
func resolveIdentityPath(v stringValue) (string, error) {
    if v.set {
        return expandPath(v.value), nil
    }
    home, err := os.UserHomeDir()
    if err != nil { return "", err }
    return filepath.Join(home, ".ssh", "id_rsa"), nil
}

// resolvePloydBinaryPath locates the ployd binary adjacent to the CLI.
func resolvePloydBinaryPath(v stringValue) (string, error) {
    if v.set {
        return expandPath(v.value), nil
    }
    execPath, err := os.Executable()
    if err != nil {
        return "", fmt.Errorf("locate ploy executable: %w", err)
    }
    dir := filepath.Dir(execPath)
    osName := runtime.GOOS
    candidates := make([]string, 0, 3)
    if osName != "linux" {
        candidates = append(candidates, filepath.Join(dir, "ployd-linux"))
    }
    if osName == "windows" {
        candidates = append(candidates, filepath.Join(dir, "ployd.exe"))
    }
    candidates = append(candidates, filepath.Join(dir, "ployd"))
    for _, c := range candidates {
        if info, err := os.Stat(c); err == nil && !info.IsDir() {
            return c, nil
        }
    }
    return "", errors.New("ploy server deploy: ployd binary not found alongside CLI; provide --ployd-binary")
}

func expandPath(path string) string {
    if path == "" { return "" }
    if path == "~" {
        if home, err := os.UserHomeDir(); err == nil { return home }
        return path
    }
    if len(path) > 2 && path[:2] == "~/" {
        if home, err := os.UserHomeDir(); err == nil { return filepath.Join(home, path[2:]) }
    }
    return path
}

