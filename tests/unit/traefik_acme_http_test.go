package unit_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTraefikDefaultAcmeResolverUsesHttpAndTlsChallenges(t *testing.T) {
	p := filepath.FromSlash(filepath.Join("iac", "common", "templates", "nomad-traefik-system.hcl.j2"))
	content := mustReadFile(t, p)
	required := []string{
		"--entrypoints.websecure.http.tls.certresolver=default-acme",
		"--certificatesresolvers.default-acme.acme.email=",
		"--certificatesresolvers.default-acme.acme.storage=/data/default-acme.json",
		"--certificatesresolvers.default-acme.acme.httpchallenge=true",
		"--certificatesresolvers.default-acme.acme.httpchallenge.entrypoint=web",
		"--certificatesresolvers.default-acme.acme.tlschallenge=true",
	}

	for _, needle := range required {
		if !strings.Contains(content, needle) {
			t.Fatalf("missing %s in %s", needle, p)
		}
	}

	if strings.Contains(content, "namecheap") {
		t.Fatalf("unexpected legacy Namecheap configuration in %s", p)
	}
}
