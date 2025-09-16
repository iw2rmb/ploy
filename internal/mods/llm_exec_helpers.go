package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/orchestration"
)

// llmPrepareContext builds a context directory under baseDir for LLM exec and
// populates inputs.json, optional source files, precomputed diff, and delete_paths.
// Returns the context directory path.
func llmPrepareContext(baseDir string, branch BranchSpec, repoRoot string, rep EventReporter, ctx context.Context) (string, error) {
	ctxDir := filepath.Join(baseDir, "context")
	if err := os.MkdirAll(ctxDir, 0755); err != nil {
		return "", err
	}
	_ = os.WriteFile(filepath.Join(ctxDir, ".keep"), []byte("llm-context"), 0644)

	// Inject inputs.json with last_error if provided
	if be, ok := branch.Inputs["build_error"].(string); ok && strings.TrimSpace(be) != "" {
		parsed := ParseBuildErrors("java", "maven", be)
		var b strings.Builder
		b.WriteString("{\n  \"language\": \"java\",\n  \"lane\": \"\",\n  \"last_error\": {\n    \"stdout\": \"\",\n    \"stderr\": ")
		b.WriteString(strconv.Quote(be))
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
		b.WriteString("]\n}\n")
		inputsJSON := b.String()
		_ = os.WriteFile(filepath.Join(ctxDir, "inputs.json"), []byte(inputsJSON), 0644)
		if rep != nil {
			_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("prepared inputs.json (bytes=%d)", len(inputsJSON)), Time: time.Now()})
		}
		// Collect a small set of related sources to aid diffing
		seen := make(map[string]struct{})
		paths := extractJavaPathsFromError(be, 5)
		if len(paths) == 0 {
			classNames := parseClassNamesFromError(be, 5)
			guessed := findJavaFilesByBasename(repoRoot, classNames, 5)
			paths = append(paths, guessed...)
		}
		var manifest []string
		for _, cand := range paths {
			rel := cand
			if strings.HasPrefix(rel, repoRoot+string(os.PathSeparator)) {
				rel = strings.TrimPrefix(rel, repoRoot+string(os.PathSeparator))
			}
			if _, ok := seen[rel]; ok || strings.TrimSpace(rel) == "" {
				continue
			}
			seen[rel] = struct{}{}
			srcAbs := filepath.Join(repoRoot, rel)
			if b, err := os.ReadFile(srcAbs); err == nil {
				dst := filepath.Join(ctxDir, "sources", rel)
				_ = os.MkdirAll(filepath.Dir(dst), 0755)
				_ = os.WriteFile(dst, b, 0644)
				manifest = append(manifest, rel)
			}
		}
		if len(manifest) > 0 {
			_ = os.WriteFile(filepath.Join(ctxDir, "source_manifest.txt"), []byte(strings.Join(manifest, "\n")+"\n"), 0644)
		}
	}

	// Optional: precomputed diff and delete paths
	if v, ok := branch.Inputs["precomputed_diff"].(string); ok && strings.TrimSpace(v) != "" {
		_ = os.WriteFile(filepath.Join(ctxDir, "diff.patch"), []byte(v), 0644)
	}
	if arr, ok := branch.Inputs["delete_paths"].([]string); ok && len(arr) > 0 {
		var b strings.Builder
		for _, p := range arr {
			if strings.TrimSpace(p) != "" {
				b.WriteString(p)
				b.WriteString("\n")
			}
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

	return ctxDir, nil
}

// llmMakeVars builds template variable map for LLM exec.
func llmMakeVars(baseDir string) map[string]string {
	imgs := ResolveImagesFromEnv()
	infra := ResolveInfraFromEnv()
	llm := ResolveLLMDefaultsFromEnv()
	modID := os.Getenv("MOD_ID")
	if modID != "" && !strings.HasPrefix(modID, "mod-") {
		modID = "mod-" + modID
	}
	return map[string]string{
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
}

// llmPersistSubmittedHCL saves a copy of the submitted HCL for diagnostics and reports an event.
func llmPersistSubmittedHCL(renderedHCLPath, branchID, runID string, rep EventReporter, ctx context.Context) {
	if modID := os.Getenv("MOD_ID"); modID != "" {
		persistDir := filepath.Join("/tmp/mods-submitted", modID, "llm-exec", branchID)
		_ = os.MkdirAll(persistDir, 0755)
		dest := filepath.Join(persistDir, "llm_exec.submitted.hcl")
		if b, e := os.ReadFile(renderedHCLPath); e == nil {
			_ = os.WriteFile(dest, b, 0644)
			if rep != nil {
				_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("Saved submitted HCL to %s", dest), JobName: runID, Time: time.Now()})
			}
		}
	}
}

// llmValidateAndSubmit validates HCL and submits the job using HCLSubmitter.
func llmValidateAndSubmit(ctx context.Context, h HCLSubmitter, renderedHCLPath, runID string, rep EventReporter) error {
	if h != nil {
		if err := h.Validate(renderedHCLPath); err != nil {
			return fmt.Errorf("LLM exec HCL validation failed: %w", err)
		}
	}
	timeout := ResolveDefaultsFromEnv().LLMExecTimeout
	if h != nil {
		if err := h.SubmitCtx(ctx, renderedHCLPath, timeout); err != nil {
			if rep != nil {
				_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "error", Message: fmt.Sprintf("submission failed: %v", err), JobName: runID, Time: time.Now()})
			}
			_ = orchestration.DeregisterJob(runID, true)
			return fmt.Errorf("LLM exec job failed: %w", err)
		}
		return nil
	}
	return fmt.Errorf("LLM exec job failed: no HCL submitter in test mode")
}

// llmFetchDiffIfProd downloads diff.patch from SeaweedFS in prod mode and reports progress.
func llmFetchDiffIfProd(ctx context.Context, rep EventReporter, seaweedURL, modID, branchID, runID, renderedHCLPath string) error {
	diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	_ = os.MkdirAll(filepath.Dir(diffPath), 0755)
	if seaweedURL == "" || modID == "" {
		return fmt.Errorf("LLM exec missing SeaweedFS URL or execution ID for artifact fetch")
	}
	key := computeBranchDiffKey(modID, branchID, runID)
	url := strings.TrimRight(seaweedURL, "/") + "/artifacts/" + key
	dlStart := time.Now()
	if rep != nil {
		_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("download start: key=%s start_ts=%s", key, dlStart.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
	}
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
		if rep != nil {
			_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "error", Message: fmt.Sprintf("download failed: key=%s error=%v start_ts=%s end_ts=%s", key, dlErr, dlStart.UTC().Format(time.RFC3339Nano), dlEnd.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
		}
		return fmt.Errorf("LLM exec diff download failed: %v", dlErr)
	}
	if rep != nil {
		var sz int64
		if fi, err := os.Stat(diffPath); err == nil {
			sz = fi.Size()
		}
		_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("download succeeded: key=%s bytes=%d start_ts=%s end_ts=%s", key, sz, dlStart.UTC().Format(time.RFC3339Nano), dlEnd.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
	}
	return nil
}

// llmUploadDiffAndMeta best-effort uploads diff.patch and writes chain metadata.
func llmUploadDiffAndMeta(seaweedURL, modID, branchID, stepID, renderedHCLPath string) {
	if seaweedURL == "" || modID == "" {
		return
	}
	diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	diffKey := computeBranchDiffKey(modID, branchID, stepID)
	_ = putFileFn(seaweedURL, diffKey, diffPath, "text/plain")
	_ = writeBranchChainStepMeta(seaweedURL, modID, branchID, stepID, diffKey)
}
