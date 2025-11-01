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
		"BOOTSTRAP_PRIMARY":       "true",
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

	// Assert server config file fragments exist
	assertContains("cat > /etc/ploy/ployd.yaml <<'EOF'")
	assertContains("http:")
	assertContains("listen: :8443")
	assertContains("tls:")
	assertContains("enabled: true")
	assertContains("cert: /etc/ploy/pki/server.crt")
	assertContains("key: /etc/ploy/pki/server.key")
	assertContains("client_ca: /etc/ploy/pki/ca.crt")
	assertContains("require_client_cert: true")
	assertContains("metrics:")
	assertContains("listen: :9100")
	assertContains("postgres:")
	assertContains("dsn: ${PLOY_SERVER_PG_DSN}")

	// Assert server systemd unit fragments exist
	assertContains("cat > /etc/systemd/system/ployd.service <<'EOF'")
	assertContains("[Unit]")
	assertContains("Description=Ploy Server")
	assertContains("After=network.target postgresql.service")
	assertContains("[Service]")
	assertContains("Type=simple")
	assertContains("ExecStart=/usr/local/bin/ployd")
	assertContains("Restart=always")
	assertContains("RestartSec=5")
	assertContains("User=root")
	assertContains("Environment=PLOYD_CONFIG_PATH=/etc/ploy/ployd.yaml")
	assertContains("[Install]")
	assertContains("WantedBy=multi-user.target")
	assertContains("systemctl enable --now ployd.service")
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

func TestRenderBootstrapScript_NodeConfigAndUnitFragments(t *testing.T) {
	script := renderBootstrapScript(map[string]string{
		"PLOY_CA_CERT_PEM":     "-----BEGIN CERTIFICATE-----\nNODE\n-----END CERTIFICATE-----\n",
		"PLOY_SERVER_CERT_PEM": "-----BEGIN CERTIFICATE-----\nNODE_CERT\n-----END CERTIFICATE-----\n",
		"PLOY_SERVER_KEY_PEM":  "-----BEGIN PRIVATE KEY-----\nNODE_KEY\n-----END PRIVATE KEY-----\n",
		"PLOY_SERVER_URL":      "https://server.example.com:8443",
		"NODE_ID":              "node-123",
		"BOOTSTRAP_PRIMARY":    "false",
	})

	assertContains := func(needle string) {
		if !strings.Contains(script, needle) {
			t.Fatalf("expected script to contain %q", needle)
		}
	}

	// Assert node config file fragments exist
	assertContains("cat > /etc/ploy/ployd-node.yaml <<'EOF'")
	assertContains("server_url: ${PLOY_SERVER_URL:-}")
	assertContains("node_id: ${NODE_ID:-}")
	assertContains("http:")
	assertContains("listen: :8444")
	assertContains("tls:")
	assertContains("enabled: true")
	assertContains("ca_path: /etc/ploy/pki/ca.crt")
	assertContains("cert_path: /etc/ploy/pki/node.crt")
	assertContains("key_path: /etc/ploy/pki/node.key")
	assertContains("heartbeat:")
	assertContains("interval: 30s")
	assertContains("timeout: 10s")

	// Assert node systemd unit fragments exist
	assertContains("cat > /etc/systemd/system/ployd-node.service <<'EOF'")
	assertContains("[Unit]")
	assertContains("Description=Ploy Node Agent")
	assertContains("After=network.target")
	assertContains("[Service]")
	assertContains("Type=simple")
	assertContains("ExecStart=/usr/local/bin/ployd-node")
	assertContains("Restart=always")
	assertContains("RestartSec=5")
	assertContains("User=root")
	assertContains("[Install]")
	assertContains("WantedBy=multi-user.target")
	assertContains("systemctl enable --now ployd-node.service")

	// Verify node-specific PKI paths
	assertContains("echo \"$PLOY_SERVER_CERT_PEM\" > /etc/ploy/pki/node.crt")
	assertContains("echo \"$PLOY_SERVER_KEY_PEM\" > /etc/ploy/pki/node.key")
}
