//go:build legacy
// +build legacy

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// gitWorkspacePreparer clones repositories into the workspace when repo metadata is supplied.
type gitWorkspacePreparer struct{}

// Prepare clones the specified repository (if any) into the workspace path.
func (p gitWorkspacePreparer) Prepare(ctx context.Context, req runner.WorkspacePrepareRequest) error {
	repo := req.Ticket.Repo
	if strings.TrimSpace(repo.URL) == "" {
		return nil
	}
	dest := filepath.Join(req.Path, "workspace")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create workspace directory: %w", err)
	}

	args := []string{"clone", "--depth", "1"}
	if strings.TrimSpace(repo.BaseRef) != "" {
		args = append(args, "--branch", repo.BaseRef)
	}
	args = append(args, repo.URL, dest)
	if err := runGitCommand(ctx, args...); err != nil {
		return err
	}

	if target := strings.TrimSpace(repo.TargetRef); target != "" {
		if err := runGitCommand(ctx, "-C", dest, "checkout", "-b", target); err != nil {
			return err
		}
	}

	if hint := strings.TrimSpace(repo.WorkspaceHint); hint != "" {
		hinted := filepath.Join(req.Path, hint)
		_ = os.MkdirAll(hinted, 0o755)
	}
	return nil
}

func runGitCommand(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func defaultWorkspacePreparerFactory() (runner.WorkspacePreparer, error) {
	return gitWorkspacePreparer{}, nil
}

// repoAugmentedEventsClient decorates an EventsClient to inject repo metadata into claimed tickets.
type repoAugmentedEventsClient struct {
	runner.EventsClient
	repo contracts.RepoMaterialization
}

func newRepoAugmentedEventsClient(base runner.EventsClient, repo contracts.RepoMaterialization) runner.EventsClient {
	return repoAugmentedEventsClient{EventsClient: base, repo: repo}
}

func (c repoAugmentedEventsClient) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	ticket, err := c.EventsClient.ClaimTicket(ctx, ticketID)
	if err != nil {
		return contracts.WorkflowTicket{}, err
	}
	if strings.TrimSpace(ticket.Repo.URL) == "" && strings.TrimSpace(c.repo.URL) != "" {
		ticket.Repo = c.repo
	}
	return ticket, nil
}
