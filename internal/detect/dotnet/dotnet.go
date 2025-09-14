package dotnet

import (
    "encoding/json"
    "os"
    "path/filepath"
    "regexp"
)

// DetectVersion tries to determine .NET SDK/runtime from global.json or *.csproj TargetFramework
func DetectVersion(srcDir string) string {
    if v := detectFromGlobalJSON(filepath.Join(srcDir, "global.json")); v != "" { return v }
    if v := detectFromCsproj(srcDir); v != "" { return v }
    return ""
}

func detectFromGlobalJSON(path string) string {
    b, err := os.ReadFile(path)
    if err != nil { return "" }
    var obj struct { Sdk struct{ Version string `json:"version"` } `json:"sdk"` }
    if json.Unmarshal(b, &obj) == nil { return obj.Sdk.Version }
    return ""
}

func detectFromCsproj(dir string) string {
    // naive: search for first .csproj and parse TargetFramework
    entries, _ := os.ReadDir(dir)
    for _, e := range entries {
        if !e.IsDir() && filepath.Ext(e.Name()) == ".csproj" {
            b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
            re := regexp.MustCompile(`<TargetFramework>\s*([^<]+)\s*</TargetFramework>`)
            if re.Match(b) { return re.FindStringSubmatch(string(b))[1] }
        }
    }
    return ""
}

