package mods

import (
	"os"
	"path/filepath"
)

// computeBranchDiffKey returns artifacts key for a branch step diff.
// Format: mods/<modID>/branches/<branchID>/steps/<stepID>/diff.patch
func computeBranchDiffKey(modID, branchID, stepID string) string {
	return "mods/" + modID + "/branches/" + branchID + "/steps/" + stepID + "/diff.patch"
}

// ensureOutDir ensures the out directory exists under the base dir and returns its path.
func ensureOutDir(baseDir string) string {
	outDir := filepath.Join(baseDir, "out")
	_ = os.MkdirAll(outDir, 0755)
	return outDir
}

// makeORWVars builds the substitution variables for ORW apply HCL templates.
func makeORWVars(baseDir, modID, diffKey, seaweed string) map[string]string {
	imgs := ResolveImagesFromEnv()
	infra := ResolveInfraFromEnv()
	vars := map[string]string{
		"MODS_CONTEXT_DIR":     baseDir,
		"MODS_OUT_DIR":         ensureOutDir(baseDir),
		"MOD_ID":               modID,
		"MODS_DIFF_KEY":        diffKey,
		"PLOY_CONTROLLER":      infra.Controller,
		"PLOY_SEAWEEDFS_URL":   seaweed,
		"MODS_ORW_APPLY_IMAGE": imgs.ORWApply,
		"MODS_REGISTRY":        imgs.Registry,
		"NOMAD_DC":             infra.DC,
	}
	return vars
}
