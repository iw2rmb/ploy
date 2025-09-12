package transflow

import (
	"os"
	"path/filepath"
)

// computeBranchDiffKey returns artifacts key for a branch step diff.
// Format: transflow/<execID>/branches/<branchID>/steps/<stepID>/diff.patch
func computeBranchDiffKey(execID, branchID, stepID string) string {
	return "transflow/" + execID + "/branches/" + branchID + "/steps/" + stepID + "/diff.patch"
}

// ensureOutDir ensures the out directory exists under the base dir and returns its path.
func ensureOutDir(baseDir string) string {
	outDir := filepath.Join(baseDir, "out")
	_ = os.MkdirAll(outDir, 0755)
	return outDir
}

// makeORWVars builds the substitution variables for ORW apply HCL templates.
func makeORWVars(baseDir, execID, diffKey, seaweed string) map[string]string {
	imgs := ResolveImagesFromEnv()
	infra := ResolveInfraFromEnv()
	vars := map[string]string{
		"TRANSFLOW_CONTEXT_DIR":       baseDir,
		"TRANSFLOW_OUT_DIR":           ensureOutDir(baseDir),
		"PLOY_TRANSFLOW_EXECUTION_ID": execID,
		"TRANSFLOW_DIFF_KEY":          diffKey,
		"PLOY_CONTROLLER":             infra.Controller,
		"PLOY_SEAWEEDFS_URL":          seaweed,
		"TRANSFLOW_ORW_APPLY_IMAGE":   imgs.ORWApply,
		"TRANSFLOW_REGISTRY":          imgs.Registry,
		"NOMAD_DC":                    infra.DC,
	}
	return vars
}
