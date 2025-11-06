package git

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
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
		return redactError(fmt.Errorf("invalid push options: %w", err), opts.PAT)
	}

	// Configure git user identity.
	if err := p.configureGitUser(ctx, opts.RepoDir, opts.UserName, opts.UserEmail); err != nil {
		return redactError(fmt.Errorf("configure git user: %w", err), opts.PAT)
	}

	// Push the branch using HTTP extra header for authentication to avoid prompts and
	// prevent writing secrets to disk. We pass the header via environment only.
	if err := p.pushBranch(ctx, opts.RepoDir, opts.TargetRef, opts.PAT); err != nil {
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
func (p *pusher) pushBranch(ctx context.Context, repoDir, targetRef, pat string) error {
	env := []string{
		"GIT_TERMINAL_PROMPT=0",
		// Supply Authorization header only for this command; nothing persisted.
		"GIT_HTTP_EXTRAHEADER=Authorization: Bearer " + pat,
	}
	if err := runGitCommand(ctx, repoDir, env, "push", "origin", targetRef); err != nil {
		return fmt.Errorf("git push origin %s: %w", targetRef, err)
	}
	return nil
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
		// Redact any PAT from env variables before including output in error.
		// Look for GIT_HTTP_EXTRAHEADER Authorization bearer token.
		pat := ""
		for _, e := range env {
			if strings.HasPrefix(e, "GIT_HTTP_EXTRAHEADER=") {
				// e.g., GIT_HTTP_EXTRAHEADER=Authorization: Bearer <token>
				if i := strings.Index(e, "Bearer "); i >= 0 {
					pat = e[i+len("Bearer "):]
					break
				}
			}
		}
		baseErr := fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, string(output))
		return redactError(baseErr, pat)
	}

	return nil
}

// redactError replaces any occurrence of the PAT in error messages with [REDACTED].
// It handles both literal PAT and URL-encoded variants.
func redactError(err error, pat string) error {
	if err == nil {
		return nil
	}
	if pat == "" {
		return err
	}

	msg := err.Error()

	// Build a set of variants to redact: literal, query-escaped, path-escaped,
	// and a minimal legacy replacement used in early code paths.
	variants := map[string]struct{}{
		pat: {},
	}
	if q := url.QueryEscape(pat); q != pat {
		variants[q] = struct{}{}
		// Some logs render spaces as %20 not "+"; include that form.
		variants[strings.ReplaceAll(q, "+", "%20")] = struct{}{}
	}
	if p := url.PathEscape(pat); p != pat {
		variants[p] = struct{}{}
	}
	// Legacy minimal encoding coverage.
	variants[strings.ReplaceAll(strings.ReplaceAll(pat, " ", "%20"), "@", "%40")] = struct{}{}

	modified := false
	for v := range variants {
		if v == "" || v == msg {
			continue
		}
		if strings.Contains(msg, v) {
			msg = strings.ReplaceAll(msg, v, "[REDACTED]")
			modified = true
		}
	}

	if modified {
		return fmt.Errorf("%s", msg)
	}
	return err
}
