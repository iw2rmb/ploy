package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SubmitReducerJob submits a reducer job with branch results and optional winner
func (h *jobSubmissionHelper) SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error) {
	if h.runner != nil {
		assets, err := h.runner.RenderReducerAssets()
		if err != nil {
			return nil, fmt.Errorf("failed to render reducer assets: %w", err)
		}

		// Write history.json
		var b strings.Builder
		b.WriteString("{\n  \"plan_id\": ")
		b.WriteString(strconv.Quote(planID))
		b.WriteString(",\n  \"branches\": [\n")
		for i, r := range results {
			if i > 0 {
				b.WriteString(",\n")
			}
			b.WriteString("    {")
			b.WriteString("\"id\":")
			b.WriteString(strconv.Quote(r.ID))
			b.WriteString(",")
			b.WriteString("\"status\":")
			b.WriteString(strconv.Quote(r.Status))
			if strings.TrimSpace(r.Notes) != "" {
				b.WriteString(",\"notes\":")
				b.WriteString(strconv.Quote(r.Notes))
			}
			b.WriteString("}")
		}
		b.WriteString("\n  ],\n  \"winner\": ")
		if winner != nil {
			b.WriteString(strconv.Quote(winner.ID))
		} else {
			b.WriteString("\"\"")
		}
		b.WriteString("\n}\n")
		_ = os.WriteFile(assets.HistoryPath, []byte(b.String()), 0644)

		// Prepare vars and HCL
		imgs := ResolveImagesFromEnv()
		infra := ResolveInfraFromEnv()
		runID := ReducerRunID(planID)
		contextDir := filepath.Dir(assets.HistoryPath)
		outDir := filepath.Join(workspace, "reducer", "out")
		vars := map[string]string{
			"MODS_REGISTRY":      imgs.Registry,
			"MODS_PLANNER_IMAGE": imgs.Planner,
			"MODS_REDUCER_IMAGE": imgs.Reducer,
			"PLOY_CONTROLLER":    infra.Controller,
			"MOD_ID":             os.Getenv("MOD_ID"),
			"PLOY_SEAWEEDFS_URL": infra.SeaweedURL,
			"NOMAD_DC":           infra.DC,
			"MODS_CONTEXT_DIR":   contextDir,
			"MODS_OUT_DIR":       outDir,
		}
		// If SeaweedFS is available, tar the reducer context and upload; set MODS_CONTEXT_URL for artifact source
		if infra.SeaweedURL != "" {
			_ = os.WriteFile(filepath.Join(contextDir, ".keep"), []byte("reducer-context"), 0644)
			tarPath := filepath.Join(workspace, "reducer", "context.tar")
			if err := createTarFromDir(contextDir, tarPath); err == nil {
				if modID := os.Getenv("MOD_ID"); modID != "" {
					if !strings.HasPrefix(modID, "mod-") {
						modID = "mod-" + modID
					}
					key := fmt.Sprintf("mods/%s/contexts/%s.tar", modID, runID)
					_ = putFileFn(infra.SeaweedURL, key, tarPath, "application/octet-stream")
					vars["MODS_CONTEXT_URL"] = strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
				}
			}
		}
		renderedHCLPath, err := substituteHCLTemplateWithMCPVars(assets.HCLPath, runID, vars, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to substitute reducer HCL: %w", err)
		}

		// Report and submit
		if controller := ResolveInfraFromEnv().Controller; controller != "" {
			rep := NewControllerEventReporter(controller, os.Getenv("MOD_ID"))
			_ = rep.Report(ctx, Event{Phase: "reducer", Step: "reducer", Level: "info", Message: "job started", JobName: runID, Time: time.Now()})
			reportJobSubmittedAsync(ctx, rep, runID, "reducer", "reducer")
		}
		if err := h.runner.GetHCLSubmitter().Validate(renderedHCLPath); err != nil {
			return nil, fmt.Errorf("reducer HCL validation failed: %w", err)
		}
		timeout := ResolveDefaultsFromEnv().ReducerTimeout
		if err := h.runner.GetHCLSubmitter().SubmitCtx(ctx, renderedHCLPath, timeout); err != nil {
			if controller := ResolveInfraFromEnv().Controller; controller != "" {
				rep := NewControllerEventReporter(controller, os.Getenv("MOD_ID"))
				_ = rep.Report(ctx, Event{Phase: "reducer", Step: "reducer", Level: "error", Message: fmt.Sprintf("job failed: %v", err), JobName: runID, Time: time.Now()})
			}
			return nil, fmt.Errorf("reducer job failed: %w", err)
		}
		// Download and parse next.json
		artifactPath := filepath.Join(workspace, "reducer", "out", "next.json")
		if err := os.MkdirAll(filepath.Dir(artifactPath), 0755); err != nil {
			return nil, err
		}
		modID := os.Getenv("MOD_ID")
		if infra.SeaweedURL == "" || modID == "" {
			return nil, fmt.Errorf("reducer artifact fetch requires SeaweedFS URL and execution ID")
		}
		key := fmt.Sprintf("mods/%s/reducer/%s/next.json", modID, runID)
		url := strings.TrimRight(infra.SeaweedURL, "/") + "/artifacts/" + key
		// Event-driven: wait for upload event, then HEAD+jitter readiness, then single download
		_ = waitForStepContainingFn(infra.Controller, modID, "reducer", "uploaded next to", 120*time.Second)
		for i := 0; i < 30; i++ { // ~2s
			if headURLFn(url) {
				break
			}
			time.Sleep(300*time.Millisecond + time.Duration(i%5)*80*time.Millisecond)
		}
		if err := downloadToFileFn(url, artifactPath); err != nil {
			if infra.Controller != "" {
				fallbackURL := strings.TrimRight(infra.Controller, "/") + "/mods/" + modID + "/artifacts/next_json"
				if err2 := downloadToFileFn(fallbackURL, artifactPath); err2 != nil {
					return nil, fmt.Errorf("failed to download reducer artifact: %w (controller fallback: %v)", err, err2)
				}
			} else {
				return nil, fmt.Errorf("failed to download reducer artifact: %w", err)
			}
		}
		var next NextAction
		if err := readJobArtifact(artifactPath, &next); err != nil {
			return nil, err
		}
		return &next, nil
	}

	// Fallback/mock path
	spec := JobSpec{Name: "reducer", Type: "reducer", Timeout: ResolveDefaultsFromEnv().ReducerTimeout}
	result, err := h.submitter.SubmitAndWaitTerminal(ctx, spec)
	if err != nil {
		return nil, err
	}
	artifactPath := filepath.Join(workspace, "reducer", "out", "next.json")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0755); err == nil {
		type artifactCollector interface {
			CollectArtifacts(ctx context.Context, jobID string, outputDir string) (map[string]string, error)
		}
		if strings.TrimSpace(result.Output) != "" {
			_ = os.WriteFile(artifactPath, []byte(result.Output), 0644)
		} else if ac, ok := h.submitter.(artifactCollector); ok {
			if artifacts, err := ac.CollectArtifacts(ctx, result.JobID, filepath.Dir(artifactPath)); err == nil {
				if p, ok := artifacts["next.json"]; ok {
					_ = os.Rename(p, artifactPath)
				}
			}
		}
	}
	var next NextAction
	if err := readJobArtifact(artifactPath, &next); err != nil {
		return nil, err
	}
	return &next, nil
}
