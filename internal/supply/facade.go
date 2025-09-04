package supply

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"
)

// SBOMGenerationOptions represent options for SBOM generation
type SBOMGenerationOptions struct {
    Lane    string
    AppName string
    SHA     string
}

// SBOMGenerator generates SBOMs for source code
type SBOMGenerator struct{}

func DefaultSBOMOptions() SBOMGenerationOptions { return SBOMGenerationOptions{} }
func NewSBOMGenerator() *SBOMGenerator { return &SBOMGenerator{} }

// GenerateForSourceCode writes a minimal SBOM file into the source directory
func (g *SBOMGenerator) GenerateForSourceCode(srcDir string, opts SBOMGenerationOptions) error {
    sbom := map[string]any{
        "type":    "sbom",
        "source":  "source",
        "app":     opts.AppName,
        "sha":     opts.SHA,
        "lane":    opts.Lane,
        "created": time.Now().Format(time.RFC3339),
    }
    out := filepath.Join(srcDir, "SBOM.json")
    if err := writeJSON(out, sbom); err != nil { return err }
    return nil
}

// GenerateSBOM writes a minimal SBOM next to the target file when applicable
func GenerateSBOM(target, lane, app, sha string) error {
    // If target looks like a docker image (contains ':' and '/'), do nothing
    if strings.Contains(target, ":") && strings.Contains(target, "/") {
        return nil
    }
    sbom := map[string]any{
        "type":    "sbom",
        "source":  "artifact",
        "target":  filepath.Base(target),
        "app":     app,
        "sha":     sha,
        "lane":    lane,
        "created": time.Now().Format(time.RFC3339),
    }
    out := target + ".sbom.json"
    return writeJSON(out, sbom)
}

func SignArtifact(path string) error {
    sig := path + ".sig"
    if err := os.WriteFile(sig, []byte("signed-by: ploy"), 0o644); err != nil { return err }
    return nil
}

func SignDockerImage(image string) error { return nil }
func VerifySignature(path, sig string) error { return nil }

func writeJSON(path string, v any) error {
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return err }
    b, err := json.MarshalIndent(v, "", "  ")
    if err != nil { return fmt.Errorf("marshal sbom: %w", err) }
    return os.WriteFile(path, b, 0o644)
}
