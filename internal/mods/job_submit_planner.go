package mods

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	build "github.com/iw2rmb/ploy/internal/build"
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// SubmitPlannerJob submits a planner job after a build failure
func (h *jobSubmissionHelper) SubmitPlannerJob(ctx context.Context, config *ModConfig, buildError string, workspace string) (*PlanResult, error) {
	// Production implementation using real Nomad job submission
	if h.runner != nil {
		// Step 1: Render planner assets
		assets, err := h.runner.RenderPlannerAssets()
		if err != nil {
			return nil, fmt.Errorf("failed to render planner assets: %w", err)
		}

		// Inject build error into planner inputs.json so downstream jobs have full compiler context
		{
			lane := ""
			if config != nil {
				lane = config.Lane
			}
			parsed := build.ParseBuildErrors("java", "maven", buildError)
			// Build JSON: last_error + optional first_error_* + full errors[]
			var b strings.Builder
			b.WriteString("{\n  \"language\": \"java\",\n  \"lane\": ")
			b.WriteString(strconv.Quote(lane))
			b.WriteString(",\n  \"last_error\": {\n    \"stdout\": \"\",\n    \"stderr\": ")
			b.WriteString(strconv.Quote(buildError))
			b.WriteString("\n  }")
			if len(parsed) > 0 {
				b.WriteString(",\n  \"first_error_file\": ")
				b.WriteString(strconv.Quote(parsed[0].File))
				b.WriteString(",\n  \"first_error_line\": ")
				b.WriteString(strconv.Itoa(parsed[0].Line))
			}
			b.WriteString(",\n  \"errors\": [")
			for i, e := range parsed {
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString("{\"file\":")
				b.WriteString(strconv.Quote(e.File))
				b.WriteString(",\"line\":")
				b.WriteString(strconv.Itoa(e.Line))
				b.WriteString(",\"column\":")
				b.WriteString(strconv.Itoa(e.Column))
				b.WriteString(",\"message\":")
				b.WriteString(strconv.Quote(e.Message))
				b.WriteString("}")
			}
			b.WriteString("\n  ]\n}\n")
			inputs := b.String()
			_ = os.WriteFile(assets.InputsPath, []byte(inputs), 0644)
			if controller := ResolveInfraFromEnv().Controller; controller != "" {
				rep := NewControllerEventReporter(controller, os.Getenv("MOD_ID"))
				_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "info", Message: fmt.Sprintf("prepared inputs.json (bytes=%d)", len(inputs)), JobName: "", Time: time.Now()})
			}
		}

		// Step 2: Generate unique run ID for this planner job
		runID := PlannerRunID(config.ID)

		// Step 3: Determine model from mods.yaml (if provided), provision in registry, then substitute env placeholders
		contextDir := filepath.Dir(assets.InputsPath)
		outDir := filepath.Join(workspace, "planner", "out")
		imgs := ResolveImagesFromEnv()
		infra := ResolveInfraFromEnv()
		modID := os.Getenv("MOD_ID")
		if modID == "" {
			return nil, fmt.Errorf("MOD_ID is required for planner job submission")
		}
		if !strings.HasPrefix(modID, "mod-") {
			modID = "mod-" + modID
		}
		llm := ResolveLLMDefaultsFromEnv()
		if config != nil {
			if pref := config.PreferredModel(); pref != "" {
				llm.Model = pref
			}
		}
		vars := map[string]string{
			"MODS_CONTEXT_DIR":     contextDir,
			"MODS_OUT_DIR":         outDir,
			"MODS_REGISTRY":        imgs.Registry,
			"MODS_PLANNER_IMAGE":   imgs.Planner,
			"MODS_REDUCER_IMAGE":   imgs.Reducer,
			"MODS_LLM_EXEC_IMAGE":  imgs.LLMExec,
			"MODS_ORW_APPLY_IMAGE": imgs.ORWApply,
			"MODS_MODEL":           llm.Model,
			"MODS_TOOLS":           llm.ToolsJSON,
			"MODS_LIMITS":          llm.LimitsJSON,
			"PLOY_CONTROLLER":      infra.Controller,
			"MOD_ID":               modID,
			"PLOY_SEAWEEDFS_URL":   infra.SeaweedURL,
			"NOMAD_DC":             infra.DC,
		}

		if infra.SeaweedURL != "" {
			_ = os.WriteFile(filepath.Join(contextDir, ".keep"), []byte("planner-context"), 0644)
			tarPath := filepath.Join(workspace, "planner", "context.tar")
			if err := createTarFromDir(contextDir, tarPath); err == nil {
				if modID != "" {
					key := fmt.Sprintf("mods/%s/contexts/%s.tar", modID, runID)
					_ = putFileFn(infra.SeaweedURL, key, tarPath, "application/octet-stream")
					vars["MODS_CONTEXT_URL"] = strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
				}
			}
		}
		if infra.Controller != "" && config != nil && config.TargetRepo != "" {
			vars["SBOM_LATEST_URL"] = fmt.Sprintf("%s/sbom/latest?repo=%s", strings.TrimRight(infra.Controller, "/"), url.QueryEscape(config.TargetRepo))
		}
		renderedHCLPath, err := substituteHCLTemplateWithMCPVars(assets.HCLPath, runID, vars, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to substitute HCL template: %w", err)
		}

		if modID != "" {
			persistDir := filepath.Join("/tmp/mods-submitted", modID, "planner")
			_ = os.MkdirAll(persistDir, 0755)
			dest := filepath.Join(persistDir, "planner.submitted.hcl")
			if b, e := os.ReadFile(renderedHCLPath); e == nil {
				_ = os.WriteFile(dest, b, 0644)
				if controller := ResolveInfraFromEnv().Controller; controller != "" {
					rep := NewControllerEventReporter(controller, modID)
					_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "info", Message: fmt.Sprintf("Saved submitted HCL to %s", dest), JobName: runID, Time: time.Now()})
				}
			}
		}

		if controller := ResolveInfraFromEnv().Controller; controller != "" {
			rep := NewControllerEventReporter(controller, modID)
			_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "info", Message: "job started", JobName: runID, Time: time.Now()})
			reportJobSubmittedAsync(ctx, rep, runID, "planner", "planner")
		}

		if err := h.runner.GetHCLSubmitter().Validate(renderedHCLPath); err != nil {
			return nil, fmt.Errorf("planner HCL validation failed: %w", err)
		}
		timeout := ResolveDefaultsFromEnv().PlannerTimeout
		if err := h.runner.GetHCLSubmitter().SubmitCtx(ctx, renderedHCLPath, timeout); err != nil {
			if controller := ResolveInfraFromEnv().Controller; controller != "" {
				rep := NewControllerEventReporter(controller, modID)
				_ = rep.Report(ctx, Event{Phase: "planner", Step: "planner", Level: "error", Message: fmt.Sprintf("job failed: %v", err), JobName: runID, Time: time.Now()})
			}
			_ = orchestration.DeregisterJob(runID, true)
			return nil, fmt.Errorf("planner job failed: %w", err)
		}

		artifactPath := filepath.Join(workspace, "planner", "out", "plan.json")
		if infra.SeaweedURL == "" || modID == "" {
			return nil, fmt.Errorf("planner artifact fetch requires SeaweedFS URL and execution ID")
		}
		if err := os.MkdirAll(filepath.Dir(artifactPath), 0755); err != nil {
			return nil, fmt.Errorf("planner artifact path prep: %w", err)
		}
		key := fmt.Sprintf("mods/%s/planner/%s/plan.json", modID, runID)
		url := strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
		// Event-driven: wait for upload event, then HEAD+jitter for filer readiness, single download
		_ = waitForStepContainingFn(infra.Controller, modID, "planner", "uploaded plan to", 120*time.Second)
		for i := 0; i < 30; i++ { // up to ~2s
			if headURLFn(url) {
				break
			}
			time.Sleep(300*time.Millisecond + time.Duration(i%5)*80*time.Millisecond)
		}
		if err := downloadToFileFn(url, artifactPath); err != nil {
			if infra.Controller != "" {
				fallbackURL := strings.TrimRight(infra.Controller, "/") + "/mods/" + modID + "/artifacts/plan_json"
				if err2 := downloadToFileFn(fallbackURL, artifactPath); err2 != nil {
					return nil, fmt.Errorf("failed to download planner artifact: %w (controller fallback: %v)", err, err2)
				}
			} else {
				return nil, fmt.Errorf("failed to download planner artifact: %w", err)
			}
		}

		// Parse plan.json to PlanResult
		var plan PlanResult
		if err := readJobArtifact(artifactPath, &plan); err != nil {
			return nil, err
		}
		return &plan, nil
	}

	// Fallback/mock path for unit tests - use provided submitter
	lane := "A"
	if config != nil && strings.TrimSpace(config.Lane) != "" {
		lane = config.Lane
	}
	spec := JobSpec{
		Name:    "planner",
		Type:    "planner",
		Inputs:  map[string]interface{}{"build_error": buildError, "lane": lane},
		Timeout: ResolveDefaultsFromEnv().PlannerTimeout,
	}
	result, err := h.submitter.SubmitAndWaitTerminal(ctx, spec)
	if err != nil {
		return nil, err
	}
	// Collect plan.json artifact if available
	artifactPath := filepath.Join(workspace, "planner", "out", "plan.json")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0755); err == nil {
		if strings.TrimSpace(result.Output) != "" {
			_ = os.WriteFile(artifactPath, []byte(result.Output), 0644)
		} else {
			type artifactCollector interface {
				CollectArtifacts(ctx context.Context, jobID string, outputDir string) (map[string]string, error)
			}
			if ac, ok := h.submitter.(artifactCollector); ok {
				if artifacts, err := ac.CollectArtifacts(ctx, result.JobID, filepath.Dir(artifactPath)); err == nil {
					if p, ok := artifacts["plan.json"]; ok {
						_ = os.Rename(p, artifactPath)
					}
				}
			}
		}
	}
	var plan PlanResult
	if err := readJobArtifact(artifactPath, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}
