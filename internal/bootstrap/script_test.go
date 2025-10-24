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
	if !strings.Contains(script, "AllowUsers ploy-admin ploy-user") {
		t.Fatalf("bootstrap script must restrict SSH access to ploy-admin and ploy-user")
	}
	if !strings.Contains(script, "Match User ploy-admin") {
		t.Fatalf("bootstrap script must match admin user for authorized keys")
	}
	if !strings.Contains(script, "AuthorizedKeysFile /etc/ploy/ssh/admin_authorized_keys") {
		t.Fatalf("bootstrap script must reference admin authorized keys file")
	}
	if !strings.Contains(script, "Match User ploy-user") {
		t.Fatalf("bootstrap script must match user role for authorized keys")
	}
	if !strings.Contains(script, "AuthorizedKeysFile /etc/ploy/ssh/user_authorized_keys") {
		t.Fatalf("bootstrap script must reference user authorized keys file")
	}
	if !strings.Contains(script, "LogLevel VERBOSE") {
		t.Fatalf("bootstrap script must enable verbose sshd logging for telemetry")
	}
}

func TestScriptDecodesAuthorizedKeysPayloads(t *testing.T) {
	script := Script()
	if !strings.Contains(script, "base64 --decode") {
		t.Fatalf("bootstrap script should decode base64-encoded authorized keys")
	}
	if !strings.Contains(script, "/etc/ploy/ssh/admin_authorized_keys") {
		t.Fatalf("bootstrap script must write admin authorized keys")
	}
	if !strings.Contains(script, "/etc/ploy/ssh/user_authorized_keys") {
		t.Fatalf("bootstrap script must write user authorized keys")
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
