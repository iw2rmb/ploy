// execution_mr.go separates GitLab merge request workflow from core execution.
//
// This file contains MR creation logic for runs that complete with terminal
// status (succeeded or failed). It handles Git commit creation, branch push,
// and GitLab MR API interaction. MR creation is triggered by manifest options
// (mr_on_success/mr_on_fail) and is isolated from execution orchestration to
// maintain single responsibility for each file.
package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/nodeagent/gitlab"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// GitLab merge request operations.
// This file isolates MR creation workflow from core execution orchestration.

// shouldCreateMR determines if an MR should be created based on terminal status and manifest options.
// MR creation is triggered by mr_on_success (for succeeded runs) or mr_on_fail (for failed runs).
// Only boolean true values trigger MR creation; non-boolean values are ignored.
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
// 3. Creates a unique source branch name (ploy-<run-id>)
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

	// Use a unique source branch per run: ploy-<ticket-id>.
	// This avoids MR conflicts on repeated runs regardless of the submitted target ref.
	sourceBranch := req.TargetRef.String()

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
//
// Rules:
//   - If repoURL is already https, return as-is.
//   - If repoURL is ssh, synthesize https://{domain}/{projectPath}.git where
//     projectPath is the unescaped projectID (e.g., org%2Fproj -> org/proj).
//   - Any other scheme (e.g., file) is unsupported for MR push and returns error.
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
