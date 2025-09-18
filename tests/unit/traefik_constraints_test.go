package unit_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// Test that Traefik Nomad templates are constrained to gateway/edge nodes via node.class
func TestTraefikGatewayNodeConstraint(t *testing.T) {
	{
		p := filepath.FromSlash(filepath.Join("iac", "common", "templates", "nomad-traefik-system.hcl.j2"))
		content := mustReadFile(t, p)
		if !strings.Contains(content, "attribute = \"${meta.") {
			t.Fatalf("missing meta.* constraint in %s", p)
		}
		if !strings.Contains(content, "ploy_gateway_node_class") {
			t.Fatalf("missing ploy_gateway_node_class variable in %s", p)
		}
	}
	{
		p := filepath.FromSlash(filepath.Join("platform", "nomad", "traefik.hcl"))
		content := mustReadFile(t, p)
		if !strings.Contains(content, "attribute = \"${meta.role}\"") || !strings.Contains(content, "value     = \"gateway\"") {
			t.Fatalf("missing meta.role gateway constraint in %s", p)
		}
	}
}
