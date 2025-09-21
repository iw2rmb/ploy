package orchestration

import (
	"strings"
	"testing"

	platformnomad "github.com/iw2rmb/ploy/platform/nomad"
)

func TestEmbeddedTemplatesPresent(t *testing.T) {
	if b := getEmbeddedTemplate("platform/nomad/lane-d-jail.hcl"); len(b) == 0 {
		t.Fatalf("expected embedded template for lane D")
	}
	if b := getEmbeddedTemplate("platform/nomad/debug-oci.hcl"); len(b) == 0 {
		t.Fatalf("expected embedded debug template")
	}
	if b := getEmbeddedTemplate("platform/nomad/jetstream.nomad.hcl"); len(b) == 0 {
		t.Fatalf("expected embedded jetstream template")
	}
}

func TestEmbeddedTemplatesHaveNoVaultReferences(t *testing.T) {
	for _, path := range platformnomad.ListEmbeddedTemplatePaths() {
		content := platformnomad.GetEmbeddedTemplate(path)
		if len(content) == 0 {
			t.Fatalf("expected embedded template for %s", path)
		}
		if strings.Contains(strings.ToLower(string(content)), "vault") {
			t.Fatalf("forbidden secret-manager token found in embedded template %s", path)
		}
	}
}
