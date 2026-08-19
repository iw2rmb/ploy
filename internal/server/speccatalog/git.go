package speccatalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/gitexec"
)

func (r *repository) runGit(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	result, err := r.runner.Run(ctx, gitexec.Request{Dir: dir, Env: env, Args: args})
	if err != nil {
		output := append(result.Stdout, result.Stderr...)
		return nil, fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return result.Stdout, nil
}

func (r *repository) refreshNow(ctx context.Context) ([]Entry, error) {
	r.checkoutMu.Lock()
	defer r.checkoutMu.Unlock()

	prepared := gitauth.PrepareURL(r.cloneURL, r.auth)
	if _, err := os.Stat(filepath.Join(r.checkout, ".git")); err == nil {
		if _, err := r.runGit(ctx, r.checkout, prepared.Env, "fetch", "--depth", "1", "--no-tags", "origin", "HEAD"); err != nil {
			return nil, err
		}
		if _, err := r.runGit(ctx, r.checkout, nil, "checkout", "--detach", "--force", "FETCH_HEAD"); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect cached checkout: %w", err)
	} else if err := r.clone(ctx, prepared); err != nil {
		return nil, err
	}

	if _, err := r.runGit(ctx, r.checkout, nil, "remote", "set-url", "origin", prepared.URL); err != nil {
		return nil, fmt.Errorf("sanitize cached origin: %w", err)
	}
	shaRaw, err := r.runGit(ctx, r.checkout, nil, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return nil, err
	}
	sha := strings.TrimSpace(string(shaRaw))
	if !domaintypes.IsCanonicalFullCommitSHA(sha) {
		return nil, fmt.Errorf("git rev-parse returned invalid commit SHA %q", sha)
	}
	committedAtRaw, err := r.runGit(ctx, r.checkout, nil, "show", "-s", "--format=%cI", "HEAD")
	if err != nil {
		return nil, err
	}
	committedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(string(committedAtRaw)))
	if err != nil {
		return nil, fmt.Errorf("parse spec repository commit time: %w", err)
	}
	return r.scan(ctx, sha, committedAt)
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
	if _, err := r.runGit(ctx, "", prepared.Env, "clone", "--depth", "1", "--single-branch", "--no-tags", prepared.URL, tempCheckout); err != nil {
		return err
	}
	if _, err := r.runGit(ctx, tempCheckout, nil, "remote", "set-url", "origin", prepared.URL); err != nil {
		return fmt.Errorf("sanitize cloned origin: %w", err)
	}
	if err := os.Rename(tempCheckout, r.checkout); err != nil {
		return fmt.Errorf("install cached checkout: %w", err)
	}
	return nil
}
