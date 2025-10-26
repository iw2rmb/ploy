package bootstrap

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptIncludesPloydService(t *testing.T) {
	script := Script()
	if !strings.Contains(script, "ployd.service") {
		t.Fatalf("bootstrap script missing ployd.service declaration")
	}
	if !strings.Contains(script, "ExecStart=${BIN_DIR}/ployd") {
		t.Fatalf("bootstrap script missing ployd ExecStart line")
	}
}

func TestScriptConfiguresSSHDHardening(t *testing.T) {
	script := Script()
	if !strings.Contains(script, "openssh-server") {
		t.Fatalf("bootstrap script must install openssh-server")
	}
	if !strings.Contains(script, "PasswordAuthentication no") {
		t.Fatalf("bootstrap script must disable password authentication")
	}
	if !strings.Contains(script, "ChallengeResponseAuthentication no") {
		t.Fatalf("bootstrap script must disable challenge-response auth")
	}
	if !strings.Contains(script, "PermitRootLogin prohibit-password") {
		t.Fatalf("bootstrap script must restrict root login")
	}
	if !strings.Contains(script, "AllowUsers root") {
		t.Fatalf("bootstrap script must restrict SSH access to root")
	}
	if !strings.Contains(script, "LogLevel VERBOSE") {
		t.Fatalf("bootstrap script must enable verbose sshd logging for telemetry")
	}
}

func TestScriptDoesNotRequireEmbeddedAuthorizedKeys(t *testing.T) {
	script := Script()
	if strings.Contains(script, "PLOY_SSH_ADMIN_KEYS_B64") || strings.Contains(script, "PLOY_SSH_USER_KEYS_B64") {
		t.Fatalf("bootstrap script should not require PLOY_SSH_* authorized key payloads")
	}
}

func TestScriptConfiguresHTTPDefaults(t *testing.T) {
	script := Script()
	if !strings.Contains(script, "PLOYD_HTTP_LISTEN:-0.0.0.0:8443") {
		t.Fatalf("ployd config must default HTTP listen to 0.0.0.0:8443")
	}
	if !strings.Contains(script, "PLOYD_HTTP_TLS_ENABLED:-false") {
		t.Fatalf("ployd config must default HTTP TLS disabled")
	}
	if !strings.Contains(script, "PLOYD_TLS_CERT_PATH:-/etc/ploy/pki/node.pem") {
		t.Fatalf("ployd config must default TLS cert path")
	}
	if !strings.Contains(script, "PLOYD_HTTP_TLS_REQUIRE_CLIENT_CERT:-false") {
		t.Fatalf("ployd config must default TLS client cert requirement to false")
	}
	if !strings.Contains(script, "PLOYD_METRICS_LISTEN:-127.0.0.1:9100") {
		t.Fatalf("ployd config must default metrics listen to loopback")
	}
}

func TestDockerDaemonJSONBlockTerminatesBeforePloydConfig(t *testing.T) {
	script := Script()
	jsonLog := `log "wrote /etc/docker/daemon.json"`
	idxLog := strings.Index(script, jsonLog)
	if idxLog == -1 {
		t.Fatalf("bootstrap script missing docker daemon.json log marker")
	}
	idxFunc := strings.Index(script, "configure_ployd_service() {")
	if idxFunc == -1 {
		t.Fatalf("bootstrap script missing ployd configuration function")
	}
	if idxLog > idxFunc {
		t.Fatalf("docker daemon.json heredoc terminator/log must appear before ployd configuration block")
	}
}

func TestStopServiceIfActiveTracksRestarts(t *testing.T) {
	t.Parallel()
	scriptPath := writeBootstrapScript(t)
	logPath := filepath.Join(t.TempDir(), "systemctl.log")
	snippet := fmt.Sprintf(`
trap - EXIT
LOG_PATH=%q
SERVICE_STATE="active"
systemctl() {
	local cmd="$1"
	shift || true
	case "$cmd" in
		list-unit-files)
			printf 'docker.service\n'
			;;
		is-active)
			local svc=""
			for arg in "$@"; do
				svc="$arg"
			done
			if [[ "$svc" == "docker" && "$SERVICE_STATE" == "active" ]]; then
				return 0
			fi
			return 1
			;;
		stop)
			SERVICE_STATE="inactive"
			printf 'stop %%s\n' "$1" >>"$LOG_PATH"
			return 0
			;;
		restart)
			SERVICE_STATE="active"
			printf 'restart %%s\n' "$1" >>"$LOG_PATH"
			return 0
			;;
		*)
			return 0
			;;
	esac
}
log() { :; }
warn() { :; }

stop_service_if_active docker
restart_stopped_services
restart_stopped_services
`, logPath)
	runBootstrapSnippet(t, scriptPath, snippet)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected stop+restart entries, got %d: %v", len(lines), lines)
	}
	if lines[0] != "stop docker" {
		t.Fatalf("expected first entry 'stop docker', got %q", lines[0])
	}
	if lines[1] != "restart docker" {
		t.Fatalf("expected second entry 'restart docker', got %q", lines[1])
	}
}

func TestStopServiceIfActiveSkipsUnknownServices(t *testing.T) {
	t.Parallel()
	scriptPath := writeBootstrapScript(t)
	logPath := filepath.Join(t.TempDir(), "systemctl.log")
	snippet := fmt.Sprintf(`
trap - EXIT
LOG_PATH=%q
SERVICE_STATE="inactive"
systemctl() {
	local cmd="$1"
	shift || true
	case "$cmd" in
		list-unit-files)
			printf 'etcd.service\n'
			;;
		is-active)
			return 1
			;;
		stop|restart)
			printf '%%s %%s\n' "$cmd" "$1" >>"$LOG_PATH"
			return 0
			;;
		*)
			return 0
			;;
	esac
}
log() { :; }
warn() { :; }

stop_service_if_active docker
restart_stopped_services
`, logPath)
	runBootstrapSnippet(t, scriptPath, snippet)
	data, err := os.ReadFile(logPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read log: %v", err)
	}
	if len(bytes.TrimSpace(data)) != 0 {
		t.Fatalf("expected no stop/restart entries, got %q", string(data))
	}
}

func writeBootstrapScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bootstrap.sh")
	if err := os.WriteFile(path, []byte(Script()), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func runBootstrapSnippet(t *testing.T, scriptPath, snippet string) {
	t.Helper()
	cmd := exec.Command("bash", "-c", fmt.Sprintf(`set -euo pipefail
source %q
%s
`, scriptPath, snippet))
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("snippet failed: %v\noutput:\n%s", err, buf.String())
	}
}
