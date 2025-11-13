package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"net/url"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/nodeagent/gitlab"
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// Runtime component factory methods.
// These methods isolate component initialization logic from the orchestration flow.

// createGitFetcher initializes a git fetcher for repository operations.
func (r *runController) createGitFetcher() (step.GitFetcher, error) {
	return hydration.NewGitFetcher(hydration.GitFetcherOptions{PublishSnapshot: false})
}

// createWorkspaceHydrator initializes a workspace hydrator with the provided repo fetcher.
func (r *runController) createWorkspaceHydrator(fetcher step.GitFetcher) (step.WorkspaceHydrator, error) {
	return step.NewFilesystemWorkspaceHydrator(step.FilesystemWorkspaceHydratorOptions{
		RepoFetcher: fetcher,
	})
}

// createContainerRuntime initializes a Docker container runtime with image pull enabled.
func (r *runController) createContainerRuntime() (step.ContainerRuntime, error) {
	return step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{
		PullImage: true,
	})
}

// createDiffGenerator initializes a filesystem diff generator.
func (r *runController) createDiffGenerator() step.DiffGenerator {
	return step.NewFilesystemDiffGenerator(step.FilesystemDiffGeneratorOptions{})
}

// shouldCreateMR determines if an MR should be created based on terminal status and manifest options.
func shouldCreateMR(terminalStatus string, manifest contracts.StepManifest) bool {
	if terminalStatus == "succeeded" {
		if mrOnSuccess, ok := manifest.OptionBool("mr_on_success"); ok && mrOnSuccess {
			return true
		}
	}
	if terminalStatus == "failed" {
		if mrOnFail, ok := manifest.OptionBool("mr_on_fail"); ok && mrOnFail {
			return true
		}
	}
	return false
}

// createMR pushes the branch and creates a GitLab merge request.
func (r *runController) createMR(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, workspaceRoot string) (string, error) {
	// Extract GitLab options.
	gitlabPAT, _ := manifest.OptionString("gitlab_pat")
	gitlabDomain, _ := manifest.OptionString("gitlab_domain")

	// Validate required fields.
	if strings.TrimSpace(gitlabPAT) == "" {
		return "", fmt.Errorf("gitlab_pat is required for MR creation")
	}
	// Normalize domain: accept either host or full URL; coerce to host for MR client.
	gitlabDomain = strings.TrimSpace(gitlabDomain)
	if gitlabDomain == "" {
		gitlabDomain = "gitlab.com"
	} else {
		if strings.HasPrefix(gitlabDomain, "http://") || strings.HasPrefix(gitlabDomain, "https://") {
			if u, err := url.Parse(gitlabDomain); err == nil && u.Host != "" {
				gitlabDomain = u.Host
			}
		}
		// Remove any trailing slash artifacts.
		gitlabDomain = strings.TrimSuffix(gitlabDomain, "/")
	}

	// Extract project ID from repo URL.
	projectID, err := extractProjectIDFromRepoURL(req.RepoURL.String())
	if err != nil {
		return "", fmt.Errorf("extract project id: %w", err)
	}

	// Use a unique source branch per run: ploy-<ticket-id>.
	// This avoids MR conflicts on repeated runs regardless of the submitted target ref.
	sourceBranch := fmt.Sprintf("ploy-%s", req.RunID)

	// Create a commit with any workspace changes before pushing.
	if committed, cerr := git.EnsureCommit(ctx, workspaceRoot, "ploy-bot", "ploy-bot@ploy.local", fmt.Sprintf("Ploy: apply changes for run %s", req.RunID)); cerr != nil {
		slog.Error("git commit failed", "run_id", req.RunID, "error", cerr)
	} else if !committed {
		slog.Info("no changes detected; proceeding to push branch without commit", "run_id", req.RunID)
	}

	// Push branch to origin using git push (Phase E).
	pusher := git.NewPusher()
	pushOpts := git.PushOptions{
		RepoDir:   workspaceRoot,
		TargetRef: sourceBranch,
		PAT:       gitlabPAT,
		UserName:  "ploy-bot",
		UserEmail: "ploy-bot@ploy.local",
		RemoteURL: req.RepoURL.String(),
	}

	slog.Info("pushing branch to origin", "run_id", req.RunID, "source_branch", sourceBranch, "submitted_target", req.TargetRef)
	if err := pusher.Push(ctx, pushOpts); err != nil {
		return "", fmt.Errorf("git push: %w", err)
	}

	// Create MR via GitLab API.
	mrClient := gitlab.NewMRClient()
	mrReq := gitlab.MRCreateRequest{
		Domain:       gitlabDomain,
		ProjectID:    projectID,
		PAT:          gitlabPAT,
		Title:        fmt.Sprintf("Ploy: %s", req.RunID),
		SourceBranch: sourceBranch,
		TargetBranch: req.BaseRef.String(),
		Description:  fmt.Sprintf("Automated changes from Ploy run %s", req.RunID),
		Labels:       "ploy",
	}

	slog.Info("creating merge request", "run_id", req.RunID, "source", sourceBranch, "target", req.BaseRef)
	mrURL, err := mrClient.CreateMR(ctx, mrReq)
	if err != nil {
		return "", fmt.Errorf("create mr: %w", err)
	}

	return mrURL, nil
}

// extractProjectIDFromRepoURL extracts the URL-encoded project ID from a GitLab repo URL.
func extractProjectIDFromRepoURL(repoURL string) (string, error) {
	return gitlab.ExtractProjectIDFromURL(repoURL)
}

// gateRunMetadata captures gate execution metadata and timing for stats reporting.
type gateRunMetadata struct {
	Metadata   *contracts.BuildGateStageMetadata
	DurationMs int64
}

// executionResult wraps step.Result with additional gate run history for stats.
type executionResult struct {
	step.Result
	// PreGate captures the initial gate run metadata (if gate was executed).
	PreGate *gateRunMetadata
	// ReGates captures re-gate attempts after healing (if healing was attempted).
	ReGates []gateRunMetadata
}

// executeWithHealing runs the main step with optional healing loop when the build gate fails.
// It handles the gate-heal-regate orchestration as specified in build_gate_healing options.
func (r *runController) executeWithHealing(
	ctx context.Context,
	runner step.Runner,
	req StartRunRequest,
	manifest contracts.StepManifest,
	workspace string,
	outDir string,
	inDir *string,
) (executionResult, error) {
	// First execution attempt (includes pre-mod gate check).
	result, err := runner.Run(ctx, step.Request{
		TicketID:  types.TicketID(req.RunID),
		Manifest:  manifest,
		Workspace: workspace,
		OutDir:    outDir,
		InDir:     *inDir,
	})

	// Capture pre-gate metadata for stats (if gate was executed).
	var preGate *gateRunMetadata
	if result.BuildGate != nil {
		preGate = &gateRunMetadata{
			Metadata:   result.BuildGate,
			DurationMs: result.Timings.BuildGateDuration.Milliseconds(),
		}
	}

	// If execution succeeded or error is not a build gate failure, return immediately.
	if err == nil || !errors.Is(err, step.ErrBuildGateFailed) {
		return executionResult{Result: result, PreGate: preGate}, err
	}

	// Build gate failed. Check if healing is configured.
	healingConfig, hasHealing := req.Options["build_gate_healing"].(map[string]any)
	if !hasHealing {
		// No healing configured; return the gate failure.
		return executionResult{Result: result, PreGate: preGate}, err
	}

	// Extract healing parameters.
	retries := 1 // Default to 1 retry
	if r, ok := healingConfig["retries"].(int); ok && r > 0 {
		retries = r
	} else if rf, ok := healingConfig["retries"].(float64); ok && rf > 0 {
		retries = int(rf)
	}

	healingMods, ok := healingConfig["mods"].([]any)
	if !ok || len(healingMods) == 0 {
		slog.Warn("build_gate_healing configured but no mods provided", "run_id", req.RunID)
		return executionResult{Result: result, PreGate: preGate}, err
	}

	// Create /in directory if not already created (for build-gate.log).
	if *inDir == "" {
		tmpInDir, dirErr := os.MkdirTemp("", "ploy-mod-in-*")
		if dirErr != nil {
			slog.Error("failed to create /in directory for healing", "run_id", req.RunID, "error", dirErr)
			return executionResult{Result: result, PreGate: preGate}, err
		}
		*inDir = tmpInDir
		// Caller handles cleanup via defer.

		// Write build-gate.log to /in for healing containers.
		if result.BuildGate != nil && result.BuildGate.LogsText != "" {
			inLogPath := filepath.Join(*inDir, "build-gate.log")
			if writeErr := os.WriteFile(inLogPath, []byte(result.BuildGate.LogsText), 0o644); writeErr != nil {
				slog.Warn("failed to write /in/build-gate.log", "run_id", req.RunID, "error", writeErr)
			} else {
				slog.Info("build-gate.log persisted to /in for healing", "run_id", req.RunID, "path", inLogPath)
			}
		}
	}

	// Track re-gate runs for stats.
	var reGates []gateRunMetadata

	// Attempt healing loop.
	for attempt := 1; attempt <= retries; attempt++ {
		slog.Info("starting healing attempt", "run_id", req.RunID, "attempt", attempt, "max_retries", retries)

		// Execute each healing mod in sequence.
		for idx, modEntry := range healingMods {
			healManifest, buildErr := buildHealingManifest(req, modEntry, idx)
			if buildErr != nil {
				slog.Error("failed to build healing manifest", "run_id", req.RunID, "mod_index", idx, "error", buildErr)
				return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("build healing manifest[%d]: %w", idx, buildErr)
			}

			slog.Info("executing healing mod", "run_id", req.RunID, "attempt", attempt, "mod_index", idx, "image", healManifest.Image)

			// Provide host workspace path for in-container build verification tools.
			if healManifest.Env == nil {
				healManifest.Env = map[string]string{}
			}
			healManifest.Env["PLOY_HOST_WORKSPACE"] = workspace

			// Inject server connection details for buildgate API access from healing containers.
			healManifest.Env["PLOY_SERVER_URL"] = r.cfg.ServerURL
			healManifest.Env["PLOY_CA_CERT_PATH"] = "/etc/ploy/certs/ca.crt"
			healManifest.Env["PLOY_CLIENT_CERT_PATH"] = "/etc/ploy/certs/client.crt"
			healManifest.Env["PLOY_CLIENT_KEY_PATH"] = "/etc/ploy/certs/client.key"

			// Mount node's TLS certificates into healing container for buildgate API access.
			if healManifest.Options == nil {
				healManifest.Options = make(map[string]any)
			}
			healManifest.Options["ploy_ca_cert_path"] = r.cfg.HTTP.TLS.CAPath
			healManifest.Options["ploy_client_cert_path"] = r.cfg.HTTP.TLS.CertPath
			healManifest.Options["ploy_client_key_path"] = r.cfg.HTTP.TLS.KeyPath

			// Run the healing mod container.
			healResult, healErr := runner.Run(ctx, step.Request{
				TicketID:  types.TicketID(req.RunID),
				Manifest:  healManifest,
				Workspace: workspace,
				OutDir:    outDir,
				InDir:     *inDir,
			})

			if healErr != nil {
				slog.Error("healing mod execution failed", "run_id", req.RunID, "mod_index", idx, "error", healErr)
				return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("healing mod[%d] failed: %w", idx, healErr)
			}

			if healResult.ExitCode != 0 {
				slog.Warn("healing mod exited with non-zero code", "run_id", req.RunID, "mod_index", idx, "exit_code", healResult.ExitCode)
				// Continue with remaining mods; we'll check gate after all mods run.
			}

			// Upload /out artifacts for this healing mod if present.
			// Use centralized options accessor for stage_id when re-gating.
			stageID, _ := manifest.OptionString("stage_id")
			if uploadErr := uploadOutDirIfPresent(ctx, r.cfg, req.RunID.String(), stageID, outDir); uploadErr != nil {
				slog.Warn("failed to upload /out for healing mod", "run_id", req.RunID, "mod_index", idx, "error", uploadErr)
			}
		}

		// Re-run the gate after healing mods.
		slog.Info("re-running build gate after healing", "run_id", req.RunID, "attempt", attempt)

		gateSpec := manifest.Gate
		//lint:ignore SA1019 Backward compatibility: support deprecated Shift by mapping to Gate.
		if gateSpec == nil && manifest.Shift != nil {
			gateSpec = &contracts.StepGateSpec{
				Enabled: manifest.Shift.Enabled, //lint:ignore SA1019 compat field access
				Profile: manifest.Shift.Profile, //lint:ignore SA1019 compat field access
				Env:     manifest.Shift.Env,     //lint:ignore SA1019 compat field access
			}
		}

		if runner.Gate != nil && gateSpec != nil && gateSpec.Enabled {
			regateStart := time.Now()
			gateMetadata, gateErr := runner.Gate.Execute(ctx, gateSpec, workspace)
			regateDuration := time.Since(regateStart)

			if gateErr != nil {
				slog.Error("re-gate execution failed", "run_id", req.RunID, "error", gateErr)
				return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("re-gate execution failed: %w", gateErr)
			}

			result.BuildGate = gateMetadata

			// Capture re-gate metadata for stats.
			reGates = append(reGates, gateRunMetadata{
				Metadata:   gateMetadata,
				DurationMs: regateDuration.Milliseconds(),
			})

			// Check if gate passed.
			gatePassed := false
			if len(gateMetadata.StaticChecks) > 0 {
				gatePassed = gateMetadata.StaticChecks[0].Passed
			}

			if gatePassed {
				slog.Info("build gate passed after healing", "run_id", req.RunID, "attempt", attempt)
				// Gate passed; proceed to main mod execution.
				// Disable the gate and hydration for the follow-up main mod run to
				// avoid cloning the repository a second time in the same workspace.
				manifestForMainMod := manifest
				manifestForMainMod.Gate = &contracts.StepGateSpec{Enabled: false}
				//lint:ignore SA1019 Backward compatibility: also disable deprecated Shift field.
				manifestForMainMod.Shift = nil
				if len(manifestForMainMod.Inputs) > 0 {
					inputs := make([]contracts.StepInput, len(manifestForMainMod.Inputs))
					copy(inputs, manifestForMainMod.Inputs)
					for i := range inputs {
						inputs[i].Hydration = nil
					}
					manifestForMainMod.Inputs = inputs
				}

				// Execute the main mod without re-running gate or hydration.
				mainResult, mainErr := runner.Run(ctx, step.Request{
					TicketID:  types.TicketID(req.RunID),
					Manifest:  manifestForMainMod,
					Workspace: workspace,
					OutDir:    outDir,
					InDir:     *inDir,
				})
				// Return with all gate history.
				return executionResult{Result: mainResult, PreGate: preGate, ReGates: reGates}, mainErr
			}

			// Re-gate failed; continue to next retry or exit when exhausted.
			slog.Warn("build gate still failing after healing", "run_id", req.RunID, "attempt", attempt)
		}
	}

	// Retries exhausted; return the gate failure.
	slog.Error("healing retries exhausted, build gate still failing", "run_id", req.RunID)
	return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("%w: healing retries exhausted", step.ErrBuildGateFailed)
}
