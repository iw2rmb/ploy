package transflow

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// executeWithPlan handles execution modes that require a plan.json
func executeWithPlan(runner *TransflowRunner, planBytes []byte, execFirst, execLLM, execORW, applyFirst bool) error {
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
		if typ == "llm-exec" {
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
func executeFirstLLMExec(runner *TransflowRunner, options []map[string]any) error {
	// Find first llm-exec
	for _, o := range options {
		if t, _ := o["type"].(string); t == "llm-exec" {
			lid, _ := o["id"].(string)
			if hcl, err := runner.RenderLLMExecAssets(lid); err == nil {
				fmt.Printf("Rendered llm_exec HCL: %s\n", hcl)
				// Substitute envs
				hb, _ := os.ReadFile(hcl)
				model := os.Getenv("TRANSFLOW_MODEL")
				if model == "" {
					model = "gpt-4o-mini@2024-08-06"
				}
				tools := os.Getenv("TRANSFLOW_TOOLS")
				if tools == "" {
					tools = `{"file":{"allow":["src/**","pom.xml"]}}`
				}
				limits := os.Getenv("TRANSFLOW_LIMITS")
				if limits == "" {
					limits = `{"max_steps":8,"max_tool_calls":12,"timeout":"30m"}`
				}
				runID := fmt.Sprintf("%s-%d", runner.config.ID, time.Now().Unix())
				rendered := strings.NewReplacer(
					"${MODEL}", model,
					"${TOOLS_JSON}", tools,
					"${LIMITS_JSON}", limits,
					"${RUN_ID}", runID,
				).Replace(string(hb))
				renderedPath := strings.ReplaceAll(hcl, ".rendered.hcl", ".rendered.submitted.hcl")
				_ = os.WriteFile(renderedPath, []byte(rendered), 0644)
				fmt.Printf("Rendered llm_exec HCL (substituted): %s\n", renderedPath)
				if os.Getenv("TRANSFLOW_SUBMIT") == "1" {
					if err := orchestration.SubmitAndWaitTerminal(renderedPath, 30*time.Minute); err != nil {
						fmt.Printf("llm-exec job failed: %v\n", err)
					} else {
						// Show where diff.patch would be
						diffPath := filepath.Join(filepath.Dir(renderedPath), "out", "diff.patch")
						fmt.Printf("llm-exec completed. diff.patch expected at: %s (or via TRANSFLOW_DIFF_URL/TRANSFLOW_DIFF_PATH).\n", diffPath)
					}
				} else {
					fmt.Println("Skipping llm-exec submission (unset TRANSFLOW_SUBMIT).")
				}
			}
			break
		}
	}
	return nil
}

// executeFirstORWGen finds first orw-gen option and executes it
func executeFirstORWGen(runner *TransflowRunner, options []map[string]any) error {
	// Find first orw-gen
	for _, o := range options {
		if t, _ := o["type"].(string); t == "orw-gen" {
			oid, _ := o["id"].(string)
			if hcl, err := runner.RenderORWApplyAssets(oid); err == nil {
				fmt.Printf("Rendered orw_apply HCL: %s\n", hcl)
				// Pre-substitute recipe placeholders
				hb, _ := os.ReadFile(hcl)
				rclass := os.Getenv("TRANSFLOW_RECIPE_CLASS")
				if rclass == "" {
					rclass = "org.openrewrite.java.migrate.Java11toJava17"
				}
				rcoords := os.Getenv("TRANSFLOW_RECIPE_COORDS")
				rtimeout := os.Getenv("TRANSFLOW_RECIPE_TIMEOUT")
				if rtimeout == "" {
					rtimeout = "10m"
				}
				pre := strings.NewReplacer(
					"${RECIPE_CLASS}", rclass,
					"${RECIPE_COORDS}", rcoords,
					"${RECIPE_TIMEOUT}", rtimeout,
				).Replace(string(hb))
				prePath := strings.ReplaceAll(hcl, ".rendered.hcl", ".pre.hcl")
				_ = os.WriteFile(prePath, []byte(pre), 0644)
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
				os.Setenv("TRANSFLOW_CONTEXT_DIR", contextDir)
				os.Setenv("TRANSFLOW_OUT_DIR", filepath.Join(baseDir, "out"))
				runID2 := fmt.Sprintf("%s-orw-apply-%d", runner.config.ID, time.Now().Unix())
				submittedPath, serr := substituteORWTemplate(prePath, runID2)
				if serr != nil {
					fmt.Printf("failed to write submitted HCL: %v\n", serr)
				} else {
					fmt.Printf("Rendered orw_apply HCL (substituted): %s\n", submittedPath)
					if os.Getenv("TRANSFLOW_SUBMIT") == "1" {
						if err := orchestration.SubmitAndWaitTerminal(submittedPath, 30*time.Minute); err != nil {
							fmt.Printf("orw-apply job failed: %v\n", err)
						} else {
							diffPath := filepath.Join(filepath.Dir(submittedPath), "out", "diff.patch")
							fmt.Printf("orw-apply completed. diff.patch expected at: %s\n", diffPath)
						}
					} else {
						fmt.Println("Skipping orw-apply submission (unset TRANSFLOW_SUBMIT).")
					}
				}
			}
			break
		}
	}
	return nil
}

// executeApplyFirst fetches diff and applies it to repo
func executeApplyFirst(runner *TransflowRunner) error {
	// Fetch diff content path or URL
	var diffPath string
	if url := os.Getenv("TRANSFLOW_DIFF_URL"); url != "" {
		if resp, err := http.Get(url); err == nil && resp.StatusCode == 200 {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			// Write to temp file in workspace
			dp := filepath.Join(runner.workspaceDir, "apply", "diff.patch")
			_ = os.MkdirAll(filepath.Dir(dp), 0755)
			_ = os.WriteFile(dp, b, 0644)
			diffPath = dp
		}
	}
	if diffPath == "" {
		if p := os.Getenv("TRANSFLOW_DIFF_PATH"); p != "" {
			diffPath = p
		}
	}
	if diffPath == "" {
		fmt.Println("Missing TRANSFLOW_DIFF_URL or TRANSFLOW_DIFF_PATH for --apply-first")
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

// substituteHCLTemplate performs HCL template substitution (needs to be implemented based on original logic)
func substituteORWTemplate(prePath, runID string) (string, error) {
	// Backward-compatible wrapper that reads from process env
	vars := map[string]string{
		"TRANSFLOW_CONTEXT_DIR":       os.Getenv("TRANSFLOW_CONTEXT_DIR"),
		"TRANSFLOW_OUT_DIR":           os.Getenv("TRANSFLOW_OUT_DIR"),
		"TRANSFLOW_ORW_APPLY_IMAGE":   os.Getenv("TRANSFLOW_ORW_APPLY_IMAGE"),
		"TRANSFLOW_REGISTRY":          os.Getenv("TRANSFLOW_REGISTRY"),
		"PLOY_CONTROLLER":             os.Getenv("PLOY_CONTROLLER"),
		"PLOY_TRANSFLOW_EXECUTION_ID": os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"),
		"PLOY_SEAWEEDFS_URL":          os.Getenv("PLOY_SEAWEEDFS_URL"),
		"TRANSFLOW_DIFF_KEY":          os.Getenv("TRANSFLOW_DIFF_KEY"),
		"NOMAD_DC":                    os.Getenv("NOMAD_DC"),
	}
	return substituteORWTemplateVars(prePath, runID, vars)
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
	contextDir := vars["TRANSFLOW_CONTEXT_DIR"]
	outDir := vars["TRANSFLOW_OUT_DIR"]

	// Default image from vars or registry
	d := ResolveDefaults(func(k string) string { return vars[k] })
	orwImage := vars["TRANSFLOW_ORW_APPLY_IMAGE"]
	if orwImage == "" {
		orwImage = d.ORWApplyImage
	}

	// Controller and execution ID for in-job event push
	controllerURL := vars["PLOY_CONTROLLER"]
	execID := vars["PLOY_TRANSFLOW_EXECUTION_ID"]
    seaweedURL := vars["PLOY_SEAWEEDFS_URL"]
    if seaweedURL == "" { seaweedURL = d.SeaweedURL }
	// Keys under artifacts/ namespace used by uploader/runner
	// Allow override via TRANSFLOW_DIFF_KEY for branch-scoped step uploads
	diffKey := vars["TRANSFLOW_DIFF_KEY"]
	if diffKey == "" {
		diffKey = "transflow/" + execID + "/diff.patch"
	}
	inputKey := "transflow/" + execID + "/input.tar"
	inputURL := seaweedURL + "/artifacts/" + inputKey
	log.Printf("[Transflow] Computed INPUT_URL=%s (SEAWEEDFS_URL=%s)", inputURL, seaweedURL)

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
		"${EXECUTION_ID}", execID,
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
