package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// buildORWRecipeConfig extracts recipe class, coords and timeout from branch inputs.
func buildORWRecipeConfig(inputs map[string]interface{}) (class, coords, timeout, pluginVersion string) {
	class, coords, timeout, pluginVersion = "", "", "10m", ""
	if inputs == nil {
		return
	}
	if cfg, ok := inputs["recipe_config"].(map[string]interface{}); ok {
		if v, ok := cfg["class"].(string); ok {
			class = v
		}
		if v, ok := cfg["coords"].(string); ok {
			coords = v
		}
		if v, ok := cfg["timeout"].(string); ok {
			timeout = v
		}
	}
	if v, ok := inputs["maven_plugin_version"].(string); ok {
		pluginVersion = v
	}
	return
}

// orwPreSubstitute writes a pre-substituted HCL with recipe-specific variables.
func orwPreSubstitute(hclPath, class, coords, timeout string) (string, error) {
	b, err := os.ReadFile(hclPath)
	if err != nil {
		return "", fmt.Errorf("failed to read ORW HCL template: %w", err)
	}
	pre := strings.NewReplacer(
		"${RECIPE_CLASS}", class,
		"${RECIPE_COORDS}", coords,
		"${RECIPE_TIMEOUT}", timeout,
	).Replace(string(b))
	prePath := strings.ReplaceAll(hclPath, ".rendered.hcl", ".pre.hcl")
	if err := os.WriteFile(prePath, []byte(pre), 0644); err != nil {
		return "", fmt.Errorf("failed to write pre-substituted ORW HCL: %w", err)
	}
	return prePath, nil
}

// orwMakeVars builds environment variables map for ORW job.
func orwMakeVars(baseDir string) map[string]string {
	imgs := ResolveImagesFromEnv()
	infra := ResolveInfraFromEnv()
	_ = os.MkdirAll(filepath.Join(baseDir, "out"), 0755)
	return map[string]string{
		"MODS_CONTEXT_DIR":     baseDir,
		"MODS_OUT_DIR":         filepath.Join(baseDir, "out"),
		"PLOY_CONTROLLER":      infra.Controller,
		"MOD_ID":               os.Getenv("MOD_ID"),
		"PLOY_SEAWEEDFS_URL":   infra.SeaweedURL,
		"MODS_DIFF_KEY":        os.Getenv("MODS_DIFF_KEY"),
		"MODS_ORW_APPLY_IMAGE": imgs.ORWApply,
		"MODS_REGISTRY":        imgs.Registry,
		"NOMAD_DC":             infra.DC,
	}
}

// orwValidateAndSubmit validates the HCL and submits job via HCLSubmitter; allows partial completion path.
func orwValidateAndSubmit(ctx context.Context, hcl HCLSubmitter, renderedHCLPath string, allowPartial bool) error {
	if hcl != nil {
		if err := hcl.Validate(renderedHCLPath); err != nil {
			return fmt.Errorf("ORW apply HCL validation failed: %w", err)
		}
	}
	timeout := ResolveDefaultsFromEnv().ORWApplyTimeout
	if hcl != nil {
		// Use Submit (non-ctx) to enable unit-test stubbing via package-level var
		if err := hcl.Submit(renderedHCLPath, timeout); err != nil {
			if allowPartial {
				diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
				if fi, statErr := os.Stat(diffPath); statErr == nil && fi.Size() > 0 {
					return nil
				}
			}
			return fmt.Errorf("ORW apply job failed: %w", err)
		}
		return nil
	}
	return fmt.Errorf("ORW apply job failed: no HCL submitter in test mode")
}

// orwFinalize checks diff.patch existence and sets result fields.
func orwFinalize(result *BranchResult, renderedHCLPath, branchID string) {
	diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	if _, err := os.Stat(diffPath); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("ORW apply job completed but no diff.patch found: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return
	}
	result.Status = "completed"
	result.Notes = fmt.Sprintf("ORW apply job completed successfully, diff.patch at: %s", diffPath)
	result.FinishedAt = time.Now()
	result.Duration = time.Since(result.StartedAt)
}
