package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	build "github.com/iw2rmb/ploy/internal/build"
	"github.com/iw2rmb/ploy/internal/orchestration"
)

type llmInputsPayload struct {
	Language       string       `json:"language"`
	Lane           string       `json:"lane"`
	LastError      llmLastError `json:"last_error"`
	FirstErrorFile string       `json:"first_error_file,omitempty"`
	FirstErrorLine int          `json:"first_error_line,omitempty"`
	Errors         []llmError   `json:"errors"`
	BuilderLogsKey string       `json:"builder_logs_key,omitempty"`
	BuilderLogsURL string       `json:"builder_logs_url,omitempty"`
}

type llmLastError struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

type llmError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

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
	be := strings.TrimSpace(stringValue(branch.Inputs["build_error"]))
	hasBuildError := be != ""
	builderLogsKey := strings.TrimSpace(stringValue(branch.Inputs["builder_logs_key"]))
	builderLogsURL := strings.TrimSpace(stringValue(branch.Inputs["builder_logs_url"]))
	var parsed []build.ParsedBuildError

	if hasBuildError && be != "" {
		parsed = build.ParseBuildErrors("java", "maven", be)
		if len(parsed) == 0 {
			re := regexp.MustCompile(`([A-Za-z0-9_./\\\-]+\.java):([0-9]+)`) // accept windows and linux seps
			if m := re.FindStringSubmatch(be); len(m) == 3 {
				file := strings.ReplaceAll(m[1], "\\", "/")
				line, _ := strconv.Atoi(m[2])
				parsed = append(parsed, build.ParsedBuildError{File: file, Line: line, Column: 0, Message: "compile error"})
			}
		}
	}

	// Fallback: fetch builder logs to parse errors if build_error lacked details
	var builderLogsBytes []byte
	if len(parsed) == 0 && builderLogsKey != "" {
		if logs, usedURL, err := downloadBuilderLogs(ctxDir, builderLogsKey, builderLogsURL); err == nil {
			builderLogsBytes = logs
			parsed = build.ParseBuildErrors("java", "maven", string(logs))
			if rep != nil && len(parsed) > 0 {
				_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("parsed compile error from builder logs (%s)", usedURL), Time: time.Now()})
			}
		} else if rep != nil {
			_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "warn", Message: fmt.Sprintf("failed to download builder logs: %v", err), Time: time.Now()})
		}
	}

	normalizeParsedErrors(repoRoot, parsed)

	if hasBuildError && be != "" {
		payload := llmInputsPayload{
			Language: "java",
			Lane:     "",
			LastError: llmLastError{
				Stdout: "",
				Stderr: be,
			},
			BuilderLogsKey: builderLogsKey,
			BuilderLogsURL: builderLogsURL,
		}

		if len(parsed) > 0 {
			payload.FirstErrorFile = parsed[0].File
			payload.FirstErrorLine = parsed[0].Line
		}

		payload.Errors = make([]llmError, 0, len(parsed))
		for _, e := range parsed {
			payload.Errors = append(payload.Errors, llmError{
				File:    e.File,
				Line:    e.Line,
				Column:  e.Column,
				Message: e.Message,
			})
		}
		if len(payload.Errors) == 0 {
			payload.Errors = []llmError{}
		}

		inputsJSON, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return "", err
		}
		inputsJSON = append(inputsJSON, '\n')
		if err := os.WriteFile(filepath.Join(ctxDir, "inputs.json"), inputsJSON, 0644); err != nil {
			return "", err
		}
		if rep != nil {
			_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("prepared inputs.json (bytes=%d)", len(inputsJSON)), Time: time.Now()})
			if payload.FirstErrorFile != "" && payload.FirstErrorLine > 0 {
				_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("first_error file=%s line=%d", payload.FirstErrorFile, payload.FirstErrorLine), Time: time.Now()})
			}
		}

		// Ensure builder logs are persisted alongside context for debuggability when fetched
		if len(builderLogsBytes) > 0 {
			_ = os.WriteFile(filepath.Join(ctxDir, "builder.log"), builderLogsBytes, 0644)
		}

		// Collect a small set of related sources to aid diffing
		seen := make(map[string]struct{})
		paths := extractJavaPathsFromError(be, 5)
		for _, e := range parsed {
			if strings.TrimSpace(e.File) != "" {
				paths = append(paths, e.File)
			}
		}
		if len(paths) == 0 && len(builderLogsBytes) > 0 {
			paths = append(paths, extractJavaPathsFromError(string(builderLogsBytes), 5)...)
		}
		if len(paths) == 0 {
			classNames := parseClassNamesFromError(be, 5)
			guessed := findJavaFilesByBasename(repoRoot, classNames, 5)
			paths = append(paths, guessed...)
		}
		var manifest []string
		for _, cand := range paths {
			matched := false
			for _, rel := range candidateRepoRelativePaths(repoRoot, cand) {
				if copySourceIfExists(ctxDir, repoRoot, rel, seen, &manifest) {
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			base := strings.TrimSuffix(filepath.Base(cand), filepath.Ext(cand))
			if base == "" {
				continue
			}
			for _, rel := range findJavaFilesByBasename(repoRoot, []string{base}, 3) {
				if copySourceIfExists(ctxDir, repoRoot, rel, seen, &manifest) {
					break
				}
			}
		}
		if len(manifest) > 0 {
			_ = os.WriteFile(filepath.Join(ctxDir, "source_manifest.txt"), []byte(strings.Join(manifest, "\n")+"\n"), 0644)
			if rep != nil {
				show := manifest
				if len(show) > 3 {
					show = show[:3]
				}
				_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("collected sources: %s", strings.Join(show, ", ")), Time: time.Now()})
			}
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

func stringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func downloadBuilderLogs(ctxDir, key, directURL string) ([]byte, string, error) {
	var urls []string
	if strings.TrimSpace(key) != "" {
		seaweed := strings.TrimSpace(ResolveInfraFromEnv().SeaweedURL)
		if seaweed != "" {
			url := strings.TrimRight(seaweed, "/") + "/artifacts/" + strings.TrimLeft(key, "/")
			urls = append(urls, url)
		}
	}
	if strings.TrimSpace(directURL) != "" {
		urls = append(urls, directURL)
	}
	if len(urls) == 0 {
		return nil, "", fmt.Errorf("no builder logs endpoint available")
	}
	dest := filepath.Join(ctxDir, "builder.log")
	var lastErr error
	for _, u := range urls {
		err := downloadToFileFn(u, dest)
		if err == nil {
			data, err := os.ReadFile(dest)
			if err != nil {
				return nil, u, err
			}
			return data, u, nil
		}
		lastErr = err
	}
	_ = os.Remove(dest)
	return nil, urls[len(urls)-1], fmt.Errorf("builder logs download failed: %w", lastErr)
}

func normalizeParsedErrors(repoRoot string, errs []build.ParsedBuildError) {
	for i := range errs {
		file := strings.TrimSpace(errs[i].File)
		if file == "" {
			continue
		}
		candidates := candidateRepoRelativePaths(repoRoot, file)
		var chosen string
		for _, rel := range candidates {
			rel = filepath.ToSlash(strings.TrimSpace(rel))
			if rel == "" {
				continue
			}
			if chosen == "" {
				chosen = rel
			}
			if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel))); err == nil {
				chosen = rel
				break
			}
		}
		if chosen == "" {
			chosen = strings.TrimPrefix(strings.ReplaceAll(file, "\\", "/"), "/")
		}
		chosen = strings.TrimPrefix(chosen, "/")
		if strings.HasPrefix(chosen, "src/src/") {
			chosen = strings.TrimPrefix(chosen, "src/")
		}
		errs[i].File = chosen
	}
}

func candidateRepoRelativePaths(repoRoot, cand string) []string {
	repoClean := filepath.ToSlash(filepath.Clean(repoRoot))
	pathClean := filepath.ToSlash(filepath.Clean(cand))
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		p = strings.TrimPrefix(p, "./")
		p = strings.TrimPrefix(p, ".\\")
		p = strings.TrimPrefix(p, "/")
		p = strings.TrimPrefix(p, "\\")
		if p == "" {
			return
		}
		p = filepath.ToSlash(p)
		for _, existing := range out {
			if existing == p {
				return
			}
		}
		out = append(out, p)
	}

	add(pathClean)
	if strings.HasPrefix(pathClean, repoClean+"/") {
		add(pathClean[len(repoClean)+1:])
	}
	if idx := strings.Index(pathClean, "/repo/"); idx >= 0 {
		add(pathClean[idx+len("/repo/"):])
	}
	if idx := strings.Index(pathClean, "/src/"); idx >= 0 {
		add(pathClean[idx+1:])
	}
	return out
}

func copySourceIfExists(ctxDir, repoRoot, rel string, seen map[string]struct{}, manifest *[]string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		return false
	}
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, ".\\")
	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimPrefix(rel, "\\")
	if rel == "" {
		return false
	}
	if _, ok := seen[rel]; ok {
		return false
	}
	srcAbs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	b, err := os.ReadFile(srcAbs)
	if err != nil {
		return false
	}
	dst := filepath.Join(ctxDir, "sources", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return false
	}
	if err := os.WriteFile(dst, b, 0644); err != nil {
		return false
	}
	*manifest = append(*manifest, rel)
	seen[rel] = struct{}{}
	return true
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
	infra := ResolveInfraFromEnv()
	dlStart := time.Now()
	if rep != nil {
		_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("download start: key=%s start_ts=%s", key, dlStart.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
	}
	// Wait for job to report upload before starting download loop
	if infra.Controller != "" {
		_ = waitForStepContainingFn(infra.Controller, modID, "llm-exec", "uploaded diff to", 120*time.Second)
	}
	// Event observed — wait for filer to index object using lightweight HEAD with backoff
	// Try up to 30s with jitter
	for i := 0; i < 30; i++ {
		if headURLFn(url) {
			break
		}
		// jittered sleep between 300–700ms
		time.Sleep(300*time.Millisecond + time.Duration(i%5)*80*time.Millisecond)
	}
	// Single download attempt after HEAD reports ready (or timeout)
	dlErr := downloadToFileFn(url, diffPath)
	dlEnd := time.Now()
	if dlErr != nil {
		if infra.Controller != "" {
			fallbackURL := strings.TrimRight(infra.Controller, "/") + "/mods/" + modID + "/artifacts/diff_patch"
			if err2 := downloadToFileFn(fallbackURL, diffPath); err2 == nil {
				dlEnd = time.Now()
				if rep != nil {
					_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "info", Message: fmt.Sprintf("download fallback succeeded: key=%s via controller start_ts=%s end_ts=%s", key, dlStart.UTC().Format(time.RFC3339Nano), dlEnd.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
				}
				dlErr = nil
			} else {
				if rep != nil {
					_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "error", Message: fmt.Sprintf("download failed: key=%s error=%v fallback_error=%v start_ts=%s end_ts=%s", key, dlErr, err2, dlStart.UTC().Format(time.RFC3339Nano), dlEnd.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
				}
				return fmt.Errorf("LLM exec diff download failed: %v (controller fallback: %v)", dlErr, err2)
			}
		} else {
			if rep != nil {
				_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "error", Message: fmt.Sprintf("download failed: key=%s error=%v start_ts=%s end_ts=%s", key, dlErr, dlStart.UTC().Format(time.RFC3339Nano), dlEnd.UTC().Format(time.RFC3339Nano)), Time: time.Now()})
			}
			return fmt.Errorf("LLM exec diff download failed: %v", dlErr)
		}
	}
	allowed := ResolveDefaultsFromEnv().Allowlist
	if err := validateDiffPathsFn(diffPath, allowed); err != nil {
		if rep != nil {
			_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "error", Message: fmt.Sprintf("download failed validation: key=%s error=%v", key, err), Time: time.Now()})
		}
		return fmt.Errorf("LLM exec diff download failed validation: %w", err)
	}
	if data, readErr := os.ReadFile(diffPath); readErr == nil {
		if strings.Contains(string(data), ".llm-healing") {
			if rep != nil {
				_ = rep.Report(ctx, Event{Phase: "llm-exec", Step: "llm-exec", Level: "warn", Message: "downloaded diff only touched placeholder .llm-healing", Time: time.Now()})
			}
			return fmt.Errorf("LLM exec diff contained placeholder .llm-healing marker")
		}
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
