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

	assertContains("export PLOY_SERVER_PG_DSN='postgres://user:pass@localhost:5432/ploy?sslmode=disable'")
	assertContains("export PLOY_INSTALL_POSTGRESQL='true'")
	assertContains("export PLOY_CA_CERT_PEM='-----BEGIN CERTIFICATE-----")

	// Verify functional body fragments exist
	assertContains("mkdir -p /etc/ploy/pki")
	assertContains("cat > /etc/ploy/ployd.yaml")
	assertContains("systemctl daemon-reload")
}

func TestRenderBootstrapScript_PostgreSQLInstallWithoutDSN(t *testing.T) {
	script := renderBootstrapScript(map[string]string{
		"PLOY_INSTALL_POSTGRESQL": "true",
		"PLOY_CA_CERT_PEM":        "-----BEGIN CERTIFICATE-----\nXYZ\n-----END CERTIFICATE-----\n",
	})

	assertContains := func(needle string) {
		if !strings.Contains(script, needle) {
			t.Fatalf("expected script to contain %q", needle)
		}
	}

	// Should contain the install flag
	assertContains("export PLOY_INSTALL_POSTGRESQL='true'")

	// Should NOT contain PLOY_SERVER_PG_DSN in the initial environment exports
	// (but it's fine if it appears later in the script as part of derive_postgresql_dsn function)
	lines := strings.Split(script, "\n")
	foundInEnvExports := false
	inEnvSection := true
	for i, line := range lines {
		// The environment exports section ends at the first non-export line after exports start
		if inEnvSection && strings.HasPrefix(strings.TrimSpace(line), "export ") {
			if strings.Contains(line, "PLOY_SERVER_PG_DSN") {
				foundInEnvExports = true
				t.Logf("Found PLOY_SERVER_PG_DSN in env exports at line %d: %q", i, line)
			}
		} else if inEnvSection && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "export ") {
			// End of environment exports section
			inEnvSection = false
		}
	}

	if foundInEnvExports {
		t.Fatalf("PLOY_SERVER_PG_DSN should not be in initial environment exports when installing PostgreSQL")
	}

	// Verify functional body fragments exist
	assertContains("mkdir -p /etc/ploy/pki")
	assertContains("CREATE DATABASE ploy OWNER ploy")
	assertContains("systemctl enable postgresql")
}
