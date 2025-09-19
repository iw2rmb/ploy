package mods

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// executeWithPlan handles execution modes that require a plan.json
func executeWithPlan(runner *ModRunner, planBytes []byte, execFirst, execLLM, execORW, applyFirst bool) error {
	if len(planBytes) == 0 {
		return nil
	}

	// Sequential stub: select first option and print intended action
	var parsed struct {
		PlanID  string           `json:"plan_id"`
		Options []map[string]any `json:"options"`
	}
	if err := jsonUnmarshal(planBytes, &parsed); err != nil || len(parsed.Options) == 0 {
		return nil
	}

	// Find first matching option for each request
	var first = parsed.Options[0]
	id, _ := first["id"].(string)
	typ, _ := first["type"].(string)

	if execFirst {
		fmt.Printf("Sequential stub: would execute first option %s (%s) next.\n", id, typ)
		if typ == string(StepTypeLLMExec) {
			if path, err := runner.RenderLLMExecAssets(id); err == nil {
				fmt.Printf("Rendered llm_exec HCL: %s\n", path)
			}
		}
	}

	if execLLM {
		if err := executeFirstLLMExec(runner, parsed.Options); err != nil {
			return err
		}
	}

	if execORW {
		if err := executeFirstORWGen(runner, parsed.Options); err != nil {
			return err
		}
	}

	if applyFirst {
		if err := executeApplyFirst(runner); err != nil {
			return err
		}
	}

	return nil
}

// executeFirstLLMExec finds first llm-exec option and executes it
func executeFirstLLMExec(runner *ModRunner, options []map[string]any) error {
	// Find first llm-exec
	for _, o := range options {
		if t, _ := o["type"].(string); t == string(StepTypeLLMExec) {
			lid, _ := o["id"].(string)
			if hcl, err := runner.RenderLLMExecAssets(lid); err == nil {
				fmt.Printf("Rendered llm_exec HCL: %s\n", hcl)
				// Centralized substitution (no global env writes)
				runID := LLMRunID(lid)
				imgs := ResolveImagesFromEnv()
				infra := ResolveInfraFromEnv()
				llm := ResolveLLMDefaultsFromEnv()
				vars := map[string]string{
					"MODS_CONTEXT_DIR":     filepath.Dir(hcl),
					"MODS_OUT_DIR":         filepath.Join(filepath.Dir(hcl), "out"),
					"MODS_REGISTRY":        imgs.Registry,
					"MODS_PLANNER_IMAGE":   imgs.Planner,
					"MODS_REDUCER_IMAGE":   imgs.Reducer,
					"MODS_LLM_EXEC_IMAGE":  imgs.LLMExec,
					"MODS_ORW_APPLY_IMAGE": imgs.ORWApply,
					"MODS_MODEL":           llm.Model,
					"MODS_TOOLS":           llm.ToolsJSON,
					"MODS_LIMITS":          llm.LimitsJSON,
					"PLOY_CONTROLLER":      infra.Controller,
					"MOD_ID":               os.Getenv("MOD_ID"),
					"NOMAD_DC":             infra.DC,
				}
				if infra.Controller != "" && runner.config != nil && runner.config.TargetRepo != "" {
					vars["SBOM_LATEST_URL"] = fmt.Sprintf("%s/sbom/latest?repo=%s", strings.TrimRight(infra.Controller, "/"), url.QueryEscape(runner.config.TargetRepo))
				}
				renderedPath, sErr := substituteHCLTemplateWithMCPVars(hcl, runID, vars, nil)
				if sErr != nil {
					fmt.Printf("failed to write substituted HCL: %v\n", sErr)
					return nil
				}
				fmt.Printf("Rendered llm_exec HCL (substituted): %s\n", renderedPath)
				if os.Getenv("MODS_SUBMIT") == "1" {
					timeout := ResolveDefaultsFromEnv().LLMExecTimeout
					if err := runner.hcl.SubmitCtx(context.Background(), renderedPath, timeout); err != nil {
						fmt.Printf("llm-exec job failed: %v\n", err)
					} else {
						// Show where diff.patch would be
						diffPath := filepath.Join(filepath.Dir(renderedPath), "out", "diff.patch")
						fmt.Printf("llm-exec completed. diff.patch expected at: %s (or via MODS_DIFF_URL/MODS_DIFF_PATH).\n", diffPath)
					}
				} else {
					fmt.Println("Skipping llm-exec submission (unset MODS_SUBMIT).")
				}
			}
			break
		}
	}
	return nil
}

// executeFirstORWGen finds first orw-gen option and executes it
func executeFirstORWGen(runner *ModRunner, options []map[string]any) error {
	// Find first orw-gen
	for _, o := range options {
		if t, _ := o["type"].(string); t == string(StepTypeORWGen) {
			oid, _ := o["id"].(string)
			if hcl, err := runner.RenderORWApplyAssets(oid); err == nil {
				fmt.Printf("Rendered orw_apply HCL: %s\n", hcl)
				// Pre-substitute recipe placeholders (no global env mutation)
				rclass := os.Getenv("MODS_RECIPE_CLASS")
				rcoords := os.Getenv("MODS_RECIPE_COORDS")
				rtimeout := os.Getenv("MODS_RECIPE_TIMEOUT")
				prePath, _ := preSubstituteRecipe(hcl, rclass, rcoords, rtimeout)
				// Prepare context: clone repo into context subdir
				baseDir := filepath.Dir(hcl)
				contextDir := filepath.Join(baseDir, "context")
				_ = os.MkdirAll(contextDir, 0755)
				_ = os.MkdirAll(filepath.Join(baseDir, "out"), 0755)
				// repo info from plan inputs or fallback to config
				var repoURL, repoRef string
				if inputsRaw, ok := o["inputs"].(map[string]any); ok {
					if repoMap, ok2 := inputsRaw["repo"].(map[string]any); ok2 {
						if u, ok3 := repoMap["url"].(string); ok3 {
							repoURL = u
						}
						if r, ok3 := repoMap["ref"].(string); ok3 {
							repoRef = r
						}
					}
				}
				if repoURL == "" {
					repoURL = runner.config.TargetRepo
					repoRef = runner.config.BaseRef
				}
				if repoURL != "" {
					_ = os.RemoveAll(contextDir)
					if err := cloneRepo(repoURL, repoRef, contextDir); err != nil {
						fmt.Printf("warning: repo clone failed: %v\n", err)
					}
				}
				runID2 := ORWRunID(oid)
				imgs := ResolveImagesFromEnv()
				infra := ResolveInfraFromEnv()
				vars := map[string]string{
					"MODS_CONTEXT_DIR":     contextDir,
					"MODS_OUT_DIR":         filepath.Join(baseDir, "out"),
					"MODS_ORW_APPLY_IMAGE": imgs.ORWApply,
					"MODS_REGISTRY":        imgs.Registry,
					"PLOY_CONTROLLER":      infra.Controller,
					"MOD_ID":               os.Getenv("MOD_ID"),
					"PLOY_SEAWEEDFS_URL":   infra.SeaweedURL,
					"MODS_DIFF_KEY":        os.Getenv("MODS_DIFF_KEY"),
					"NOMAD_DC":             infra.DC,
				}
				submittedPath, serr := substituteORWTemplateVars(prePath, runID2, vars)
				if serr != nil {
					fmt.Printf("failed to write submitted HCL: %v\n", serr)
				} else {
					fmt.Printf("Rendered orw_apply HCL (substituted): %s\n", submittedPath)
					if os.Getenv("MODS_SUBMIT") == "1" {
						timeout := ResolveDefaultsFromEnv().ORWApplyTimeout
						if err := runner.hcl.SubmitCtx(context.Background(), submittedPath, timeout); err != nil {
							fmt.Printf("orw-apply job failed: %v\n", err)
						} else {
							diffPath := filepath.Join(filepath.Dir(submittedPath), "out", "diff.patch")
							fmt.Printf("orw-apply completed. diff.patch expected at: %s\n", diffPath)
						}
					} else {
						fmt.Println("Skipping orw-apply submission (unset MODS_SUBMIT).")
					}
				}
			}
			break
		}
	}
	return nil
}

// executeApplyFirst fetches diff and applies it to repo
func executeApplyFirst(runner *ModRunner) error {
	// Fetch diff content path or URL
	var diffPath string
	if url := os.Getenv("MODS_DIFF_URL"); url != "" {
		dp := filepath.Join(runner.workspaceDir, "apply", "diff.patch")
		_ = os.MkdirAll(filepath.Dir(dp), 0755)
		if err := downloadToFileFn(url, dp); err == nil {
			diffPath = dp
		}
	}
	if diffPath == "" {
		if p := os.Getenv("MODS_DIFF_PATH"); p != "" {
			diffPath = p
		}
	}
	if diffPath == "" {
		fmt.Println("Missing MODS_DIFF_URL or MODS_DIFF_PATH for --apply-first")
		return nil
	}

	// Prepare repo and apply diff
	repoPath, _, err := runner.PrepareRepo(context.Background())
	if err != nil {
		fmt.Printf("PrepareRepo failed: %v\n", err)
		return err
	}

	if err := runner.ApplyDiffAndBuild(context.Background(), repoPath, diffPath); err != nil {
		fmt.Printf("Apply/build failed: %v\n", err)
		return err
	}

	fmt.Println("Apply/build succeeded")
	return nil
}

// substituteORWTemplateVars performs HCL substitution using provided variables (no global env mutation)
func substituteORWTemplateVars(prePath, runID string, vars map[string]string) (string, error) {
	submittedPath := strings.ReplaceAll(prePath, ".pre.hcl", ".submitted.hcl")

	// Read the template
	content, err := os.ReadFile(prePath)
	if err != nil {
		return "", err
	}

	// Resolve variables and defaults
	contextDir := vars["MODS_CONTEXT_DIR"]
	outDir := vars["MODS_OUT_DIR"]

	// Default image from vars or registry
	d := ResolveDefaults(func(k string) string { return vars[k] })
	orwImage := vars["MODS_ORW_APPLY_IMAGE"]
	if orwImage == "" {
		orwImage = d.ORWApplyImage
	}

	// Controller and MOD_ID for in-job event push
	controllerURL := vars["PLOY_CONTROLLER"]
	modID := vars["MOD_ID"]
	seaweedURL := vars["PLOY_SEAWEEDFS_URL"]
	if seaweedURL == "" {
		seaweedURL = d.SeaweedURL
	}
	// Keys under artifacts/ namespace used by uploader/runner
	// Allow override via MODS_DIFF_KEY for branch-scoped step uploads
	diffKey := vars["MODS_DIFF_KEY"]
	if diffKey == "" {
		diffKey = "mods/" + modID + "/diff.patch"
	}
	inputKey := "mods/" + modID + "/input.tar"
	inputURL := seaweedURL + "/artifacts/" + inputKey
	log.Printf("[Mods] Computed INPUT_URL=%s (SEAWEEDFS_URL=%s)", inputURL, seaweedURL)

	dc := vars["NOMAD_DC"]
	if dc == "" {
		dc = d.DC
	}

	// Compute API base (without /v1) for PLOY_API_URL used by runner metadata registration
	apiBase := strings.TrimSuffix(controllerURL, "/v1")

	rendered := strings.NewReplacer(
		"${RUN_ID}", runID,
		"${CONTEXT_HOST_DIR}", contextDir,
		"${OUT_HOST_DIR}", outDir,
		"${ORW_IMAGE}", orwImage,
		"${CONTROLLER_URL}", controllerURL,
		"${PLOY_API_URL}", apiBase,
		"${MOD_ID}", modID,
		"${SEAWEEDFS_URL}", seaweedURL,
		"${DIFF_KEY}", diffKey,
		"${INPUT_KEY}", inputKey,
		"${INPUT_URL}", inputURL,
		"${NOMAD_DC}", dc,
	).Replace(string(content))

	if err := os.WriteFile(submittedPath, []byte(rendered), 0644); err != nil {
		return "", err
	}
	return submittedPath, nil
}
