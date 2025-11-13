package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

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
