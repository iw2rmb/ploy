package orchestration

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// TestRenderTemplate_UsesTemplateDir verifies that PLOY_TEMPLATE_DIR is searched for templates
func TestRenderTemplate_UsesTemplateDir(t *testing.T) {
    dir := t.TempDir()
    // Create nested path platform/nomad/lane-c-osv.hcl under temp dir
    nested := filepath.Join(dir, "platform", "nomad")
    if err := os.MkdirAll(nested, 0o755); err != nil {
        t.Fatalf("failed to create nested dirs: %v", err)
    }
    templatePath := filepath.Join(nested, "lane-c-osv.hcl")
    content := "job \"{{APP_NAME}}\" { # domain {{DOMAIN_SUFFIX}} }"
    if err := os.WriteFile(templatePath, []byte(content), 0o644); err != nil {
        t.Fatalf("failed to write template: %v", err)
    }

    t.Setenv("PLOY_TEMPLATE_DIR", dir)

    data := RenderData{App: "myapp", ImagePath: "/tmp/image", DockerImage: "busybox"}
    outPath, err := RenderTemplate("c", data)
    if err != nil {
        t.Fatalf("RenderTemplate failed: %v", err)
    }
    b, err := os.ReadFile(outPath)
    if err != nil {
        t.Fatalf("failed to read output: %v", err)
    }
    s := string(b)
    if !strings.Contains(s, "job \"myapp\"") {
        t.Fatalf("expected app name substituted, got: %s", s)
    }
}

