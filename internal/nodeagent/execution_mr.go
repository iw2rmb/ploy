// execution_mr.go handles GitLab merge request creation for completed runs.
package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/nodeagent/gitlab"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// GitLab merge request operations.
// This file isolates MR creation workflow from core execution orchestration.

// shouldCreateMR determines if an MR should be created based on terminal status and manifest options.
// MR creation is triggered by mr_on_success (for Success runs) or mr_on_fail (for Fail runs).
// Only boolean true values trigger MR creation; non-boolean values are ignored.
// v1 uses capitalized job status values: Success, Fail, Cancelled.
func shouldCreateMR(terminalStatus string, manifest contracts.StepManifest) bool {
	if terminalStatus == JobStatusSuccess.String() {
		if mrOnSuccess, ok := manifest.OptionBool("mr_on_success"); ok && mrOnSuccess {
			return true
		}
	}
	if terminalStatus == JobStatusFail.String() {
		if mrOnFail, ok := manifest.OptionBool("mr_on_fail"); ok && mrOnFail {
			return true
		}
	}
	return false
}

// Abstraction seams for testing. These are narrow wrappers we can swap in tests.
type (
	// pusherIface aliases the git.Pusher interface for local indirection in tests.
	pusherIface = git.Pusher
	// pushOptions aliases git.PushOptions for test fakes without importing git.
	pushOptions = git.PushOptions

	// mrCreateReq is an alias for the GitLab MR create DTO.
	mrCreateReq = gitlab.MRCreateRequest
	// mrCreator captures just the CreateMR method used by this file.
	mrCreator interface {
		CreateMR(ctx context.Context, req gitlab.MRCreateRequest) (string, error)
	}
)

var (
	// newPusher is a factory function for creating git pushers.
	// Indirection allows test mocking of git.NewPusher.
	newPusher   = git.NewPusher
	newMRClient = func() mrCreator { return gitlab.NewMRClient() }
)

// createMR pushes the branch and creates a GitLab merge request.
// This method performs the following steps:
// 1. Validates GitLab credentials and normalizes the domain
// 2. Extracts the project ID from the repository URL
// 3. Creates a unique source branch name (ploy/{run_name|run_id})
// 4. Commits any workspace changes with a standard commit message
// 5. Pushes the branch to origin using the provided PAT
// 6. Creates a merge request via GitLab API with standardized metadata
//
// Returns the MR URL on success, or an error if any step fails.
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

	// Determine source branch for MR:
	//   - When TargetRef is provided, use it as-is (caller-managed branch name).
	//   - When TargetRef is empty, derive a unique per-run branch:
	//       ploy/<run_name> when a run name is set,
	//       otherwise ploy/<run-id>.
	// This matches the control plane contract where missing target_ref signals that
	// downstream components should synthesize a branch name based on the run identity.
	sourceBranch := strings.TrimSpace(req.TargetRef.String())
	if sourceBranch == "" {
		if runName := strings.TrimSpace(req.Name); runName != "" {
			sourceBranch = fmt.Sprintf("ploy/%s", runName)
		} else {
			sourceBranch = fmt.Sprintf("ploy/%s", req.RunID.String())
		}
	}

	// Create a commit with any workspace changes before pushing.
	if committed, cerr := git.EnsureCommit(ctx, workspaceRoot, "ploy-bot", "ploy-bot@ploy.local", fmt.Sprintf("Ploy: apply changes for run %s", req.RunID)); cerr != nil {
		slog.Error("git commit failed", "run_id", req.RunID, "error", cerr)
	} else if !committed {
		slog.Info("no changes detected; proceeding to push branch without commit", "run_id", req.RunID)
	}

	// Determine the remote URL used for the push. For SSH/file remotes we
	// synthesize an HTTPS remote using the normalized domain and project path
	// because PAT-based auth only works over HTTPS.
	remoteURL, err := buildPushRemoteURL(req.RepoURL.String(), gitlabDomain, projectID)
	if err != nil {
		return "", fmt.Errorf("resolve push remote: %w", err)
	}

	// Push branch to origin using git push (Phase E).
	pusher := newPusher()
	pushOpts := git.PushOptions{
		RepoDir:   workspaceRoot,
		TargetRef: sourceBranch,
		PAT:       gitlabPAT,
		UserName:  "ploy-bot",
		UserEmail: "ploy-bot@ploy.local",
		RemoteURL: remoteURL,
	}

	slog.Info("pushing branch to origin", "run_id", req.RunID, "source_branch", sourceBranch, "submitted_target", req.TargetRef)
	if err := pusher.Push(ctx, pushOpts); err != nil {
		return "", fmt.Errorf("git push: %w", err)
	}

	// Create MR via GitLab API.
	mrClient := newMRClient()
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
// This delegates to the gitlab package's URL parsing logic to maintain consistency.
func extractProjectIDFromRepoURL(repoURL string) (string, error) {
	return gitlab.ExtractProjectIDFromURL(repoURL)
}

// buildPushRemoteURL returns the HTTPS remote used for pushing branches with PAT auth.
func buildPushRemoteURL(repoURL, gitlabDomain, projectID string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("parse repo url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
		return repoURL, nil
	case "ssh":
		// Use provided domain (already normalized to host) and unescaped project path.
		projPath, uerr := url.PathUnescape(projectID)
		if uerr != nil {
			return "", fmt.Errorf("unescape project id: %w", uerr)
		}
		host := strings.TrimSpace(gitlabDomain)
		if host == "" {
			host = "gitlab.com"
		}
		return "https://" + host + "/" + strings.TrimSuffix(projPath, ".git") + ".git", nil
	default:
		return "", fmt.Errorf("unsupported repo scheme for MR push: %s", scheme)
	}
}

// executeMRJob runs a post-run MR creation job.
// It rehydrates the final workspace state and invokes createMR.
func (r *runController) executeMRJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	typedOpts := req.TypedOptions
	manifest, err := buildManifestFromRequest(req, typedOpts, 0, contracts.ModStackUnknown)
	if err != nil {
		slog.Error("failed to build manifest for MR job", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest)
	if err != nil {
		slog.Error("failed to rehydrate workspace for MR job", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() {
		if err := os.RemoveAll(workspace); err != nil {
			slog.Warn("failed to remove workspace", "path", workspace, "error", err)
		}
	}()

	slog.Info("starting MR job execution", "run_id", req.RunID, "job_id", req.JobID, "step_index", req.StepIndex)

	mrURL, mrErr := r.createMR(ctx, req, manifest, workspace)
	duration := time.Since(startTime)

	builder := types.NewRunStatsBuilder().DurationMs(duration.Milliseconds())
	if mrURL != "" {
		builder.MetadataEntry("mr_url", mrURL)
	}

	if mrErr != nil {
		builder.Error(mrErr.Error())
		stats := builder.MustBuild()

		if errors.Is(mrErr, context.Canceled) || errors.Is(mrErr, context.DeadlineExceeded) {
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusCancelled.String(), nil, stats, req.StepIndex, req.JobID); uploadErr != nil {
				slog.Error("failed to upload MR job cancelled status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
			}
			slog.Info("MR job cancelled", "run_id", req.RunID, "job_id", req.JobID, "error", mrErr, "duration", duration)
			return
		}

		var exitCode int32 = -1
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusFail.String(), &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload MR job failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Warn("MR job failed", "run_id", req.RunID, "job_id", req.JobID, "error", mrErr, "duration", duration)
		return
	}

	stats := builder.MustBuild()
	var exitCodeZero int32 = 0
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusSuccess.String(), &exitCodeZero, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload MR job success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("MR job succeeded", "run_id", req.RunID, "job_id", req.JobID, "mr_url", mrURL, "duration", duration)
}
