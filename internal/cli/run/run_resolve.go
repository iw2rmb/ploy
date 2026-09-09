package run

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/gitexec"
)

type resolvedSourceRepo struct {
	Worktree  string
	RepoURL   string
	Ref       string
	CommitSHA string
	IsLocal   bool
}

func resolveSourceRepo(ctx context.Context, base *url.URL, httpClient *http.Client, selector string) (resolvedSourceRepo, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		selector = "."
	}

	if pathLooksLocal(selector) {
		return resolveLocalSourceRepo(ctx, selector)
	}
	return resolveRemoteSourceRepo(ctx, base, httpClient, selector)
}

func pathLooksLocal(selector string) bool {
	if selector == "." || strings.HasPrefix(selector, "./") || strings.HasPrefix(selector, "../") || filepath.IsAbs(selector) {
		return true
	}
	if _, err := os.Stat(selector); err == nil {
		return true
	}
	return false
}

func resolveLocalSourceRepo(ctx context.Context, path string) (resolvedSourceRepo, error) {
	worktree, err := resolveLocalWorktree(ctx, path)
	if err != nil {
		return resolvedSourceRepo{}, err
	}
	repoURL, err := gitOutput(ctx, worktree, "remote", "get-url", "origin")
	if err != nil {
		return resolvedSourceRepo{}, fmt.Errorf("repo: git remote %q not found", "origin")
	}
	head, err := gitOutput(ctx, worktree, "rev-parse", "HEAD")
	if err != nil {
		return resolvedSourceRepo{}, fmt.Errorf("repo: resolve HEAD: %w", err)
	}
	return resolvedSourceRepo{
		Worktree:  worktree,
		RepoURL:   repoURL,
		Ref:       head,
		CommitSHA: head,
		IsLocal:   true,
	}, nil
}

func resolveLocalWorktree(ctx context.Context, path string) (string, error) {
	worktree, err := gitOutput(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("repo: %s is not a git worktree", path)
	}
	return worktree, nil
}

func resolveRemoteSourceRepo(ctx context.Context, base *url.URL, httpClient *http.Client, selector string) (resolvedSourceRepo, error) {
	resolved, err := common.ResolveRemoteRepoSelector(ctx, base, httpClient, selector)
	if err != nil {
		return resolvedSourceRepo{}, err
	}
	return resolvedSourceRepo{RepoURL: resolved.RepoURL, Ref: resolved.Ref, CommitSHA: resolved.CommitSHA}, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	result, err := gitexec.Execute(ctx, gitexec.Request{Dir: dir, Args: args})
	if err != nil {
		msg := strings.TrimSpace(string(result.Stderr))
		if msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}
