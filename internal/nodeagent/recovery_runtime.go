package nodeagent

import (
	"log/slog"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// injectHealingEnvVars adds recovery-job environment variables to the manifest.
func (r *runController) injectHealingEnvVars(manifest *contracts.StepManifest, workspace string) {
	if manifest.Envs == nil {
		manifest.Envs = map[string]string{}
	}
	manifest.Envs["PLOY_HOST_WORKSPACE"] = workspace
	manifest.Envs["PLOY_SERVER_URL"] = r.cfg.ServerURL
	// CA certs are delivered via Hydra CA mount entries (mounted at
	// /etc/ploy/ca/<hash>) and TLS certs via certMountOptions; no longer
	// injected as PLOY_CA_CERTS env.
	manifest.Envs["PLOY_CLIENT_CERT_PATH"] = "/etc/ploy/certs/client.crt"
	manifest.Envs["PLOY_CLIENT_KEY_PATH"] = "/etc/ploy/certs/client.key"

	if token := os.Getenv("PLOY_API_TOKEN"); token != "" {
		manifest.Envs["PLOY_API_TOKEN"] = token
	} else if !r.cfg.HTTP.TLS.Enabled {
		if data, err := os.ReadFile(bearerTokenPath()); err == nil {
			if token := strings.TrimSpace(string(data)); token != "" {
				manifest.Envs["PLOY_API_TOKEN"] = token
			}
		} else {
			slog.Warn("recovery: failed to read bearer token for PLOY_API_TOKEN fallback", "error", err)
		}
	}
}

// mountHealingTLSCerts configures TLS certificate paths in manifest options.
func (r *runController) mountHealingTLSCerts(manifest *contracts.StepManifest) {
	if manifest.Options == nil {
		manifest.Options = make(map[string]any)
	}
	manifest.Options["ploy_ca_cert_path"] = r.cfg.HTTP.TLS.CAPath
	manifest.Options["ploy_client_cert_path"] = r.cfg.HTTP.TLS.CertPath
	manifest.Options["ploy_client_key_path"] = r.cfg.HTTP.TLS.KeyPath
}
