package transflow

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

// ORWRecipeParams captures required coordinates for ORW apply job.
type ORWRecipeParams struct {
    Class         string
    Group         string
    Artifact      string
    Version       string
    PluginVersion string // optional
}

// writeORWPreHCL reads a rendered HCL template and writes a pre-substituted file with
// recipe coordinates, input tar host path, and run id. Returns the .pre.hcl path.
func writeORWPreHCL(renderedHCLPath string, params ORWRecipeParams, inputTarPath, runID string) (string, error) {
    b, err := os.ReadFile(renderedHCLPath)
    if err != nil { return "", err }

    content := string(b)
    // Required substitutions
    content = strings.ReplaceAll(content, "${RECIPE_CLASS}", params.Class)
    content = strings.ReplaceAll(content, "${RECIPE_GROUP}", params.Group)
    content = strings.ReplaceAll(content, "${RECIPE_ARTIFACT}", params.Artifact)
    content = strings.ReplaceAll(content, "${RECIPE_VERSION}", params.Version)
    content = strings.ReplaceAll(content, "${INPUT_TAR_HOST_PATH}", inputTarPath)
    content = strings.ReplaceAll(content, "${RUN_ID}", runID)
    // Optional
    if params.PluginVersion != "" {
        content = strings.ReplaceAll(content, "${MAVEN_PLUGIN_VERSION}", params.PluginVersion)
    }

    prePath := strings.ReplaceAll(renderedHCLPath, ".rendered.hcl", ".pre.hcl")
    if prePath == renderedHCLPath {
        base := filepath.Base(renderedHCLPath)
        prePath = filepath.Join(filepath.Dir(renderedHCLPath), fmt.Sprintf("%s.pre.hcl", base))
    }
    if err := os.WriteFile(prePath, []byte(content), 0644); err != nil {
        return "", err
    }
    return prePath, nil
}

