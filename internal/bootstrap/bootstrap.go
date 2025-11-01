package bootstrap

// Minimal bootstrap script helpers used by deploy.ProvisionHost.
// These replace the legacy bootstrap package and generate a shell
// script that begins with environment exports followed by a simple
// body stub. Tests only assert the presence/absence of certain
// exports in the preamble, not the full script content.

import (
	"sort"
	"strings"
)

// Version identifies the bootstrap script bundle.
var Version = "dev"

// DefaultExports provides baseline environment variables included
// in every bootstrap script. Callers can override/extend via map merge.
func DefaultExports() map[string]string {
	return map[string]string{
		"PLOY_BOOTSTRAP_VERSION": Version,
	}
}

// PrefixedScript renders a shell script with an export preamble for env.
// Keys are emitted in sorted order for determinism.
func PrefixedScript(env map[string]string) string {
	// Deterministic key order for stable tests
	keys := make([]string, 0, len(env))
	for k := range env {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		v := strings.TrimSpace(env[k])
		// Shell-safe single-quote quoting to prevent expansion of $, ``, etc.
		b.WriteString("export ")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(singleQuote(v))
		b.WriteString("\n")
	}
	// Separator between exports and script body
	b.WriteString("\n")
	// Functional bootstrap script body
	b.WriteString("set -euo pipefail\n\n")
	b.WriteString("# ploy bootstrap script\n\n")

	// Create directories
	b.WriteString("echo 'Creating ploy directories...'\n")
	b.WriteString("mkdir -p /etc/ploy/pki\n\n")

	// Write CA cert if provided via environment
	b.WriteString("if [ -n \"${PLOY_CA_CERT_PEM:-}\" ]; then\n")
	b.WriteString("  echo \"$PLOY_CA_CERT_PEM\" > /etc/ploy/pki/ca.crt\n")
	b.WriteString("  chmod 644 /etc/ploy/pki/ca.crt\n")
	b.WriteString("fi\n\n")

	// Leaf certificate and key are written later in primary/non-primary branches
	// to match correct file names (server.* vs node.*).

	// PostgreSQL installation
	b.WriteString("if [ \"${PLOY_INSTALL_POSTGRESQL:-false}\" = \"true\" ]; then\n")
	b.WriteString("  echo 'Installing PostgreSQL...'\n")
	b.WriteString("  if command -v apt-get >/dev/null 2>&1; then\n")
	b.WriteString("    export DEBIAN_FRONTEND=noninteractive\n")
	b.WriteString("    apt-get update -qq\n")
	b.WriteString("    apt-get install -y -qq postgresql postgresql-contrib\n")
	b.WriteString("  elif command -v yum >/dev/null 2>&1; then\n")
	b.WriteString("    yum install -y -q postgresql-server postgresql-contrib\n")
	b.WriteString("    postgresql-setup --initdb --unit postgresql\n")
	b.WriteString("  else\n")
	b.WriteString("    echo 'Error: Unsupported package manager. Only apt-get and yum are supported.' >&2\n")
	b.WriteString("    exit 1\n")
	b.WriteString("  fi\n")
	b.WriteString("  systemctl enable postgresql\n")
	b.WriteString("  systemctl start postgresql\n\n")

	// Create database and user
	b.WriteString("  echo 'Creating ploy database and user...'\n")
	b.WriteString("  PLOY_DB_PASSWORD=\"$(openssl rand -hex 16)\"\n")
	b.WriteString("  sudo -u postgres psql -c \"CREATE USER ploy WITH PASSWORD '$PLOY_DB_PASSWORD';\" || true\n")
	b.WriteString("  sudo -u postgres psql -c \"CREATE DATABASE ploy OWNER ploy;\" || true\n")
	b.WriteString("  sudo -u postgres psql -c \"GRANT ALL PRIVILEGES ON DATABASE ploy TO ploy;\" || true\n\n")

	// Derive DSN
	b.WriteString("  export PLOY_SERVER_PG_DSN=\"postgres://ploy:${PLOY_DB_PASSWORD}@localhost:5432/ploy?sslmode=disable\"\n")
	b.WriteString("  echo 'PostgreSQL configured successfully.'\n")
	b.WriteString("fi\n\n")

	// Write server config if this is a primary bootstrap
	// Note: BOOTSTRAP_PRIMARY (without PLOY_ prefix) is the canonical toggle per docs/envs.
	b.WriteString("if [ \"${BOOTSTRAP_PRIMARY:-false}\" = \"true\" ]; then\n")
	b.WriteString("  echo 'Writing server certificate and configuration...'\n")
	b.WriteString("  if [ -n \"${PLOY_SERVER_CERT_PEM:-}\" ]; then\n")
	b.WriteString("    echo \"$PLOY_SERVER_CERT_PEM\" > /etc/ploy/pki/server.crt\n")
	b.WriteString("    chmod 644 /etc/ploy/pki/server.crt\n")
	b.WriteString("  fi\n")
	b.WriteString("  if [ -n \"${PLOY_SERVER_KEY_PEM:-}\" ]; then\n")
	b.WriteString("    echo \"$PLOY_SERVER_KEY_PEM\" > /etc/ploy/pki/server.key\n")
	b.WriteString("    chmod 600 /etc/ploy/pki/server.key\n")
	b.WriteString("  fi\n")
	b.WriteString("  cat > /etc/ploy/ployd.yaml <<'EOF'\n")
	b.WriteString("http:\n")
	b.WriteString("  listen: :8443\n")
	b.WriteString("  tls:\n")
	b.WriteString("    enabled: true\n")
	b.WriteString("    cert: /etc/ploy/pki/server.crt\n")
	b.WriteString("    key: /etc/ploy/pki/server.key\n")
	b.WriteString("    client_ca: /etc/ploy/pki/ca.crt\n")
	b.WriteString("    require_client_cert: true\n")
	b.WriteString("metrics:\n")
	b.WriteString("  listen: :9100\n")
	b.WriteString("control_plane:\n")
	b.WriteString("  endpoint: https://127.0.0.1:8443\n")
	b.WriteString("  ca: /etc/ploy/pki/ca.crt\n")
	b.WriteString("  certificate: /etc/ploy/pki/server.crt\n")
	b.WriteString("  key: /etc/ploy/pki/server.key\n")
	b.WriteString("postgres:\n")
	b.WriteString("  dsn: ${PLOY_SERVER_PG_DSN}\n")
	b.WriteString("EOF\n\n")

	// Install systemd unit for server
	b.WriteString("  echo 'Installing ployd systemd unit...'\n")
	b.WriteString("  cat > /etc/systemd/system/ployd.service <<'EOF'\n")
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Ploy Server\n")
	b.WriteString("After=network.target postgresql.service\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	b.WriteString("ExecStart=/usr/local/bin/ployd\n")
	b.WriteString("Restart=always\n")
	b.WriteString("RestartSec=5\n")
	b.WriteString("User=root\n")
	b.WriteString("Environment=PLOYD_CONFIG_PATH=/etc/ploy/ployd.yaml\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	b.WriteString("EOF\n\n")

	b.WriteString("  systemctl daemon-reload\n")
	b.WriteString("  systemctl enable ployd.service\n")
	b.WriteString("  systemctl start ployd.service\n")
	b.WriteString("  echo 'Server configuration: /etc/ploy/ployd.yaml'\n")
	b.WriteString("  echo 'PKI directory: /etc/ploy/pki'\n")
	b.WriteString("  echo 'Service: ployd.service (active)'\n")

	// Write node config if this is NOT a primary bootstrap
	b.WriteString("else\n")
	b.WriteString("  echo 'Writing node certificate and configuration...'\n")
	b.WriteString("  if [ -n \"${PLOY_SERVER_CERT_PEM:-}\" ]; then\n")
	b.WriteString("    echo \"$PLOY_SERVER_CERT_PEM\" > /etc/ploy/pki/node.crt\n")
	b.WriteString("    chmod 644 /etc/ploy/pki/node.crt\n")
	b.WriteString("  fi\n")
	b.WriteString("  if [ -n \"${PLOY_SERVER_KEY_PEM:-}\" ]; then\n")
	b.WriteString("    echo \"$PLOY_SERVER_KEY_PEM\" > /etc/ploy/pki/node.key\n")
	b.WriteString("    chmod 600 /etc/ploy/pki/node.key\n")
	b.WriteString("  fi\n")
	b.WriteString("  cat > /etc/ploy/ployd-node.yaml <<'EOF'\n")
	b.WriteString("server_url: ${PLOY_SERVER_URL:-}\n")
	b.WriteString("node_id: ${NODE_ID:-}\n")
	b.WriteString("http:\n")
	b.WriteString("  listen: :8444\n")
	b.WriteString("  tls:\n")
	b.WriteString("    enabled: true\n")
	b.WriteString("    ca_path: /etc/ploy/pki/ca.crt\n")
	b.WriteString("    cert_path: /etc/ploy/pki/node.crt\n")
	b.WriteString("    key_path: /etc/ploy/pki/node.key\n")
	b.WriteString("heartbeat:\n")
	b.WriteString("  interval: 30s\n")
	b.WriteString("  timeout: 10s\n")
	b.WriteString("EOF\n\n")

	// Install systemd unit for node
	b.WriteString("  echo 'Installing ployd-node systemd unit...'\n")
	b.WriteString("  cat > /etc/systemd/system/ployd-node.service <<'EOF'\n")
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Ploy Node Agent\n")
	b.WriteString("After=network.target\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	b.WriteString("ExecStart=/usr/local/bin/ployd-node\n")
	b.WriteString("Restart=always\n")
	b.WriteString("RestartSec=5\n")
	b.WriteString("User=root\n")
	// No environment override for node config path; see docs/envs/README.md.
	// The node reads config path from the --config flag (default /etc/ploy/ployd-node.yaml).
	// Avoid setting a non-existent env knob to keep parity with docs.
	// (Server uses PLOYD_CONFIG_PATH; node does not have an equivalent yet.)
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	b.WriteString("EOF\n\n")

	b.WriteString("  systemctl daemon-reload\n")
	b.WriteString("  systemctl enable ployd-node.service\n")
	b.WriteString("  systemctl start ployd-node.service\n")
	b.WriteString("  echo 'Node configuration: /etc/ploy/ployd-node.yaml'\n")
	b.WriteString("  echo 'PKI directory: /etc/ploy/pki'\n")
	b.WriteString("  echo 'Service: ployd-node.service (active)'\n")
	b.WriteString("fi\n\n")

	b.WriteString("echo 'Bootstrap completed successfully.'\n")
	return b.String()
}

// singleQuote returns a shell-safe single-quoted literal. Empty becomes ”.
func singleQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
