package bootstrap

import (
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

func TestScriptBindsHTTPToLoopback(t *testing.T) {
	script := Script()
	if !strings.Contains(script, "PLOYD_HTTP_LISTEN:-127.0.0.1:8443") {
		t.Fatalf("ployd config must default HTTP listen to loopback")
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
