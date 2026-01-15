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

	"github.com/iw2rmb/ploy/internal/cli/follow"
	"github.com/iw2rmb/ploy/internal/cli/mods"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// buildRunRequest constructs the initial run submission request from CLI flags and defaults.
// It mirrors the control-plane RunSubmitRequest schema:
//   - repo_url, base_ref, target_ref (required by server)
//   - spec: JSON payload built from --spec and CLI overrides
//   - created_by: populated from USER when available
func buildRunRequest(flags *modRunFlags) (modsapi.RunSubmitRequest, error) {
	// Build spec payload from file and CLI overrides (env, image, retain, GitLab flags).
	specFile := ""
	if flags.SpecFile != nil {
		specFile = strings.TrimSpace(*flags.SpecFile)
	}
	var modEnvs []string
	if flags.ModEnvs != nil {
		modEnvs = append(modEnvs, (*flags.ModEnvs)...)
	}
	modImage := ""
	if flags.ModImage != nil {
		modImage = strings.TrimSpace(*flags.ModImage)
	}
	retain := false
	if flags.Retain != nil {
		retain = *flags.Retain
	}
	modCommand := ""
	if flags.ModCommand != nil {
		modCommand = strings.TrimSpace(*flags.ModCommand)
	}
	gitlabPAT := ""
	if flags.GitLabPAT != nil {
		gitlabPAT = strings.TrimSpace(*flags.GitLabPAT)
	}
	gitlabDomain := ""
	if flags.GitLabDomain != nil {
		gitlabDomain = strings.TrimSpace(*flags.GitLabDomain)
	}
	mrSuccess := false
	if flags.MRSuccess != nil {
		mrSuccess = *flags.MRSuccess
	}
	mrFail := false
	if flags.MRFail != nil {
		mrFail = *flags.MRFail
	}

	specPayload, err := buildSpecPayload(
		specFile,
		modEnvs,
		modImage,
		retain,
		modCommand,
		gitlabPAT,
		gitlabDomain,
		mrSuccess,
		mrFail,
	)
	if err != nil {
		return modsapi.RunSubmitRequest{}, err
	}

	repoURL := strings.TrimSpace(*flags.RepoURL)
	baseRef := ""
	if flags.RepoBaseRef != nil {
		baseRef = strings.TrimSpace(*flags.RepoBaseRef)
	}
	targetRef := strings.TrimSpace(*flags.RepoTargetRef)

	typedRepoURL := domaintypes.RepoURL(repoURL)
	if err := typedRepoURL.Validate(); err != nil {
		return modsapi.RunSubmitRequest{}, fmt.Errorf("repo_url: %w", err)
	}
	typedBaseRef := domaintypes.GitRef(baseRef)
	if err := typedBaseRef.Validate(); err != nil {
		return modsapi.RunSubmitRequest{}, fmt.Errorf("base_ref: %w", err)
	}
	typedTargetRef := domaintypes.GitRef(targetRef)
	if err := typedTargetRef.Validate(); err != nil {
		return modsapi.RunSubmitRequest{}, fmt.Errorf("target_ref: %w", err)
	}

	request := modsapi.RunSubmitRequest{
		RepoURL:   typedRepoURL,
		BaseRef:   typedBaseRef,
		TargetRef: typedTargetRef,
		CreatedBy: strings.TrimSpace(os.Getenv("USER")),
	}

	if len(specPayload) > 0 {
		request.Spec = specPayload
	} else {
		request.Spec = []byte("{}")
	}

	return request, nil
}

// submitRun sends the run request to the control plane and returns the initial summary.
// Uses POST /v1/runs for submission and GET /v1/runs/{id}/status for the canonical RunSummary.
func submitRun(ctx context.Context, base *url.URL, httpClient *http.Client, request modsapi.RunSubmitRequest) (modsapi.RunSummary, error) {
	cmd := mods.SubmitCommand{
		Client:  httpClient,
		BaseURL: base,
		Request: request,
	}
	return cmd.Run(ctx)
}

// followRunEvents displays a job graph per repo until the run reaches a terminal state or timeout.
// Returns the final run state and any errors encountered during the follow loop.
// The job graph shows step index, job type, job ID, display name, status glyph, duration, and status.
func followRunEvents(ctx context.Context, base *url.URL, httpClient *http.Client, runID string, flags *modRunFlags, stderr io.Writer) (modsapi.RunState, error) {
	followCtx := ctx
	var cancel context.CancelFunc
	if *flags.CapDuration > 0 {
		followCtx, cancel = context.WithTimeout(ctx, *flags.CapDuration)
		defer cancel()
	}

	engine := follow.NewEngine(cloneForStream(httpClient), base, domaintypes.RunID(runID), follow.Config{
		MaxRetries: *flags.MaxRetries,
		Output:     stderr,
	})

	final, err := engine.Run(followCtx)
	if err != nil {
		// Handle timeout with optional cancellation.
		if *flags.CapDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
			if *flags.CancelOnCap {
				_, _ = fmt.Fprintln(stderr, "Follow timed out; requesting run cancellation...")
				_ = runs.CancelCommand{
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
