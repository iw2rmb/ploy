package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/deploy"
)

func TestInstallWorkerArtifactsUploadsCertificatesAndRewritesConfig(t *testing.T) {
	ca := "-----BEGIN CERTIFICATE-----\nMIIC\n-----END CERTIFICATE-----"
	cert := "-----BEGIN CERTIFICATE-----\nMIID\n-----END CERTIFICATE-----"
	key := "-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----"
	cfg := workerProvisionConfig{
		WorkerAddress:   "198.51.100.20",
		User:            "ploy",
		IdentityFile:    "/home/ploy/.ssh/id_worker",
		SSHPort:         3022,
		ControlPlaneURL: "http://cp.internal:9094",
	}
	result := nodeJoinResponse{
		Certificate: deploy.LeafCertificate{
			CertificatePEM: cert,
			KeyPEM:         key,
		},
		CABundle: ca,
	}

	fake := &fakeRemoteOps{}
	origCmd := remoteCommandExecutor
	origWrite := remoteFileWriter
	remoteCommandExecutor = fake.command
	remoteFileWriter = fake.write
	t.Cleanup(func() {
		remoteCommandExecutor = origCmd
		remoteFileWriter = origWrite
	})

	if err := installWorkerArtifacts(cfg, result, io.Discard); err != nil {
		t.Fatalf("installWorkerArtifacts: %v", err)
	}

	if len(fake.commandCalls) != 3 {
		t.Fatalf("expected 3 remote commands, got %d", len(fake.commandCalls))
	}
	first := fake.commandCalls[0]
	if first.target != "ploy@198.51.100.20" {
		t.Fatalf("expected target ploy@198.51.100.20, got %s", first.target)
	}
	if got := flagValue(first.args, "-i"); got != cfg.IdentityFile {
		t.Fatalf("expected ssh identity %s, got %s", cfg.IdentityFile, got)
	}
	if got := flagValue(first.args, "-p"); got != "3022" {
		t.Fatalf("expected ssh port 3022, got %s", got)
	}
	if first.command != "mkdir -p /etc/ploy/pki && chmod 700 /etc/ploy/pki" {
		t.Fatalf("unexpected mkdir command: %s", first.command)
	}
	second := fake.commandCalls[1]
	if !strings.Contains(second.command, "https://cp.internal:9094") {
		t.Fatalf("expected HTTPS endpoint rewrite, got %s", second.command)
	}
	if !strings.Contains(second.command, remoteConfigPath) {
		t.Fatalf("expected config rewrite to touch %s, got %s", remoteConfigPath, second.command)
	}
	third := fake.commandCalls[2]
	if third.command != "systemctl restart ployd" {
		t.Fatalf("expected ployd restart, got %s", third.command)
	}

	if len(fake.fileCalls) != 3 {
		t.Fatalf("expected 3 remote file uploads, got %d", len(fake.fileCalls))
	}
	files := map[string]remoteFileCall{}
	for _, call := range fake.fileCalls {
		files[call.path] = call
		if call.target != "ploy@198.51.100.20" {
			t.Fatalf("expected uploads to target ploy@198.51.100.20, got %s", call.target)
		}
	}
	if files[remoteControlPlaneCAPath].data != ca || files[remoteControlPlaneCAPath].mode != 0o644 {
		t.Fatalf("unexpected CA upload: %+v", files[remoteControlPlaneCAPath])
	}
	if files[remoteNodeCertPath].data != cert || files[remoteNodeCertPath].mode != 0o644 {
		t.Fatalf("unexpected cert upload: %+v", files[remoteNodeCertPath])
	}
	if files[remoteNodeKeyPath].data != key || files[remoteNodeKeyPath].mode != 0o600 {
		t.Fatalf("unexpected key upload: %+v", files[remoteNodeKeyPath])
	}
}

func TestInstallWorkerArtifactsReturnsErrorOnCommandFailure(t *testing.T) {
	origCmd := remoteCommandExecutor
	origWrite := remoteFileWriter
	defer func() {
		remoteCommandExecutor = origCmd
		remoteFileWriter = origWrite
	}()
	remoteFileWriter = func(context.Context, string, []string, string, os.FileMode, []byte, io.Writer) error {
		return nil
	}
	remoteCommandExecutor = func(ctx context.Context, target string, sshArgs []string, command string, stdin io.Reader, stdout, stderr io.Writer) error {
		if strings.Contains(command, "mkdir -p") {
			return errors.New("ssh failed")
		}
		return nil
	}

	cfg := workerProvisionConfig{
		WorkerAddress: "10.0.0.5",
	}
	result := nodeJoinResponse{
		Certificate: deploy.LeafCertificate{
			CertificatePEM: "CERT",
			KeyPEM:         "KEY",
		},
		CABundle: "CA",
	}
	err := installWorkerArtifacts(cfg, result, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "prepare remote pki dir") {
		t.Fatalf("expected prepare remote pki dir error, got %v", err)
	}
}
