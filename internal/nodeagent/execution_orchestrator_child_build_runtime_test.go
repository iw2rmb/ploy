package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestInjectChildBuildRuntimeEnvVars(t *testing.T) {
	rc := &runController{cfg: Config{ServerURL: "https://ploy.example", HTTP: HTTPConfig{TLS: TLSConfig{Enabled: true}}}}
	t.Setenv("PLOY_API_TOKEN", "token-from-env")

	manifest := &contracts.StepManifest{Envs: map[string]string{"EXISTING": "keep"}}
	rc.injectChildBuildRuntimeEnvVars(manifest, "/tmp/workspace", types.JobID("job-123"))

	if got := manifest.Envs["EXISTING"]; got != "keep" {
		t.Fatalf("EXISTING=%q want keep", got)
	}
	if got := manifest.Envs["PLOY_SERVER_URL"]; got != "https://ploy.example" {
		t.Fatalf("PLOY_SERVER_URL=%q want https://ploy.example", got)
	}
	if got := manifest.Envs["PLOY_HOST_WORKSPACE"]; got != "/tmp/workspace" {
		t.Fatalf("PLOY_HOST_WORKSPACE=%q want /tmp/workspace", got)
	}
	if got := manifest.Envs["PLOY_JOB_ID"]; got != "job-123" {
		t.Fatalf("PLOY_JOB_ID=%q want job-123", got)
	}
	if got := manifest.Envs["PLOY_API_TOKEN"]; got != "token-from-env" {
		t.Fatalf("PLOY_API_TOKEN=%q want token-from-env", got)
	}
	if got := manifest.Envs["PLOY_CLIENT_CERT_PATH"]; got != "/etc/ploy/certs/client.crt" {
		t.Fatalf("PLOY_CLIENT_CERT_PATH=%q want /etc/ploy/certs/client.crt", got)
	}
	if got := manifest.Envs["PLOY_CLIENT_KEY_PATH"]; got != "/etc/ploy/certs/client.key" {
		t.Fatalf("PLOY_CLIENT_KEY_PATH=%q want /etc/ploy/certs/client.key", got)
	}
}

func TestInjectChildBuildRuntimeEnvVars_BearerTokenFallback(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "bearer-token")
	if err := os.WriteFile(tokenPath, []byte("fallback-token\n"), 0o644); err != nil {
		t.Fatalf("write bearer token: %v", err)
	}
	t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenPath)
	t.Setenv("PLOY_API_TOKEN", "")

	rc := &runController{cfg: Config{ServerURL: "http://ploy.local", HTTP: HTTPConfig{TLS: TLSConfig{Enabled: false}}}}
	manifest := &contracts.StepManifest{}
	rc.injectChildBuildRuntimeEnvVars(manifest, "/workspace", types.JobID("job-456"))

	if got := manifest.Envs["PLOY_API_TOKEN"]; got != "fallback-token" {
		t.Fatalf("PLOY_API_TOKEN=%q want fallback-token", got)
	}
}

func TestMaterializeParentChildBuildLineage(t *testing.T) {
	t.Parallel()

	rc := &runController{}
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "re_build-gate-1.log"), []byte("child\n"), 0o644); err != nil {
		t.Fatalf("write re_build-gate-1.log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "errors-1.yaml"), []byte("step: child\n"), 0o644); err != nil {
		t.Fatalf("write errors-1.yaml: %v", err)
	}

	recoveryCtx := &contracts.RecoveryClaimContext{
		BuildGateLog: "baseline\n",
		Errors:       json.RawMessage(`{"task":"baseline"}`),
	}
	if err := rc.materializeParentChildBuildLineage(outDir, recoveryCtx); err != nil {
		t.Fatalf("materializeParentChildBuildLineage() error: %v", err)
	}

	if got := mustReadString(t, filepath.Join(outDir, "re_build-gate-1.log")); got != "baseline\n" {
		t.Fatalf("re_build-gate-1.log=%q want baseline", got)
	}
	if got := mustReadString(t, filepath.Join(outDir, "re_build-gate-2.log")); got != "child\n" {
		t.Fatalf("re_build-gate-2.log=%q want child", got)
	}
}

func mustReadString(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
