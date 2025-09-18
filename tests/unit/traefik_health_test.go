package unit_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// Ensure Traefik health check is aligned with entrypoint configuration
func TestTraefikPingEntrypointConfigured(t *testing.T) {
	// Template should set ping entrypoint to admin
	{
		p := filepath.FromSlash(filepath.Join("iac", "common", "templates", "nomad-traefik-system.hcl.j2"))
		content := mustReadFile(t, p)
		if !strings.Contains(content, "--ping.entryPoint=admin") {
			t.Fatalf("missing --ping.entryPoint=admin in %s", p)
		}
		// Service check should target admin port with /ping
		if !strings.Contains(content, "port = \"admin\"") || !strings.Contains(content, "path = \"/ping\"") {
			t.Fatalf("missing admin /ping health check in %s", p)
		}
	}
	// Example job should also include ping entrypoint
	{
		p := filepath.FromSlash(filepath.Join("platform", "nomad", "traefik.hcl"))
		content := mustReadFile(t, p)
		if !strings.Contains(content, "--ping.entryPoint=admin") {
			t.Fatalf("missing --ping.entryPoint=admin in %s", p)
		}
	}
}
