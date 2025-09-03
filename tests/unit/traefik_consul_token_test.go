package unit_test

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

// helper to read file content, t.Fatal on error for concise tests
func mustReadFile(t *testing.T, p string) string {
    t.Helper()
    b, err := os.ReadFile(p)
    if err != nil {
        t.Fatalf("failed to read %s: %v", p, err)
    }
    return string(b)
}

// Test that Traefik configs support Consul Catalog token configuration across Nomad and Ansible paths.
func TestTraefikConsulCatalogTokenWiring(t *testing.T) {
    // Nomad system job template
    {
        p := filepath.FromSlash(filepath.Join("..", "..", "iac", "common", "templates", "nomad-traefik-system.hcl.j2"))
        content := mustReadFile(t, p)
        if !strings.Contains(content, "--providers.consulcatalog.endpoint.token=") {
            t.Fatalf("missing consulcatalog endpoint token flag in %s", p)
        }
        if !strings.Contains(content, "CONSUL_HTTP_TOKEN") {
            t.Fatalf("missing CONSUL_HTTP_TOKEN environment wiring in %s", p)
        }
    }

    // Platform Nomad job example (kept in repo for reference) should include token flag too
    {
        p := filepath.FromSlash(filepath.Join("..", "..", "platform", "nomad", "traefik.hcl"))
        content := mustReadFile(t, p)
        if !strings.Contains(content, "--providers.consulcatalog.endpoint.token=") {
            t.Fatalf("missing consulcatalog endpoint token flag in %s", p)
        }
    }

    // Ansible Traefik static config should include token key under consulCatalog.endpoint
    {
        p := filepath.FromSlash(filepath.Join("..", "..", "iac", "dev", "playbooks", "traefik.yml"))
        content := mustReadFile(t, p)
        if !strings.Contains(content, "consulCatalog:") || !strings.Contains(content, "endpoint:") {
            t.Fatalf("consulCatalog endpoint block not found in %s", p)
        }
        if !strings.Contains(content, "token:") {
            t.Fatalf("missing token field under consulCatalog.endpoint in %s", p)
        }
    }
}
