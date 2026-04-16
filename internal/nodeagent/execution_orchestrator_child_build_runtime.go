package nodeagent

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// injectChildBuildRuntimeEnvVars injects runtime context required by child-build
// polling contracts into mig/heal manifests.
func (r *runController) injectChildBuildRuntimeEnvVars(manifest *contracts.StepManifest, workspace string, jobID types.JobID) {
	if manifest.Envs == nil {
		manifest.Envs = map[string]string{}
	}
	manifest.Envs["PLOY_HOST_WORKSPACE"] = workspace
	manifest.Envs["PLOY_SERVER_URL"] = r.cfg.ServerURL
	manifest.Envs["PLOY_JOB_ID"] = strings.TrimSpace(jobID.String())
	// TLS cert files are bind-mounted into the container.
	manifest.Envs["PLOY_CLIENT_CERT_PATH"] = "/etc/ploy/certs/client.crt"
	manifest.Envs["PLOY_CLIENT_KEY_PATH"] = "/etc/ploy/certs/client.key"

	if token := os.Getenv("PLOY_API_TOKEN"); token != "" {
		manifest.Envs["PLOY_API_TOKEN"] = token
		return
	}
	if !r.cfg.HTTP.TLS.Enabled {
		if data, err := os.ReadFile(bearerTokenPath()); err == nil {
			if token := strings.TrimSpace(string(data)); token != "" {
				manifest.Envs["PLOY_API_TOKEN"] = token
			}
		} else {
			slog.Warn("child build runtime: failed to read bearer token for PLOY_API_TOKEN fallback", "error", err)
		}
	}
}

// mountChildBuildTLSCerts configures TLS certificate paths in manifest options.
func (r *runController) mountChildBuildTLSCerts(manifest *contracts.StepManifest) {
	if manifest.Options == nil {
		manifest.Options = make(map[string]any)
	}
	manifest.Options["ploy_ca_cert_path"] = r.cfg.HTTP.TLS.CAPath
	manifest.Options["ploy_client_cert_path"] = r.cfg.HTTP.TLS.CertPath
	manifest.Options["ploy_client_key_path"] = r.cfg.HTTP.TLS.KeyPath
}

func (r *runController) materializeParentChildBuildLineage(outDir string, recoveryCtx *contracts.RecoveryClaimContext) error {
	if err := materializeParentGateLineageArtifacts(outDir, recoveryCtx); err != nil {
		return fmt.Errorf("materialize parent child-build lineage artifacts: %w", err)
	}
	return nil
}
