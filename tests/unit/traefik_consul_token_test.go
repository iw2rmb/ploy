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
		if strings.Contains(content, "--providers.consulcatalog.endpoint.token=") {
			t.Fatalf("unexpected consulcatalog endpoint token flag in %s (should rely on env)", p)
		}
		if !strings.Contains(content, "CONSUL_HTTP_TOKEN") {
			t.Fatalf("missing CONSUL_HTTP_TOKEN environment wiring in %s", p)
		}
	}

	// Platform Nomad job example should not include token flag in args
	{
		p := filepath.FromSlash(filepath.Join("..", "..", "platform", "nomad", "traefik.hcl"))
		content := mustReadFile(t, p)
		if strings.Contains(content, "--providers.consulcatalog.endpoint.token=") {
			t.Fatalf("unexpected consulcatalog endpoint token flag in %s (should rely on env)", p)
		}
	}

	// Dev site playbook should NOT import systemd-based Traefik playbook anymore
	{
		p := filepath.FromSlash(filepath.Join("..", "..", "iac", "dev", "site.yml"))
		content := mustReadFile(t, p)
		if strings.Contains(content, "playbooks/traefik.yml") {
			t.Fatalf("site.yml still imports systemd-based playbook: %s", p)
		}
	}
}
