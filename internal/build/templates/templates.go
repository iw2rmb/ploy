package templates

import (
    "bytes"
    "embed"
    "fmt"
    "text/template"
)

// Dockerfile templates are embedded to keep deployment simple and robust.
//
// Layout:
// - dockerfiles/
//   - java/
//     - gradle.Dockerfile.tmpl
//     - maven.Dockerfile.tmpl
//   - default.Dockerfile.tmpl (reserved for future generic fallback)

//go:embed dockerfiles/**/*
var Dockerfiles embed.FS

// Render renders an embedded text/template with the provided data.
func Render(path string, data any) (string, error) {
    b, err := Dockerfiles.ReadFile(path)
    if err != nil {
        return "", fmt.Errorf("template read failed: %w", err)
    }
    t, err := template.New(path).Option("missingkey=zero").Parse(string(b))
    if err != nil {
        return "", fmt.Errorf("template parse failed: %w", err)
    }
    var out bytes.Buffer
    if err := t.Execute(&out, data); err != nil {
        return "", fmt.Errorf("template render failed: %w", err)
    }
    return out.String(), nil
}

