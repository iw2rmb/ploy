package speccatalog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/gitauth"
)

type gitRunner interface {
	Run(context.Context, string, []string, ...string) ([]byte, error)
}

type execGitRunner struct{}

func (execGitRunner) Run(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	cmd.Env = append(cmd.Env, env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func (r *repository) refreshNow(ctx context.Context) ([]Entry, error) {
	prepared := gitauth.PrepareURL(r.cloneURL, r.auth)
	if _, err := os.Stat(filepath.Join(r.checkout, ".git")); err == nil {
		if _, err := r.runner.Run(ctx, r.checkout, prepared.Env, "fetch", "--depth", "1", "--no-tags", "origin", "HEAD"); err != nil {
			return nil, err
		}
		if _, err := r.runner.Run(ctx, r.checkout, nil, "checkout", "--detach", "--force", "FETCH_HEAD"); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect cached checkout: %w", err)
	} else if err := r.clone(ctx, prepared); err != nil {
		return nil, err
	}

	if _, err := r.runner.Run(ctx, r.checkout, nil, "remote", "set-url", "origin", prepared.URL); err != nil {
		return nil, fmt.Errorf("sanitize cached origin: %w", err)
	}
	shaRaw, err := r.runner.Run(ctx, r.checkout, nil, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, err
	}
	sha := strings.TrimSpace(string(shaRaw))
	if !fullCommitSHA(sha) {
		return nil, fmt.Errorf("git rev-parse returned invalid commit SHA %q", sha)
	}
	return r.scan(ctx, sha)
}

func (r *repository) clone(ctx context.Context, prepared gitauth.PreparedURL) error {
	parent := filepath.Dir(r.checkout)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create spec repository cache: %w", err)
	}
	if err := os.RemoveAll(r.checkout); err != nil {
		return fmt.Errorf("remove incomplete cached checkout: %w", err)
	}
	tempRoot, err := os.MkdirTemp(parent, ".clone-")
	if err != nil {
		return fmt.Errorf("create temporary clone directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()

	tempCheckout := filepath.Join(tempRoot, "checkout")
	if _, err := r.runner.Run(ctx, "", prepared.Env, "clone", "--depth", "1", "--single-branch", "--no-tags", prepared.URL, tempCheckout); err != nil {
		return err
	}
	if _, err := r.runner.Run(ctx, tempCheckout, nil, "remote", "set-url", "origin", prepared.URL); err != nil {
		return fmt.Errorf("sanitize cloned origin: %w", err)
	}
	if err := os.Rename(tempCheckout, r.checkout); err != nil {
		return fmt.Errorf("install cached checkout: %w", err)
	}
	return nil
}

func fullCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
