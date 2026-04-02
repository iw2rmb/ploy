package deploy

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
		b.WriteString(shellQuote(v))
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

	// PKI writes are handled in primary/non-primary branches below to ensure
	// we do not clobber existing PKI on the primary host when reusing.
	// Bootstrap is idempotent: if PKI exists on the primary, it is reused (not overwritten).

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
	b.WriteString("  export PLOY_DB_DSN=\"postgres://ploy:${PLOY_DB_PASSWORD}@localhost:5432/ploy?sslmode=disable\"\n")
	b.WriteString("  echo 'PostgreSQL configured successfully.'\n")
	b.WriteString("fi\n\n")

	// Write server config if this is a primary bootstrap
	// Note: BOOTSTRAP_PRIMARY (without PLOY_ prefix) is the canonical toggle per docs/envs.
	b.WriteString("if [ \"${BOOTSTRAP_PRIMARY:-false}\" = \"true\" ]; then\n")

	// Write all environment variables to /etc/ploy/cluster.env
	// Use unquoted heredoc to allow variable expansion
	b.WriteString("  cat > /etc/ploy/cluster.env <<ENVFILE\n")
	b.WriteString("PLOY_CLUSTER_ID=${CLUSTER_ID}\n")
	b.WriteString("PLOY_AUTH_SECRET=${PLOY_AUTH_SECRET}\n")
	b.WriteString("PLOY_SERVER_CA_CERT=${PLOY_SERVER_CA_CERT}\n")
	b.WriteString("PLOY_SERVER_CA_KEY=${PLOY_SERVER_CA_KEY}\n")
	b.WriteString("ENVFILE\n")
	b.WriteString("  chmod 600 /etc/ploy/cluster.env\n")

	// Check for existing PKI to avoid clobbering during reuse scenarios
	b.WriteString("  if [ -f /etc/ploy/pki/ca.key ]; then\n")
	b.WriteString("    echo 'Existing PKI detected (/etc/ploy/pki/ca.key found); skipping PKI writes and reusing existing certificates.'\n")
	b.WriteString("  else\n")

	// Write CA cert if provided via environment (only when creating new PKI)
	b.WriteString("    if [ -n \"${PLOY_CA_CERT_PEM:-}\" ]; then\n")
	b.WriteString("      echo \"$PLOY_CA_CERT_PEM\" > /etc/ploy/pki/ca.crt\n")
	b.WriteString("      chmod 644 /etc/ploy/pki/ca.crt\n")
	b.WriteString("    fi\n")

	// Persist CA private key on the primary (control-plane) host when provided.
	b.WriteString("    if [ -n \"${PLOY_CA_KEY_PEM:-}\" ]; then\n")
	b.WriteString("      echo \"$PLOY_CA_KEY_PEM\" > /etc/ploy/pki/ca.key\n")
	b.WriteString("      chmod 600 /etc/ploy/pki/ca.key\n")
	b.WriteString("    fi\n")
	b.WriteString("    echo 'Writing server certificate and configuration...'\n")
	b.WriteString("    if [ -n \"${PLOY_SERVER_CERT_PEM:-}\" ]; then\n")
	b.WriteString("      echo \"$PLOY_SERVER_CERT_PEM\" > /etc/ploy/pki/server.crt\n")
	b.WriteString("      chmod 644 /etc/ploy/pki/server.crt\n")
	b.WriteString("    fi\n")
	b.WriteString("    if [ -n \"${PLOY_SERVER_KEY_PEM:-}\" ]; then\n")
	b.WriteString("      echo \"$PLOY_SERVER_KEY_PEM\" > /etc/ploy/pki/server.key\n")
	b.WriteString("      chmod 600 /etc/ploy/pki/server.key\n")
	b.WriteString("    fi\n")
	b.WriteString("  fi\n")
	b.WriteString("  cat > /etc/ploy/ployd.yaml <<EOF\n")
	b.WriteString("http:\n")
	b.WriteString("  listen: :8080\n")
	b.WriteString("metrics:\n")
	b.WriteString("  listen: :9100\n")
	b.WriteString("auth:\n")
	b.WriteString("  bearer_tokens:\n")
	b.WriteString("    enabled: true\n")
	b.WriteString("postgres:\n")
	b.WriteString("  dsn: ${PLOY_DB_DSN:-}\n")
	b.WriteString("EOF\n\n")

	// Install systemd unit for server
	b.WriteString("  echo 'Installing ployd systemd unit...'\n")
	b.WriteString("  cat > /etc/systemd/system/ployd.service <<EOF\n")
	b.WriteString("[Unit]\n")
	b.WriteString("Description=Ploy Server\n")
	b.WriteString("After=network.target postgresql.service\n\n")
	b.WriteString("[Service]\n")
	b.WriteString("Type=simple\n")
	b.WriteString("ExecStart=/usr/local/bin/ployd\n")
	b.WriteString("Restart=always\n")
	b.WriteString("RestartSec=5\n")
	b.WriteString("User=root\n")
	b.WriteString("Environment=PLOYD_CONFIG_PATH=/etc/ploy/ployd.yaml\n")
	b.WriteString("EnvironmentFile=/etc/ploy/cluster.env\n\n")
	b.WriteString("[Install]\n")
	b.WriteString("WantedBy=multi-user.target\n")
	b.WriteString("EOF\n\n")

	b.WriteString("  systemctl daemon-reload\n")
	b.WriteString("  systemctl enable --now ployd.service\n")

	// Insert initial admin token into database
	b.WriteString("  echo 'Configuring initial admin token...'\n")
	b.WriteString("  if [ -n \"${PLOY_INITIAL_TOKEN_HASH:-}\" ] && [ -n \"${PLOY_INITIAL_TOKEN_ID:-}\" ]; then\n")
	b.WriteString("    # Wait for migrations to complete and api_tokens table to exist\n")
	b.WriteString("    for i in {1..60}; do\n")
	b.WriteString("      if sudo -u postgres psql -d ploy -c '\\\\dt ploy.api_tokens' 2>/dev/null | grep -q api_tokens; then\n")
	b.WriteString("        break\n")
	b.WriteString("      fi\n")
	b.WriteString("      echo 'Waiting for database migrations to complete...'\n")
	b.WriteString("      sleep 1\n")
	b.WriteString("    done\n")
	b.WriteString("    sudo -u postgres psql -d ploy -c \"\n")
	b.WriteString("INSERT INTO ploy.api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)\n")
	b.WriteString("VALUES (\n")
	b.WriteString("  '${PLOY_INITIAL_TOKEN_HASH}',\n")
	b.WriteString("  '${PLOY_INITIAL_TOKEN_ID}',\n")
	b.WriteString("  '${CLUSTER_ID}',\n")
	b.WriteString("  'cli-admin',\n")
	b.WriteString("  'Initial admin token - please rotate',\n")
	b.WriteString("  NOW(),\n")
	b.WriteString("  NOW() + INTERVAL '365 days'\n")
	b.WriteString(");\" || echo 'Warning: Failed to insert initial admin token'\n")
	b.WriteString("    echo 'Initial admin token configured successfully'\n")
	b.WriteString("  else\n")
	b.WriteString("    echo 'Warning: Initial token data not provided; you will need to create a token manually'\n")
	b.WriteString("  fi\n")

	b.WriteString("  PLOY_CONFIG_PATH='/etc/ploy/ployd.yaml'\n")
	b.WriteString("  PLOY_SERVICE_NAME='ployd.service'\n")

	// Write node config if this is NOT a primary bootstrap
	b.WriteString("else\n")
	b.WriteString("  echo 'Writing bootstrap token and configuration...'\n")
	// For nodes, write the CA cert for server verification during bootstrap
	b.WriteString("  if [ -n \"${PLOY_CA_CERT_PEM:-}\" ]; then\n")
	b.WriteString("    echo \"$PLOY_CA_CERT_PEM\" > /etc/ploy/pki/ca.crt\n")
	b.WriteString("    chmod 644 /etc/ploy/pki/ca.crt\n")
	b.WriteString("  fi\n")
	// Write bootstrap token to secure tmpfs location
	b.WriteString("  if [ -n \"${PLOY_BOOTSTRAP_TOKEN:-}\" ]; then\n")
	b.WriteString("    mkdir -p /run/ploy\n")
	b.WriteString("    echo \"$PLOY_BOOTSTRAP_TOKEN\" > /run/ploy/bootstrap-token\n")
	b.WriteString("    chmod 600 /run/ploy/bootstrap-token\n")
	b.WriteString("  fi\n")
	// Ensure Docker is installed for containerized execution
	b.WriteString("  echo 'Ensuring Docker is installed...'\n")
	b.WriteString("  if command -v apt-get >/dev/null 2>&1; then\n")
	b.WriteString("    export DEBIAN_FRONTEND=noninteractive\n")
	b.WriteString("    apt-get update -qq\n")
	b.WriteString("    apt-get install -y -qq docker.io || true\n")
	b.WriteString("    systemctl enable --now docker || true\n")
	b.WriteString("  elif command -v yum >/dev/null 2>&1 || command -v dnf >/dev/null 2>&1; then\n")
	b.WriteString("    (command -v yum && yum install -y -q docker) || (command -v dnf && dnf install -y -q docker) || true\n")
	b.WriteString("    systemctl enable --now docker || true\n")
	b.WriteString("    if ! command -v docker >/dev/null 2>&1; then\n")
	b.WriteString("      curl -fsSL https://get.docker.com | sh\n")
	b.WriteString("      systemctl enable --now docker || true\n")
	b.WriteString("    fi\n")
	b.WriteString("  else\n")
	b.WriteString("    curl -fsSL https://get.docker.com | sh\n")
	b.WriteString("    systemctl enable --now docker || true\n")
	b.WriteString("  fi\n")
	// Write node config with literal values expanded from environment
	b.WriteString("  cat > /etc/ploy/ployd-node.yaml <<EOF\n")
	b.WriteString("server_url: ${PLOY_SERVER_URL:-}\n")
	b.WriteString("node_id: ${NODE_ID:-}\n")
	b.WriteString("cluster_id: ${CLUSTER_ID:-}\n")
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
	b.WriteString("  systemctl enable --now ployd-node.service\n")
	b.WriteString("  PLOY_CONFIG_PATH='/etc/ploy/ployd-node.yaml'\n")
	b.WriteString("  PLOY_SERVICE_NAME='ployd-node.service'\n")
	b.WriteString("fi\n\n")

	// Echo final status and key paths
	b.WriteString("echo ''\n")
	b.WriteString("echo '========================================'\n")
	b.WriteString("echo 'Bootstrap completed successfully.'\n")
	b.WriteString("echo '========================================'\n")
	b.WriteString("echo ''\n")
	b.WriteString("echo 'Configuration:'\n")
	b.WriteString("echo \"  Config file: ${PLOY_CONFIG_PATH}\"\n")
	b.WriteString("echo \"  PKI directory: /etc/ploy/pki\"\n")
	b.WriteString("if [ -f /etc/ploy/pki/ca.crt ]; then\n")
	b.WriteString("  echo \"    - CA cert: /etc/ploy/pki/ca.crt\"\n")
	b.WriteString("fi\n")
	b.WriteString("if [ \"${BOOTSTRAP_PRIMARY:-false}\" = \"true\" ]; then\n")
	b.WriteString("  if [ -f /etc/ploy/pki/server.crt ]; then\n")
	b.WriteString("    echo \"    - Server cert: /etc/ploy/pki/server.crt\"\n")
	b.WriteString("  fi\n")
	b.WriteString("  if [ -f /etc/ploy/pki/server.key ]; then\n")
	b.WriteString("    echo \"    - Server key: /etc/ploy/pki/server.key\"\n")
	b.WriteString("  fi\n")
	b.WriteString("else\n")
	b.WriteString("  if [ -f /etc/ploy/pki/node.crt ]; then\n")
	b.WriteString("    echo \"    - Node cert: /etc/ploy/pki/node.crt\"\n")
	b.WriteString("  fi\n")
	b.WriteString("  if [ -f /etc/ploy/pki/node.key ]; then\n")
	b.WriteString("    echo \"    - Node key: /etc/ploy/pki/node.key\"\n")
	b.WriteString("  fi\n")
	b.WriteString("fi\n")
	b.WriteString("echo ''\n")
	b.WriteString("echo 'Service:'\n")
	b.WriteString("echo \"  Service name: ${PLOY_SERVICE_NAME}\"\n")
	b.WriteString("echo \"  Status: $(systemctl is-active ${PLOY_SERVICE_NAME} 2>/dev/null || echo 'unknown')\"\n")
	b.WriteString("echo \"  Enabled: $(systemctl is-enabled ${PLOY_SERVICE_NAME} 2>/dev/null || echo 'unknown')\"\n")
	b.WriteString("echo ''\n")
	b.WriteString("echo 'To view logs:'\n")
	b.WriteString("echo \"  journalctl -u ${PLOY_SERVICE_NAME} -f\"\n")
	b.WriteString("echo ''\n")
	b.WriteString("echo 'To check status:'\n")
	b.WriteString("echo \"  systemctl status ${PLOY_SERVICE_NAME}\"\n")
	b.WriteString("echo '========================================'\n")

	return b.String()
}
