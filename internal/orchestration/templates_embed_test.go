package orchestration

import "testing"

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
