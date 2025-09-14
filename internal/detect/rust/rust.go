package rust

import (
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

// DetectVersion tries to determine Rust toolchain from rust-toolchain or rust-toolchain.toml
func DetectVersion(srcDir string) string {
    if v := strings.TrimSpace(read(filepath.Join(srcDir, "rust-toolchain"))); v != "" { return v }
    if v := detectFromToolchainToml(filepath.Join(srcDir, "rust-toolchain.toml")); v != "" { return v }
    return ""
}

func read(p string) string { b, _ := os.ReadFile(p); return string(b) }

func detectFromToolchainToml(path string) string {
    txt := read(path)
    if txt == "" { return "" }
    if re := regexp.MustCompile(`channel\s*=\s*"([^"]+)"`); re.MatchString(txt) {
        return re.FindStringSubmatch(txt)[1]
    }
    return ""
}

