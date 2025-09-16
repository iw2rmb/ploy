package mods

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// executeLLMExecBranch executes an LLM-based code generation branch
func (o *fanoutOrchestrator) executeLLMExecBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Step 1: Render LLM exec assets
	hclPath, err := o.runner.RenderLLMExecAssets(branch.ID)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to render LLM exec assets: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Provide host directories for bind mounts during substitution
	// Use the directory of the HCL as context, and an 'out' subdir for outputs
	baseDir := filepath.Dir(hclPath)
	// Ensure out directory exists for bind mount target
	_ = os.MkdirAll(filepath.Join(baseDir, "out"), 0755)
	imgs := ResolveImagesFromEnv()
	infra := ResolveInfraFromEnv()
	llm := ResolveLLMDefaultsFromEnv()
	modID := os.Getenv("MOD_ID")
	if modID != "" && !strings.HasPrefix(modID, "mod-") {
		modID = "mod-" + modID
	}

	vars := map[string]string{
		"MODS_CONTEXT_DIR":    baseDir,
		"MODS_OUT_DIR":        filepath.Join(baseDir, "out"),
		"MODS_REGISTRY":       imgs.Registry,
		"MODS_PLANNER_IMAGE":  imgs.Planner,
		"MODS_REDUCER_IMAGE":  imgs.Reducer,
		"MODS_LLM_EXEC_IMAGE": imgs.LLMExec,
		"PLOY_CONTROLLER":     infra.Controller,
		"MOD_ID":              modID,
		"PLOY_SEAWEEDFS_URL":  infra.SeaweedURL,
		"NOMAD_DC":            infra.DC,
		"MODS_MODEL":          llm.Model,
		"MODS_TOOLS":          llm.ToolsJSON,
		"MODS_LIMITS":         llm.LimitsJSON,
	}

	// Step 2: Generate unique run ID for this branch
	runID := LLMRunID(branch.ID)

	// Prepare and upload a context tar for artifact download (mirror planner/reducer behavior)
	if infra.SeaweedURL != "" {
		ctxDir := filepath.Join(baseDir, "context")
		_ = os.MkdirAll(ctxDir, 0755)
		_ = os.WriteFile(filepath.Join(ctxDir, ".keep"), []byte("llm-context"), 0644)
		// Inject inputs.json with last_error if provided
		if be, ok := branch.Inputs["build_error"].(string); ok && strings.TrimSpace(be) != "" {
			inputsJSON := fmt.Sprintf("{\n  \"language\": \"java\",\n  \"lane\": \"%s\",\n  \"last_error\": {\n    \"stdout\": \"\",\n    \"stderr\": %q\n  },\n  \"deps\": {}\n}\n", "", be)
			_ = os.WriteFile(filepath.Join(ctxDir, "inputs.json"), []byte(inputsJSON), 0644)
			// Best-effort: extract up to 5 .java paths from error and include current sources for diffing
			repoRoot := filepath.Join(o.runner.GetWorkspaceDir(), "repo")
			seen := make(map[string]struct{})
			paths := extractJavaPathsFromError(be, 5)
			// Augment with guesses from class names in error if needed
			if len(paths) == 0 {
				classNames := parseClassNamesFromError(be, 5)
				guessed := findJavaFilesByBasename(repoRoot, classNames, 5)
				paths = append(paths, guessed...)
			}
			var manifest []string
			for _, cand := range paths {
				// Normalize to relative form
				rel := cand
				if strings.HasPrefix(rel, repoRoot+string(os.PathSeparator)) {
					rel = strings.TrimPrefix(rel, repoRoot+string(os.PathSeparator))
				}
				if _, ok := seen[rel]; ok || strings.TrimSpace(rel) == "" {
					continue
				}
				seen[rel] = struct{}{}
				srcAbs := filepath.Join(repoRoot, rel)
				if b, err := ioutil.ReadFile(srcAbs); err == nil {
					dst := filepath.Join(ctxDir, "sources", rel)
					_ = os.MkdirAll(filepath.Dir(dst), 0755)
					_ = ioutil.WriteFile(dst, b, 0644)
					manifest = append(manifest, rel)
				}
			}
			if len(manifest) > 0 {
				_ = ioutil.WriteFile(filepath.Join(ctxDir, "source_manifest.txt"), []byte(strings.Join(manifest, "\n")+"\n"), 0644)
			}
		}
		// Optional: pass through precomputed diff or delete paths from branch inputs (keeps runner generic)
		if v, ok := branch.Inputs["precomputed_diff"].(string); ok && strings.TrimSpace(v) != "" {
			_ = os.WriteFile(filepath.Join(ctxDir, "diff.patch"), []byte(v), 0644)
		}
		if arr, ok := branch.Inputs["delete_paths"].([]string); ok && len(arr) > 0 {
			var b strings.Builder
			for _, p := range arr {
				if strings.TrimSpace(p) == "" {
					continue
				}
				b.WriteString(p)
				b.WriteString("\n")
			}
			_ = os.WriteFile(filepath.Join(ctxDir, "delete_paths.txt"), []byte(b.String()), 0644)
		} else if arrI, ok := branch.Inputs["delete_paths"].([]interface{}); ok && len(arrI) > 0 {
			var b strings.Builder
			for _, it := range arrI {
				if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
					b.WriteString(s)
					b.WriteString("\n")
				}
			}
			_ = os.WriteFile(filepath.Join(ctxDir, "delete_paths.txt"), []byte(b.String()), 0644)
		}
		tarPath := filepath.Join(baseDir, "llm-context.tar")
		if err := createTarFromDir(ctxDir, tarPath); err == nil {
			if modID != "" {
				key := fmt.Sprintf("mods/%s/contexts/%s.tar", modID, runID)
				_ = putFileFn(infra.SeaweedURL, key, tarPath, "application/octet-stream")
				vars["MODS_CONTEXT_URL"] = strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
			}
		}
	}

	// Step 3: Extract MCP configuration from branch inputs
	var mcpConfig *MCPConfig = nil
	if mcpData, ok := branch.Inputs["mcp_config"]; ok {
		if mcpConfigMap, ok := mcpData.(map[string]interface{}); ok {
			// Convert map to MCPConfig struct
			if parsedMCP, err := parseMCPFromInputs(mcpConfigMap); err == nil {
				mcpConfig = parsedMCP
			}
		}
	}

	// Step 4: Substitute environment variables in HCL template with MCP support
	// If branch MCP config specifies a model, prefer it
	if mcpConfig != nil && strings.TrimSpace(mcpConfig.Model) != "" {
		vars["MODS_MODEL"] = strings.TrimSpace(mcpConfig.Model)
	}
	renderedHCLPath, err := substituteHCLTemplateWithMCPVars(hclPath, runID, vars, mcpConfig)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to substitute HCL template: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Persist submitted HCL for diagnostics and emit controller event with path
	if o.runner != nil {
		if modID := os.Getenv("MOD_ID"); modID != "" {
			persistDir := filepath.Join("/tmp/mods-submitted", modID, "llm-exec", branch.ID)
			_ = os.MkdirAll(persistDir, 0755)
			dest := filepath.Join(persistDir, "llm_exec.submitted.hcl")
			if b, e := os.ReadFile(renderedHCLPath); e == nil {
				_ = os.WriteFile(dest, b, 0644)
				if rep := o.runner.GetEventReporter(); rep != nil {
					_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("Saved submitted HCL to %s", dest), JobName: runID, Time: time.Now()})
				}
			}
		}
	}

	// Step 4: Report job metadata asynchronously (job name == runID)
	var rep EventReporter
	if o.runner != nil {
		rep = o.runner.GetEventReporter()
	}
	reportJobSubmittedAsync(ctx, rep, runID, string(StepTypeLLMExec), string(StepTypeLLMExec))

	// Step 5: Preflight validate HCL, then submit job to Nomad and wait for completion
	var vErr error
	if o.hcl != nil {
		vErr = o.hcl.Validate(renderedHCLPath)
	} else {
		// In unit tests, HCL submitter may be nil; skip validation
		vErr = nil
	}
	if vErr != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("LLM exec HCL validation failed: %v", vErr)
		if o.runner != nil {
			if rep := o.runner.GetEventReporter(); rep != nil {
				_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "error", Message: fmt.Sprintf("validation failed: %v", vErr), JobName: runID, Time: time.Now()})
			}
		}
		_ = orchestration.DeregisterJob(runID, true)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	timeout := ResolveDefaultsFromEnv().LLMExecTimeout
	var sErr error
	if o.hcl != nil {
		sErr = o.hcl.SubmitCtx(ctx, renderedHCLPath, timeout)
	} else {
		// In unit tests, HCL submitter may be nil; signal failure to match expectations
		sErr = fmt.Errorf("no HCL submitter in test mode")
	}
	if sErr != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("LLM exec job failed: %v", sErr)
		if o.runner != nil {
			if rep := o.runner.GetEventReporter(); rep != nil {
				_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "error", Message: fmt.Sprintf("submission failed: %v", sErr), JobName: runID, Time: time.Now()})
			}
		}
		_ = orchestration.DeregisterJob(runID, true)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 6: Fetch diff.patch from SeaweedFS in prod; in tests (no HCL submitter), rely on local artifact existence
	diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	_ = os.MkdirAll(filepath.Dir(diffPath), 0755)
	if o.hcl != nil {
		id := modID
		branchID := branch.ID
		stepID := runID
		if infra.SeaweedURL == "" || id == "" {
			result.Status = "failed"
			result.Notes = "LLM exec missing SeaweedFS URL or execution ID for artifact fetch"
			result.FinishedAt = time.Now()
			result.Duration = time.Since(result.StartedAt)
			return result
		}
		key := computeBranchDiffKey(id, branchID, stepID)
		url := strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
		// Emit download attempt event with timing start
		dlStart := time.Now()
		if o.runner != nil && o.runner.GetEventReporter() != nil {
			_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("download start: key=%s start_ts=%s", key, dlStart.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
		}
		// Download with extended retry/backoff to avoid race with artifact upload
		var dlErr error
		for i := 0; i < 20; i++ {
			if err := downloadToFileFn(url, diffPath); err == nil {
				dlErr = nil
				break
			} else {
				dlErr = err
				time.Sleep(1 * time.Second)
			}
		}
		dlEnd := time.Now()
		if dlErr != nil {
			if o.runner != nil && o.runner.GetEventReporter() != nil {
				_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "error", Message: fmt.Sprintf("download failed: key=%s error=%v start_ts=%s end_ts=%s", key, dlErr, dlStart.UTC().Format(time.RFC3339Nano), dlEnd.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
			}
			result.Status = "failed"
			result.Notes = fmt.Sprintf("LLM exec diff download failed: %v", dlErr)
			_ = orchestration.DeregisterJob(runID, true)
			result.FinishedAt = time.Now()
			result.Duration = time.Since(result.StartedAt)
			return result
		}
		if o.runner != nil && o.runner.GetEventReporter() != nil {
			// Best-effort size
			var sz int64
			if fi, err := os.Stat(diffPath); err == nil {
				sz = fi.Size()
			}
			_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("download succeeded: key=%s bytes=%d start_ts=%s end_ts=%s", key, sz, dlStart.UTC().Format(time.RFC3339Nano), dlEnd.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
		}
	}

	// Step 6b: Upload LLM diff to SeaweedFS with step-scoped key (align with ORW convention)
	// mods/<modID>/branches/<branchID>/steps/<stepID>/diff.patch (reuse computed IDs)
	if o.hcl != nil {
		id := modID
		branchID := branch.ID
		stepID := runID
		if id != "" && infra.SeaweedURL != "" {
			diffKey := computeBranchDiffKey(id, branchID, stepID)
			// Best-effort upload and write chain metadata
			_ = putFileFn(infra.SeaweedURL, diffKey, diffPath, "text/plain")
			_ = writeBranchChainStepMeta(infra.SeaweedURL, id, branchID, stepID, diffKey)
		}
	}

	// Cleanup job registration after successful artifact retrieval
	_ = orchestration.DeregisterJob(runID, true)
	result.Status = "completed"
	result.Notes = fmt.Sprintf("LLM exec job completed successfully, diff.patch at: %s", diffPath)
	result.FinishedAt = time.Now()
	result.Duration = time.Since(result.StartedAt)
	return result
}
