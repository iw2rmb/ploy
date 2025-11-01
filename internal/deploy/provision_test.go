package deploy

import (
    "strings"
    "testing"
)

func TestRenderBootstrapScript_InjectsServerEnv(t *testing.T) {
    script := renderBootstrapScript(map[string]string{
        "PLOY_SERVER_PG_DSN":      "postgres://user:pass@localhost:5432/ploy?sslmode=disable",
        "PLOY_INSTALL_POSTGRESQL": "true",
        "PLOY_CA_CERT_PEM":        "-----BEGIN CERTIFICATE-----\nABC\n-----END CERTIFICATE-----\n",
    })

    assertContains := func(needle string) {
        if !strings.Contains(script, needle) {
            t.Fatalf("expected script to contain %q, got: %q", needle, script)
        }
    }

    assertContains("export PLOY_SERVER_PG_DSN=\"postgres://user:pass@localhost:5432/ploy?sslmode=disable\"")
    assertContains("export PLOY_INSTALL_POSTGRESQL=\"true\"")
    assertContains("export PLOY_CA_CERT_PEM=\"-----BEGIN CERTIFICATE-----")
}

