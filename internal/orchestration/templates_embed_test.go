package orchestration

import (
	"strings"
	"testing"

	platformnomad "github.com/iw2rmb/ploy/platform/nomad"
)

// RED: verify builder templates are embedded via getEmbeddedTemplate
func TestEmbeddedBuilderTemplatesPresent(t *testing.T) {
	cases := []string{
		"platform/nomad/lane-e-kaniko-builder.hcl",
		"platform/nomad/lane-c-osv-builder.hcl",
	}
	for _, p := range cases {
		if b := getEmbeddedTemplate(p); len(b) == 0 {
			t.Fatalf("expected embedded template for %s, got none", p)
		}
	}
}

func TestEmbeddedTemplatesHaveNoVaultReferences(t *testing.T) {
	for _, path := range platformnomad.ListEmbeddedTemplatePaths() {
		content := platformnomad.GetEmbeddedTemplate(path)
		if len(content) == 0 {
			t.Fatalf("expected embedded template for %s", path)
		}
		if strings.Contains(strings.ToLower(string(content)), "vault") {
			t.Fatalf("vault reference found in embedded template %s", path)
		}
	}
}
