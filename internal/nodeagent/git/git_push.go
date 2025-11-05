package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PushOptions holds configuration for pushing a git branch.
type PushOptions struct {
	// RepoDir is the local git repository directory.
	RepoDir string
	// TargetRef is the branch name to push (e.g., "workflow/abc/123").
	TargetRef string
	// PAT is the Personal Access Token for authentication.
	// Will be provided via GIT_ASKPASS to avoid embedding in remote URL.
	PAT string
	// UserName is the git user.name config value.
	UserName string
	// UserEmail is the git user.email config value.
	UserEmail string
}

// Pusher provides git push functionality.
type Pusher interface {
	// Push pushes the specified branch to origin using PAT authentication.
	Push(ctx context.Context, opts PushOptions) error
}

type pusher struct{}

// NewPusher creates a new git Pusher.
func NewPusher() Pusher {
	return &pusher{}
}

// Push pushes the target branch to origin using PAT authentication via GIT_ASKPASS.
// It configures git user.name and user.email, then performs the push operation.
// The PAT is never persisted to disk or embedded in the remote URL.
func (p *pusher) Push(ctx context.Context, opts PushOptions) error {
	if err := validatePushOptions(opts); err != nil {
		return fmt.Errorf("invalid push options: %w", err)
	}

	// Configure git user identity.
	if err := p.configureGitUser(ctx, opts.RepoDir, opts.UserName, opts.UserEmail); err != nil {
		return fmt.Errorf("configure git user: %w", err)
	}

	// Create a temporary GIT_ASKPASS script that echoes the PAT.
	// This script will be deleted after use.
	askpassScript, cleanup, err := createAskpassScript(opts.PAT)
	if err != nil {
		return fmt.Errorf("create askpass script: %w", err)
	}
	defer cleanup()

	// Push the branch using GIT_ASKPASS for authentication.
	if err := p.pushBranch(ctx, opts.RepoDir, opts.TargetRef, askpassScript); err != nil {
		return redactError(err, opts.PAT)
	}

	return nil
}

// validatePushOptions checks that required options are provided.
func validatePushOptions(opts PushOptions) error {
	if strings.TrimSpace(opts.RepoDir) == "" {
		return fmt.Errorf("repo_dir is required")
	}
	if strings.TrimSpace(opts.TargetRef) == "" {
		return fmt.Errorf("target_ref is required")
	}
	if strings.TrimSpace(opts.PAT) == "" {
		return fmt.Errorf("pat is required")
	}
	if strings.TrimSpace(opts.UserName) == "" {
		return fmt.Errorf("user_name is required")
	}
	if strings.TrimSpace(opts.UserEmail) == "" {
		return fmt.Errorf("user_email is required")
	}
	return nil
}

// configureGitUser sets git user.name and user.email in the repository.
func (p *pusher) configureGitUser(ctx context.Context, repoDir, userName, userEmail string) error {
	if err := runGitCommand(ctx, repoDir, nil, "config", "user.name", userName); err != nil {
		return fmt.Errorf("set user.name: %w", err)
	}
	if err := runGitCommand(ctx, repoDir, nil, "config", "user.email", userEmail); err != nil {
		return fmt.Errorf("set user.email: %w", err)
	}
	return nil
}

// pushBranch performs the git push operation using the provided askpass script.
func (p *pusher) pushBranch(ctx context.Context, repoDir, targetRef, askpassScript string) error {
	env := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=" + askpassScript,
	}
	if err := runGitCommand(ctx, repoDir, env, "push", "origin", targetRef); err != nil {
		return fmt.Errorf("git push origin %s: %w", targetRef, err)
	}
	return nil
}

// createAskpassScript creates a temporary shell script that echoes the PAT.
// Returns the script path and a cleanup function.
func createAskpassScript(pat string) (string, func(), error) {
	tmpDir := os.TempDir()
	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("git-askpass-%d.sh", os.Getpid()))

	// Script content: echo the PAT when invoked.
	scriptContent := fmt.Sprintf("#!/bin/sh\necho '%s'\n", strings.ReplaceAll(pat, "'", "'\\''"))

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0700); err != nil {
		return "", nil, fmt.Errorf("write askpass script: %w", err)
	}

	cleanup := func() {
		_ = os.Remove(scriptPath)
	}

	return scriptPath, cleanup, nil
}

// runGitCommand executes a git command in the specified directory with custom environment.
func runGitCommand(ctx context.Context, dir string, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	// Start with base environment and add custom env vars.
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, string(output))
	}

	return nil
}

// redactError replaces any occurrence of the PAT in error messages with [REDACTED].
func redactError(err error, pat string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if pat != "" && strings.Contains(msg, pat) {
		msg = strings.ReplaceAll(msg, pat, "[REDACTED]")
		return fmt.Errorf("%s", msg)
	}
	return err
}
