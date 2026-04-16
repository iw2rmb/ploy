package nodeagent

import (
	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// injectHealingEnvVars adds recovery-job environment variables to the manifest.
func (r *runController) injectHealingEnvVars(manifest *contracts.StepManifest, workspace string, jobID types.JobID) {
	r.injectChildBuildRuntimeEnvVars(manifest, workspace, jobID)
}

// mountHealingTLSCerts configures TLS certificate paths in manifest options.
func (r *runController) mountHealingTLSCerts(manifest *contracts.StepManifest) {
	r.mountChildBuildTLSCerts(manifest)
}
