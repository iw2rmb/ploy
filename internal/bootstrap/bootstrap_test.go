package bootstrap

import (
	"strings"
	"testing"
)

func TestDefaultExportsAndPrefixedScript(t *testing.T) {
	env := DefaultExports()
	if env["PLOY_BOOTSTRAP_VERSION"] == "" {
		t.Fatalf("expected PLOY_BOOTSTRAP_VERSION in defaults")
	}

	script := PrefixedScript(map[string]string{
		"FOO": "bar baz",
		"QUX": "qu\"ote",
	})
	if !strings.Contains(script, "export FOO='bar baz'") {
		t.Fatalf("missing FOO export in script: %q", script)
	}
	// Double quotes are fine inside single-quoted values (no escaping needed).
	if !strings.Contains(script, "export QUX='qu\"ote'") {
		t.Fatalf("missing single-quoted QUX export in script: %q", script)
	}
	if !strings.Contains(script, "# ploy bootstrap script") {
		t.Fatalf("missing bootstrap script comment in script")
	}
}

func TestPrefixedScript_CreatesDirectories(t *testing.T) {
	script := PrefixedScript(map[string]string{})
	if !strings.Contains(script, "mkdir -p /etc/ploy/pki") {
		t.Fatalf("script should create /etc/ploy/pki directory")
	}
}

func TestPrefixedScript_WritesCertsFromEnv(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"PLOY_CA_CERT_PEM":     "ca-cert-content",
		"PLOY_SERVER_CERT_PEM": "leaf-cert-content",
		"PLOY_SERVER_KEY_PEM":  "leaf-key-content",
	})

	if !strings.Contains(script, "echo \"$PLOY_CA_CERT_PEM\" > /etc/ploy/pki/ca.crt") {
		t.Fatalf("script should write CA cert")
	}
	// The script contains both primary (server) and else (node) branches.
	if !strings.Contains(script, "echo \"$PLOY_SERVER_CERT_PEM\" > /etc/ploy/pki/server.crt") {
		t.Fatalf("script should include server cert write in primary branch")
	}
	if !strings.Contains(script, "echo \"$PLOY_SERVER_CERT_PEM\" > /etc/ploy/pki/node.crt") {
		t.Fatalf("script should include node cert write in else branch")
	}
	if !strings.Contains(script, "chmod 600 /etc/ploy/pki/server.key") {
		t.Fatalf("script should set secure permissions on server key in primary branch")
	}
	if !strings.Contains(script, "chmod 600 /etc/ploy/pki/node.key") {
		t.Fatalf("script should set secure permissions on node key in else branch")
	}
}

// singleQuoteTest mirrors the package's single-quote logic for expectations.
func singleQuoteTest(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func TestPrefixedScript_ExportQuoting_NoExpansion(t *testing.T) {
	val := "a$b `cmd` and 'q'"
	script := PrefixedScript(map[string]string{"X": val})
	expected := "export X=" + singleQuoteTest(val)
	if !strings.Contains(script, expected) {
		t.Fatalf("expected shell-safe single-quoted export, want %q in script: %q", expected, script)
	}
}

func TestPrefixedScript_IgnoresEmptyOrWhitespaceKeys(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"":  "ignored",
		" ": "ignored",
		"A": "",
	})
	if strings.Contains(script, "export =") {
		t.Fatalf("script should not contain exports for empty keys: %q", script)
	}
	if !strings.Contains(script, "export A=''") {
		t.Fatalf("empty value must be exported as two single quotes: %q", script)
	}
}

func TestPrefixedScript_PostgreSQLInstallation(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"PLOY_INSTALL_POSTGRESQL": "true",
	})

	if !strings.Contains(script, "if [ \"${PLOY_INSTALL_POSTGRESQL:-false}\" = \"true\" ]; then") {
		t.Fatalf("script should check PLOY_INSTALL_POSTGRESQL flag")
	}
	if !strings.Contains(script, "apt-get install -y -qq postgresql postgresql-contrib") {
		t.Fatalf("script should install PostgreSQL via apt-get")
	}
	if !strings.Contains(script, "yum install -y -q postgresql-server postgresql-contrib") {
		t.Fatalf("script should install PostgreSQL via yum")
	}
	if !strings.Contains(script, "CREATE USER ploy WITH PASSWORD") {
		t.Fatalf("script should create ploy database user")
	}
	if !strings.Contains(script, "CREATE DATABASE ploy OWNER ploy") {
		t.Fatalf("script should create ploy database")
	}
	if !strings.Contains(script, "export PLOY_SERVER_PG_DSN=") {
		t.Fatalf("script should export derived DSN")
	}
}

func TestPrefixedScript_ServerConfig(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"BOOTSTRAP_PRIMARY": "true",
	})

	if !strings.Contains(script, "if [ \"${BOOTSTRAP_PRIMARY:-false}\" = \"true\" ]; then") {
		t.Fatalf("script should check BOOTSTRAP_PRIMARY flag")
	}
	if !strings.Contains(script, "cat > /etc/ploy/ployd.yaml") {
		t.Fatalf("script should write server config")
	}
	if !strings.Contains(script, "postgres:") {
		t.Fatalf("server config should include postgres section")
	}
	if !strings.Contains(script, "http:") || !strings.Contains(script, "tls:") {
		t.Fatalf("server config should include http.tls section")
	}
	if !strings.Contains(script, "control_plane:") {
		t.Fatalf("server config should include control_plane section")
	}
	if !strings.Contains(script, "cat > /etc/systemd/system/ployd.service") {
		t.Fatalf("script should install ployd.service systemd unit")
	}
	if !strings.Contains(script, "Description=Ploy Server") {
		t.Fatalf("ployd.service should have proper description")
	}
	if !strings.Contains(script, "Restart=always") {
		t.Fatalf("ployd.service should have Restart=always")
	}
}

func TestPrefixedScript_NodeConfig(t *testing.T) {
	script := PrefixedScript(map[string]string{})

	if !strings.Contains(script, "else") {
		t.Fatalf("script should have else branch for node config")
	}
	if !strings.Contains(script, "cat > /etc/ploy/ployd-node.yaml") {
		t.Fatalf("script should write node config in else branch")
	}
	if !strings.Contains(script, "server_url:") {
		t.Fatalf("node config should include server_url")
	}
	if !strings.Contains(script, "cert_path:") || !strings.Contains(script, "key_path:") || !strings.Contains(script, "ca_path:") {
		t.Fatalf("node config should include http.tls cert_path/key_path/ca_path")
	}
	if !strings.Contains(script, "cat > /etc/systemd/system/ployd-node.service") {
		t.Fatalf("script should install ployd-node.service systemd unit")
	}
	if !strings.Contains(script, "Description=Ploy Node Agent") {
		t.Fatalf("ployd-node.service should have proper description")
	}
}

func TestPrefixedScript_SystemdOperations(t *testing.T) {
	script := PrefixedScript(map[string]string{})

	if !strings.Contains(script, "systemctl daemon-reload") {
		t.Fatalf("script should run systemctl daemon-reload")
	}
	if !strings.Contains(script, "systemctl enable") {
		t.Fatalf("script should enable systemd service")
	}
	if !strings.Contains(script, "systemctl start") {
		t.Fatalf("script should start systemd service")
	}
}
