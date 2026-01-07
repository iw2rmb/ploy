package nodeagent

import (
	"log/slog"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// injectHealingEnvVars adds healing-specific environment variables to the manifest.
// These variables provide Build Gate API access configuration to healing containers.
//
// This is a shared helper used by both:
//   - Inline healing: runGateWithHealing in execution_healing.go
//   - Discrete healing jobs: executeHealingJob in execution_orchestrator.go
//
// Injected environment variables:
//   - PLOY_HOST_WORKSPACE: host filesystem path to workspace for in-container tooling
//   - PLOY_SERVER_URL: control plane server URL for API calls
//   - PLOY_CA_CERT_PATH, PLOY_CLIENT_CERT_PATH, PLOY_CLIENT_KEY_PATH: in-container paths
//     where TLS certificates will be mounted (see mountHealingTLSCerts)
//   - PLOY_API_TOKEN: bearer token for API authentication (from env or file fallback)
func (r *runController) injectHealingEnvVars(manifest *contracts.StepManifest, workspace string) {
	if manifest.Env == nil {
		manifest.Env = map[string]string{}
	}
	manifest.Env["PLOY_HOST_WORKSPACE"] = workspace
	manifest.Env["PLOY_SERVER_URL"] = r.cfg.ServerURL
	manifest.Env["PLOY_CA_CERT_PATH"] = "/etc/ploy/certs/ca.crt"
	manifest.Env["PLOY_CLIENT_CERT_PATH"] = "/etc/ploy/certs/client.crt"
	manifest.Env["PLOY_CLIENT_KEY_PATH"] = "/etc/ploy/certs/client.key"

	// Inject API token from environment or file fallback.
	if token := os.Getenv("PLOY_API_TOKEN"); token != "" {
		manifest.Env["PLOY_API_TOKEN"] = token
	} else if !r.cfg.HTTP.TLS.Enabled {
		if data, err := os.ReadFile(bearerTokenPath()); err == nil {
			if token := strings.TrimSpace(string(data)); token != "" {
				manifest.Env["PLOY_API_TOKEN"] = token
			}
		} else {
			slog.Warn("healing: failed to read bearer token for PLOY_API_TOKEN fallback", "error", err)
		}
	}
}

// mountHealingTLSCerts configures TLS certificate paths in manifest options.
// This enables healing containers to access the Build Gate API over mTLS.
//
// This is a shared helper used by both:
//   - Inline healing: runGateWithHealing in execution_healing.go
//   - Discrete healing jobs: executeHealingJob in execution_orchestrator.go
//
// The options set here (ploy_ca_cert_path, ploy_client_cert_path, ploy_client_key_path)
// are read by the container runtime to mount the node's TLS certificates into healing
// containers at the paths specified by the PLOY_*_CERT_PATH environment variables.
func (r *runController) mountHealingTLSCerts(manifest *contracts.StepManifest) {
	if manifest.Options == nil {
		manifest.Options = make(map[string]any)
	}
	manifest.Options["ploy_ca_cert_path"] = r.cfg.HTTP.TLS.CAPath
	manifest.Options["ploy_client_cert_path"] = r.cfg.HTTP.TLS.CertPath
	manifest.Options["ploy_client_key_path"] = r.cfg.HTTP.TLS.KeyPath
}
