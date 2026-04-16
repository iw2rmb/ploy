package nodeagent

import (
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestInjectHealingEnvVars(t *testing.T) {
	rc := &runController{cfg: Config{ServerURL: "https://ploy.example", HTTP: HTTPConfig{TLS: TLSConfig{Enabled: true}}}}
	t.Setenv("PLOY_API_TOKEN", "token")
	manifest := &contracts.StepManifest{}

	rc.injectHealingEnvVars(manifest, "/tmp/heal-workspace", types.JobID("job-heal-1"))

	if got := manifest.Envs["PLOY_JOB_ID"]; got != "job-heal-1" {
		t.Fatalf("PLOY_JOB_ID=%q want job-heal-1", got)
	}
	if got := manifest.Envs["PLOY_SERVER_URL"]; got != "https://ploy.example" {
		t.Fatalf("PLOY_SERVER_URL=%q want https://ploy.example", got)
	}
	if got := manifest.Envs["PLOY_HOST_WORKSPACE"]; got != "/tmp/heal-workspace" {
		t.Fatalf("PLOY_HOST_WORKSPACE=%q want /tmp/heal-workspace", got)
	}
}

func TestMountHealingTLSCerts(t *testing.T) {
	t.Parallel()

	rc := &runController{cfg: Config{HTTP: HTTPConfig{TLS: TLSConfig{
		CAPath:   "/tls/ca.pem",
		CertPath: "/tls/client.crt",
		KeyPath:  "/tls/client.key",
	}}}}
	manifest := &contracts.StepManifest{}

	rc.mountHealingTLSCerts(manifest)

	if got := manifest.Options["ploy_ca_cert_path"]; got != "/tls/ca.pem" {
		t.Fatalf("ploy_ca_cert_path=%v want /tls/ca.pem", got)
	}
	if got := manifest.Options["ploy_client_cert_path"]; got != "/tls/client.crt" {
		t.Fatalf("ploy_client_cert_path=%v want /tls/client.crt", got)
	}
	if got := manifest.Options["ploy_client_key_path"]; got != "/tls/client.key" {
		t.Fatalf("ploy_client_key_path=%v want /tls/client.key", got)
	}
}
