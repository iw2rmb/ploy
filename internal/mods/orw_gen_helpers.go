package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// buildORWRecipeConfig extracts and validates recipe coordinates from branch inputs.
func buildORWRecipeConfig(inputs map[string]interface{}) (class, coords, timeout, pluginVersion string, err error) {
	class, coords, timeout, pluginVersion = "", "", "10m", ""
	if inputs == nil {
		return "", "", timeout, pluginVersion, fmt.Errorf("missing recipe_config inputs")
	}
	cfgAny, ok := inputs["recipe_config"]
	if !ok {
		if nested, okNested := inputs["inputs"].(map[string]interface{}); okNested {
			cfgAny, ok = nested["recipe_config"]
			if pluginVersion == "" {
				if pv, okPV := inputs["maven_plugin_version"].(string); okPV {
					pluginVersion = strings.TrimSpace(pv)
				} else if pv, okPV := nested["maven_plugin_version"].(string); okPV {
					pluginVersion = strings.TrimSpace(pv)
				}
			}
		}
	}
	if !ok {
		return "", "", timeout, pluginVersion, fmt.Errorf("missing recipe_config inputs")
	}
	cfg, ok := cfgAny.(map[string]interface{})
	if !ok {
		return "", "", timeout, pluginVersion, fmt.Errorf("recipe_config must be a map")
	}
	if v, ok := cfg["class"].(string); ok {
		class = strings.TrimSpace(v)
	}
	if v, ok := cfg["coords"].(string); ok {
		coords = strings.TrimSpace(v)
	}
	if v, ok := cfg["timeout"].(string); ok && strings.TrimSpace(v) != "" {
		timeout = strings.TrimSpace(v)
	}
	if v, ok := inputs["maven_plugin_version"].(string); ok {
		pluginVersion = strings.TrimSpace(v)
	}

	if class == "" {
		return "", "", timeout, pluginVersion, fmt.Errorf("recipe class is required for orw-gen branch")
	}
	if coords == "" {
		return "", "", timeout, pluginVersion, fmt.Errorf("recipe coords are required for orw-gen branch")
	}
	parts := strings.Split(coords, ":")
	if len(parts) != 3 {
		return "", "", timeout, pluginVersion, fmt.Errorf("recipe coords must be 'group:artifact:version' (got %q)", coords)
	}
	group := strings.TrimSpace(parts[0])
	artifact := strings.TrimSpace(parts[1])
	version := strings.TrimSpace(parts[2])
	if group == "" || artifact == "" || version == "" {
		return "", "", timeout, pluginVersion, fmt.Errorf("recipe coords must include non-empty group, artifact, and version")
	}
	if artifact == "rewrite-java-latest" && version == "latest" {
		artifact = "rewrite-migrate-java"
		version = "3.17.0"
		if pluginVersion == "" {
			pluginVersion = "6.18.0"
		}
	}
	coords = fmt.Sprintf("%s:%s:%s", group, artifact, version)
	return class, coords, timeout, pluginVersion, nil
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
