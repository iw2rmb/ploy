package deploy

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
	if !strings.Contains(script, "chmod 600 /etc/ploy/pki/server.key") {
		t.Fatalf("script should set secure permissions on server key in primary branch")
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
	if !strings.Contains(script, "DEBIAN_FRONTEND=noninteractive") {
		t.Fatalf("script should set DEBIAN_FRONTEND for apt installs")
	}
	if !strings.Contains(script, "apt-get update -qq") {
		t.Fatalf("script should run apt-get update -qq before install")
	}
	if !strings.Contains(script, "apt-get install -y -qq postgresql postgresql-contrib") {
		t.Fatalf("script should install PostgreSQL via apt-get")
	}
	if !strings.Contains(script, "yum install -y -q postgresql-server postgresql-contrib") {
		t.Fatalf("script should install PostgreSQL via yum")
	}
	if !strings.Contains(script, "postgresql-setup --initdb --unit postgresql") {
		t.Fatalf("script should initialize postgres on RHEL via postgresql-setup")
	}
	if !strings.Contains(script, "CREATE USER ploy WITH PASSWORD") {
		t.Fatalf("script should create ploy database user")
	}
	if !strings.Contains(script, "CREATE DATABASE ploy OWNER ploy") {
		t.Fatalf("script should create ploy database")
	}
	if !strings.Contains(script, "export PLOY_DB_DSN=") {
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
	if !strings.Contains(script, "http:") {
		t.Fatalf("server config should include http section")
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
	if !strings.Contains(script, "server_url: ${PLOY_SERVER_URL:-}") {
		t.Fatalf("node config should source server_url from $PLOY_SERVER_URL")
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
	if !strings.Contains(script, "Restart=always") {
		t.Fatalf("ployd-node.service should have Restart=always")
	}
}

func TestPrefixedScript_NodeInstallsDocker(t *testing.T) {
	script := PrefixedScript(map[string]string{})
	// Should contain a Docker installation step and service enablement
	if !strings.Contains(script, "Ensuring Docker is installed") {
		t.Fatalf("node branch should announce Docker installation")
	}
	if !strings.Contains(script, "systemctl enable --now docker") {
		t.Fatalf("node branch should enable docker service\nscript:\n%s", script)
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

func TestPrefixedScript_UsesEnableNow_ForServer(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"BOOTSTRAP_PRIMARY": "true",
	})
	if !strings.Contains(script, "systemctl enable --now ployd.service") {
		t.Fatalf("server branch should use 'systemctl enable --now ployd.service'\nscript:\n%s", script)
	}
}

func TestPrefixedScript_UsesEnableNow_ForNode(t *testing.T) {
	script := PrefixedScript(map[string]string{})
	if !strings.Contains(script, "systemctl enable --now ployd-node.service") {
		t.Fatalf("node branch should use 'systemctl enable --now ployd-node.service'\nscript:\n%s", script)
	}
}

func TestPrefixedScript_EchosFinalStatusAndKeyPaths(t *testing.T) {
	script := PrefixedScript(map[string]string{})

	// Check for final status summary header
	if !strings.Contains(script, "Bootstrap completed successfully.") {
		t.Fatalf("script should echo bootstrap completed successfully message")
	}
	if !strings.Contains(script, "========================================") {
		t.Fatalf("script should include header separator")
	}

	// Check for configuration section
	if !strings.Contains(script, "Configuration:") {
		t.Fatalf("script should echo Configuration section")
	}
	if !strings.Contains(script, "Config file: ${PLOY_CONFIG_PATH}") {
		t.Fatalf("script should echo config file path")
	}
	if !strings.Contains(script, "PKI directory: /etc/ploy/pki") {
		t.Fatalf("script should echo PKI directory path")
	}

	// Check for conditional PKI file paths
	if !strings.Contains(script, "if [ -f /etc/ploy/pki/ca.crt ]; then") {
		t.Fatalf("script should check for CA cert existence")
	}
	if !strings.Contains(script, "- CA cert: /etc/ploy/pki/ca.crt") {
		t.Fatalf("script should echo CA cert path if it exists")
	}
	if !strings.Contains(script, "- Server cert: /etc/ploy/pki/server.crt") {
		t.Fatalf("script should echo server cert path in primary branch")
	}
	if !strings.Contains(script, "- Server key: /etc/ploy/pki/server.key") {
		t.Fatalf("script should echo server key path in primary branch")
	}
	if !strings.Contains(script, "- Node cert: /etc/ploy/pki/node.crt") {
		t.Fatalf("script should echo node cert path in node branch")
	}
	if !strings.Contains(script, "- Node key: /etc/ploy/pki/node.key") {
		t.Fatalf("script should echo node key path in node branch")
	}

	// Check for service section
	if !strings.Contains(script, "Service:") {
		t.Fatalf("script should echo Service section")
	}
	if !strings.Contains(script, "Service name: ${PLOY_SERVICE_NAME}") {
		t.Fatalf("script should echo service name")
	}
	if !strings.Contains(script, "Status: $(systemctl is-active ${PLOY_SERVICE_NAME}") {
		t.Fatalf("script should echo service status")
	}
	if !strings.Contains(script, "Enabled: $(systemctl is-enabled ${PLOY_SERVICE_NAME}") {
		t.Fatalf("script should echo service enabled status")
	}

	// Check for helpful commands
	if !strings.Contains(script, "To view logs:") {
		t.Fatalf("script should echo logs viewing instructions")
	}
	if !strings.Contains(script, "journalctl -u ${PLOY_SERVICE_NAME} -f") {
		t.Fatalf("script should echo journalctl command for logs")
	}
	if !strings.Contains(script, "To check status:") {
		t.Fatalf("script should echo status check instructions")
	}
	if !strings.Contains(script, "systemctl status ${PLOY_SERVICE_NAME}") {
		t.Fatalf("script should echo systemctl status command")
	}
}

func TestPrefixedScript_SetsPLOY_CONFIG_PATH_ForServer(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"BOOTSTRAP_PRIMARY": "true",
	})
	if !strings.Contains(script, "PLOY_CONFIG_PATH='/etc/ploy/ployd.yaml'") {
		t.Fatalf("server branch should set PLOY_CONFIG_PATH to server config")
	}
	if !strings.Contains(script, "PLOY_SERVICE_NAME='ployd.service'") {
		t.Fatalf("server branch should set PLOY_SERVICE_NAME to ployd.service")
	}
}

func TestPrefixedScript_SetsPLOY_CONFIG_PATH_ForNode(t *testing.T) {
	script := PrefixedScript(map[string]string{})
	if !strings.Contains(script, "PLOY_CONFIG_PATH='/etc/ploy/ployd-node.yaml'") {
		t.Fatalf("node branch should set PLOY_CONFIG_PATH to node config")
	}
	if !strings.Contains(script, "PLOY_SERVICE_NAME='ployd-node.service'") {
		t.Fatalf("node branch should set PLOY_SERVICE_NAME to ployd-node.service")
	}
}

func TestPrefixedScript_ReuseExistingPKI(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"BOOTSTRAP_PRIMARY": "true",
	})

	// Check for existence check of ca.key
	if !strings.Contains(script, "if [ -f /etc/ploy/pki/ca.key ]; then") {
		t.Fatalf("script should check for existing /etc/ploy/pki/ca.key")
	}

	// Check for reuse message
	if !strings.Contains(script, "Existing PKI detected") {
		t.Fatalf("script should log message about existing PKI detection")
	}
	if !strings.Contains(script, "skipping PKI writes and reusing existing certificates") {
		t.Fatalf("script should log message about skipping PKI writes")
	}

	// Check that the else branch exists for writing new PKI
	if !strings.Contains(script, "  else\n") {
		t.Fatalf("script should have else branch for writing new PKI when ca.key does not exist")
	}
}

func TestPrefixedScript_ReuseSkipsPKIWrites(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"BOOTSTRAP_PRIMARY":    "true",
		"PLOY_CA_KEY_PEM":      "ca-key-content",
		"PLOY_SERVER_CERT_PEM": "server-cert-content",
		"PLOY_SERVER_KEY_PEM":  "server-key-content",
	})

	// Verify the reuse check comes before any PKI writes
	caKeyCheckIdx := strings.Index(script, "if [ -f /etc/ploy/pki/ca.key ]; then")
	if caKeyCheckIdx == -1 {
		t.Fatalf("script must check for ca.key existence")
	}

	// Verify PKI writes are inside the else block (indented with 4 spaces for nested if)
	if !strings.Contains(script, "    if [ -n \"${PLOY_CA_KEY_PEM:-}\" ]; then") {
		t.Fatalf("CA key write should be nested inside else block")
	}
	if !strings.Contains(script, "      echo \"$PLOY_CA_KEY_PEM\" > /etc/ploy/pki/ca.key") {
		t.Fatalf("CA key write command should be nested inside else block")
	}
	if !strings.Contains(script, "      echo \"$PLOY_SERVER_CERT_PEM\" > /etc/ploy/pki/server.crt") {
		t.Fatalf("server cert write should be nested inside else block")
	}
	if !strings.Contains(script, "      echo \"$PLOY_SERVER_KEY_PEM\" > /etc/ploy/pki/server.key") {
		t.Fatalf("server key write should be nested inside else block")
	}

	// Verify the structure: ca.key check → reuse message → else → PKI writes → fi
	reuseMsg := "Existing PKI detected"
	reuseIdx := strings.Index(script, reuseMsg)
	if reuseIdx == -1 {
		t.Fatalf("script must contain reuse message")
	}
	if reuseIdx < caKeyCheckIdx {
		t.Fatalf("reuse message should come after ca.key check")
	}

	elseIdx := strings.Index(script[caKeyCheckIdx:], "  else\n")
	if elseIdx == -1 {
		t.Fatalf("script must have else branch after reuse check")
	}

	caWriteIdx := strings.Index(script, "echo \"$PLOY_CA_KEY_PEM\" > /etc/ploy/pki/ca.key")
	if caWriteIdx == -1 {
		t.Fatalf("script must contain CA key write")
	}
	if caWriteIdx < caKeyCheckIdx+elseIdx {
		t.Fatalf("CA key write must come after the else branch")
	}
}

func TestPrefixedScript_ReuseBranchStructure(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"BOOTSTRAP_PRIMARY": "true",
	})

	// Verify proper nesting: the PKI reuse check must be inside BOOTSTRAP_PRIMARY block
	primaryCheckIdx := strings.Index(script, "if [ \"${BOOTSTRAP_PRIMARY:-false}\" = \"true\" ]; then")
	if primaryCheckIdx == -1 {
		t.Fatalf("script must check BOOTSTRAP_PRIMARY")
	}

	pkiCheckIdx := strings.Index(script, "if [ -f /etc/ploy/pki/ca.key ]; then")
	if pkiCheckIdx == -1 {
		t.Fatalf("script must check for ca.key")
	}

	if pkiCheckIdx < primaryCheckIdx {
		t.Fatalf("PKI reuse check must be inside BOOTSTRAP_PRIMARY block")
	}

	// Verify closing fi for the PKI check before the config write
	configWriteIdx := strings.Index(script, "cat > /etc/ploy/ployd.yaml")
	fiForPKICheck := strings.Index(script[pkiCheckIdx:configWriteIdx], "  fi\n")
	if fiForPKICheck == -1 {
		t.Fatalf("PKI reuse check must be closed with fi before config write")
	}
}

func TestPrefixedScript_ReuseSkipsCACertWrite_OnPrimary(t *testing.T) {
	script := PrefixedScript(map[string]string{
		"BOOTSTRAP_PRIMARY":    "true",
		"PLOY_CA_CERT_PEM":     "ca-cert-content",
		"PLOY_SERVER_CERT_PEM": "server-cert-content",
		"PLOY_SERVER_KEY_PEM":  "server-key-content",
	})

	// The CA cert write must not appear before the reuse check; it must be inside the else branch.
	caKeyCheckIdx := strings.Index(script, "if [ -f /etc/ploy/pki/ca.key ]; then")
	if caKeyCheckIdx == -1 {
		t.Fatalf("script must check for ca.key existence")
	}
	caCertWriteIdx := strings.Index(script, "echo \"$PLOY_CA_CERT_PEM\" > /etc/ploy/pki/ca.crt")
	if caCertWriteIdx == -1 {
		t.Fatalf("script must contain CA cert write when material provided")
	}

	// Ensure the first CA cert write comes after the else of the ca.key check (nested primary branch)
	elseIdxRel := strings.Index(script[caKeyCheckIdx:], "  else\n")
	if elseIdxRel == -1 {
		t.Fatalf("script must have else branch after reuse check")
	}
	elseIdx := caKeyCheckIdx + elseIdxRel
	if caCertWriteIdx < elseIdx {
		t.Fatalf("CA cert write must occur after the else branch in primary reuse logic")
	}

	// And should be indented for the nested block (four spaces)
	if !strings.Contains(script, "    echo \"$PLOY_CA_CERT_PEM\" > /etc/ploy/pki/ca.crt") {
		t.Fatalf("CA cert write should be nested inside else block with indentation")
	}
}
