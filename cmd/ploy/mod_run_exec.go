// Package main implements the ploy CLI.
//
// The CLI provides commands for mods execution, rollouts, server deployment,
// and related utilities. Package-level documentation lives here so `go doc
// github.com/iw2rmb/ploy/cmd/ploy` renders a clear overview for users.
//
// mod_run_exec.go contains the core mod run execution orchestration.
// This file owns executeModRun which orchestrates the end-to-end mod run
// flow: spec building, run submission, optional follow mode, and artifact
// download. It delegates to specialized files for spec parsing, artifact
// fetching, and flag handling. The orchestrator maintains the high-level
// execution flow while keeping domain-specific logic in separate files.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/logs"
	"github.com/iw2rmb/ploy/internal/cli/mods"
	"github.com/iw2rmb/ploy/internal/cli/stream"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	modplan "github.com/iw2rmb/ploy/internal/workflow/mods/plan"
)

// defaultStageDefinitions returns the standard workflow stages for a mod run.
// These stages represent the typical mods execution pipeline:
// 1. Plan stage: determines which modifications are needed
// 2. ORW stages: apply and generate OpenRewrite transformations
// 3. LLM stages: plan and execute LLM-based modifications
// When the control plane handles the simplified /v1/mods submission flow, it
// creates stages from the spec (mod/mods[]) but reuses these logical stage
// names and lane roles.
func defaultStageDefinitions() []modsapi.StageDefinition {
	return []modsapi.StageDefinition{
		{ID: modplan.StageNamePlan, Lane: "mods-plan", Priority: "default", MaxAttempts: 1},
		{ID: modplan.StageNameORWApply, Lane: "mods-java", Priority: "default", MaxAttempts: 1, Dependencies: []string{modplan.StageNamePlan}},
		{ID: modplan.StageNameORWGenerate, Lane: "mods-java", Priority: "default", MaxAttempts: 1, Dependencies: []string{modplan.StageNamePlan}},
		{ID: modplan.StageNameLLMPlan, Lane: "mods-llm", Priority: "default", MaxAttempts: 1, Dependencies: []string{modplan.StageNamePlan}},
		{ID: modplan.StageNameLLMExec, Lane: "mods-llm", Priority: "default", MaxAttempts: 1, Dependencies: []string{modplan.StageNameORWApply, modplan.StageNameORWGenerate, modplan.StageNameLLMPlan}},
	}
}

// buildRunRequest constructs the initial run submission request from CLI flags and defaults.
// Repository metadata (base_ref, target_ref, workspace_hint) is attached when provided.
func buildRunRequest(flags *modRunFlags) (modsapi.RunSubmitRequest, error) {
	repoURL := strings.TrimSpace(*flags.RepoURL)
	targetRef := strings.TrimSpace(*flags.RepoTargetRef)

	request := modsapi.RunSubmitRequest{
		Submitter:  strings.TrimSpace(os.Getenv("USER")),
		Repository: repoURL,
		Metadata:   make(map[string]string),
		Stages:     defaultStageDefinitions(),
	}

	// Attach repository metadata when provided.
	if baseRef := strings.TrimSpace(*flags.RepoBaseRef); baseRef != "" {
		request.Metadata["repo_base_ref"] = baseRef
	}
	if targetRef != "" {
		request.Metadata["repo_target_ref"] = targetRef
	}
	if hint := strings.TrimSpace(*flags.RepoWorkspaceHint); hint != "" {
		request.Metadata["repo_workspace_hint"] = hint
	}

	return request, nil
}

// submitRun sends the run request to the control plane and returns the initial summary.
func submitRun(ctx context.Context, base *url.URL, httpClient *http.Client, request modsapi.RunSubmitRequest, specPayload []byte) (modsapi.RunSummary, error) {
	cmd := mods.SubmitCommand{
		Client:  httpClient,
		BaseURL: base,
		Request: request,
		Spec:    specPayload,
	}
	return cmd.Run(ctx)
}

// followRunEvents streams run events until the run reaches a terminal state or timeout.
// Returns the final run state and any errors encountered during streaming.
// When --follow is used, streams both run/stage events and enriched log events using the
// shared log printer for a unified, informative view of the Mods execution.
func followRunEvents(ctx context.Context, base *url.URL, httpClient *http.Client, runID string, flags *modRunFlags, stderr io.Writer) (modsapi.RunState, error) {
	followCtx := ctx
	var cancel context.CancelFunc
	if *flags.CapDuration > 0 {
		followCtx, cancel = context.WithTimeout(ctx, *flags.CapDuration)
		defer cancel()
	}

	// Determine log format from flag; default to structured for unified log output.
	// The format controls how enriched log events are rendered during follow mode.
	logFormat := logs.FormatStructured
	if flags.LogFormat != nil && strings.TrimSpace(*flags.LogFormat) == "raw" {
		logFormat = logs.FormatRaw
	}

	// Create shared log printer to render enriched log events alongside run/stage updates.
	// This provides a consistent, informative view when following a Mods run directly.
	logPrinter := logs.NewPrinter(logFormat, stderr)

	ev := mods.EventsCommand{
		Client: stream.Client{
			HTTPClient:   cloneForStream(httpClient),
			MaxRetries:   *flags.MaxRetries,
			RetryBackoff: *flags.RetryWait,
		},
		BaseURL:    base,
		RunID:      domaintypes.RunID(runID), // Convert to domain type
		Output:     stderr,
		LogPrinter: logPrinter, // Wire unified logs into follow mode.
	}

	final, err := ev.Run(followCtx)
	if err != nil {
		// Handle timeout with optional cancellation.
		if *flags.CapDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
			if *flags.CancelOnCap {
				_, _ = fmt.Fprintln(stderr, "Follow timed out; requesting run cancellation...")
				_ = mods.CancelCommand{
					BaseURL: base,
					Client:  httpClient,
					RunID:   domaintypes.RunID(runID),
					Reason:  "cap exceeded",
					Output:  stderr,
				}.Run(context.Background())
			} else {
				_, _ = fmt.Fprintf(stderr, "Follow capped after %s; run %s continues running in the background.\n", flags.CapDuration.String(), runID)
			}
			return "", nil
		}
		return "", err
	}

	_, _ = fmt.Fprintf(stderr, "Final state: %s\n", strings.ToLower(string(final)))
	if final != modsapi.RunStateSucceeded {
		return final, fmt.Errorf("mod run ended in %s", strings.ToLower(string(final)))
	}

	return final, nil
}

// outputJSONSummary writes a machine-readable JSON summary to stdout when requested.
// Includes run ID, initial and final states, artifact directory, and MR URL (if available).
func outputJSONSummary(ctx context.Context, base *url.URL, httpClient *http.Client, runID domaintypes.RunID, initialState, finalState string, flags *modRunFlags) error {
	// Optional: probe MR URL from run status metadata.
	mrURL, _ := fetchMRURL(ctx, base, httpClient, runID.String())

	type runJSON struct {
		RunID       domaintypes.RunID `json:"run_id"`
		Initial     string            `json:"initial_state,omitempty"`
		Final       string            `json:"final_state,omitempty"`
		ArtifactDir string            `json:"artifact_dir,omitempty"`
		MRURL       string            `json:"mr_url,omitempty"`
	}

	out := runJSON{
		RunID:   runID,
		Initial: initialState,
		Final:   finalState,
	}

	if s := strings.TrimSpace(*flags.ArtifactDir); s != "" {
		out.ArtifactDir = s
	}
	if mrURL != "" {
		out.MRURL = mrURL
	}

	b, _ := json.Marshal(out)
	fmt.Println(string(b))
	return nil
}
