package git

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
)

// CreateBranchAndCheckout creates a new branch and switches to it (or just checks out if exists).
func (g *Service) CreateBranchAndCheckout(ctx context.Context, repoPath, branchName string) error {
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		co := exec.CommandContext(ctx, "git", "checkout", branchName)
		co.Dir = repoPath
		if err2 := co.Run(); err2 != nil {
			return fmt.Errorf("failed to checkout branch %s: %w", branchName, err2)
		}
	}
	return nil
}

// PushRequest encapsulates the inputs for a push operation.
type PushRequest struct {
	RepoPath  string
	RemoteURL string
	Branch    string
}

// PushBranchAsync executes a git push asynchronously and emits lifecycle events.
func (g *Service) PushBranchAsync(ctx context.Context, req PushRequest) *Operation {
	op := newOperation("push")
	go func() {
		g.emit(op, Event{Type: EventStarted, Operation: op.name, Message: fmt.Sprintf("pushing %s to %s", req.Branch, req.RemoteURL)})

		remoteURL := g.authenticatedRemoteURL(req.RemoteURL)
		_ = g.runner.Run(ctx, req.RepoPath, "git", "remote", "remove", "origin")
		if err := g.runner.Run(ctx, req.RepoPath, "git", "remote", "add", "origin", remoteURL); err != nil {
			wrapped := fmt.Errorf("failed to set remote origin: %w", err)
			g.finalize(op, Event{Type: EventFailed, Operation: op.name, Message: wrapped.Error(), Err: wrapped})
			return
		}

		g.emit(op, Event{Type: EventProgress, Operation: op.name, Message: "remote origin configured"})

		if err := g.runner.Run(ctx, req.RepoPath, "git", "push", "-u", "origin", req.Branch); err != nil {
			wrapped := fmt.Errorf("git push failed: %w", err)
			g.finalize(op, Event{Type: EventFailed, Operation: op.name, Message: wrapped.Error(), Err: wrapped})
			return
		}

		g.finalize(op, Event{Type: EventCompleted, Operation: op.name, Message: "branch pushed"})
	}()
	return op
}

// PushBranch is the synchronous wrapper maintained for compatibility with existing call sites.
func (g *Service) PushBranch(ctx context.Context, repoPath, remoteURL, branchName string) error {
	op := g.PushBranchAsync(ctx, PushRequest{RepoPath: repoPath, RemoteURL: remoteURL, Branch: branchName})
	return op.Wait()
}

// authenticatedRemoteURL injects credentials into the remote URL when possible.
func (g *Service) authenticatedRemoteURL(remote string) string {
	token := os.Getenv("PLOY_GITLAB_PAT")
	if token == "" {
		return remote
	}
	u, err := url.Parse(remote)
	if err != nil {
		return remote
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return remote
	}
	if u.User != nil {
		return remote
	}
	u.User = url.UserPassword("oauth2", token)
	return u.String()
}
