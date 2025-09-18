package unit_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTraefikAdminPortTemplated(t *testing.T) {
	p := filepath.FromSlash(filepath.Join("iac", "common", "templates", "nomad-traefik-system.hcl.j2"))
	content := mustReadFile(t, p)
	// network port block should be templated
	if !strings.Contains(content, "port \"admin\" {") || !strings.Contains(content, "static = {{ traefik_admin_port") {
		t.Fatalf("admin port block not templated with traefik_admin_port in %s", p)
	}
	// entrypoint should use templated port
	if !strings.Contains(content, "--entrypoints.admin.address=:") || !strings.Contains(content, "{{ traefik_admin_port") {
		t.Fatalf("admin entrypoint address not templated with traefik_admin_port in %s", p)
	}
}
