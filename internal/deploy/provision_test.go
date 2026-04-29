package deploy

import (
	"strings"
	"testing"
)

func TestRenderBootstrapScript_InjectsServerEnv(t *testing.T) {
	script := renderBootstrapScript(map[string]string{
		"PLOY_DB_DSN":             "postgres://user:pass@localhost:5432/ploy?sslmode=disable",
		"PLOY_INSTALL_POSTGRESQL": "true",
		"PLOY_CA_CERT_PEM":        "-----BEGIN CERTIFICATE-----\nABC\n-----END CERTIFICATE-----\n",
		"BOOTSTRAP_PRIMARY":       "true",
	})

	assertContains := func(needle string) {
		// mark as helper for clearer failure locations
		t.Helper()
		if !strings.Contains(script, needle) {
			t.Fatalf("expected script to contain %q, got: %q", needle, script)
		}
	}

	assertContains("export PLOY_DB_DSN='postgres://user:pass@localhost:5432/ploy?sslmode=disable'")
	assertContains("export PLOY_INSTALL_POSTGRESQL='true'")
	assertContains("export PLOY_CA_CERT_PEM='-----BEGIN CERTIFICATE-----")

	// Verify functional body fragments exist
	assertContains("mkdir -p /etc/ploy/pki")
	assertContains("cat > /etc/ploy/cluster.env")
	assertContains("systemctl daemon-reload")

	// Assert server environment file fragments exist
	assertContains("PLOY_DB_DSN=${PLOY_DB_DSN}")
	assertContains("PLOYD_HTTP_LISTEN=${PLOYD_HTTP_LISTEN:-:8080}")
	assertContains("PLOYD_METRICS_LISTEN=${PLOYD_METRICS_LISTEN:-:9100}")

	// Assert server systemd unit fragments exist
	assertContains("cat > /etc/systemd/system/ployd.service <<EOF")
	assertContains("[Unit]")
	assertContains("Description=Ploy Server")
	assertContains("After=network.target postgresql.service")
	assertContains("[Service]")
	assertContains("Type=simple")
	assertContains("ExecStart=/usr/local/bin/ployd")
	assertContains("Restart=always")
	assertContains("RestartSec=5")
	assertContains("User=root")
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
		// mark as helper for clearer failure locations
		t.Helper()
		if !strings.Contains(script, needle) {
			t.Fatalf("expected script to contain %q", needle)
		}
	}

	// Should contain the install flag
	assertContains("export PLOY_INSTALL_POSTGRESQL='true'")

	// Should NOT contain PLOY_DB_DSN in the initial environment exports
	// (it will be set later during the PostgreSQL install section)
	lines := strings.Split(script, "\n")
	foundInEnvExports := false
	inEnvSection := true
	for i, line := range lines {
		// The environment exports section ends at the first non-export line after exports start
		if inEnvSection && strings.HasPrefix(strings.TrimSpace(line), "export ") {
			if strings.Contains(line, "PLOY_DB_DSN") {
				foundInEnvExports = true
				t.Logf("Found PLOY_DB_DSN in env exports at line %d: %q", i, line)
			}
		} else if inEnvSection && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "export ") {
			// End of environment exports section
			inEnvSection = false
		}
	}

	if foundInEnvExports {
		t.Fatalf("PLOY_DB_DSN should not be in initial environment exports when installing PostgreSQL")
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
		// mark as helper for clearer failure locations
		t.Helper()
		if !strings.Contains(script, needle) {
			t.Fatalf("expected script to contain %q", needle)
		}
	}

	// Assert node config file fragments exist
	assertContains("cat > /etc/ploy/ployd-node.yaml <<EOF")
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

	// Node-specific PKI is bootstrapped via token exchange; script should not write node cert/key.
}
