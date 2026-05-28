package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type resolvedRunRepo struct {
	Worktree  string
	RepoURL   string
	Ref       string
	CommitSHA string
	IsLocal   bool
}

func resolveRunRepo(ctx context.Context, base *url.URL, httpClient *http.Client, selector string) (resolvedRunRepo, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		selector = "."
	}

	if pathLooksLocal(selector) {
		return resolveLocalRunRepo(ctx, selector)
	}
	return resolveRemoteRunRepo(ctx, base, httpClient, selector)
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

func resolveLocalRunRepo(ctx context.Context, path string) (resolvedRunRepo, error) {
	worktree, err := gitOutput(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return resolvedRunRepo{}, fmt.Errorf("repo: %s is not a git worktree", path)
	}
	repoURL, err := gitOutput(ctx, worktree, "remote", "get-url", "origin")
	if err != nil {
		return resolvedRunRepo{}, fmt.Errorf("repo: git remote %q not found", "origin")
	}
	head, err := gitOutput(ctx, worktree, "rev-parse", "HEAD")
	if err != nil {
		return resolvedRunRepo{}, fmt.Errorf("repo: resolve HEAD: %w", err)
	}
	return resolvedRunRepo{
		Worktree:  worktree,
		RepoURL:   repoURL,
		Ref:       head,
		CommitSHA: head,
		IsLocal:   true,
	}, nil
}

func resolveRemoteRunRepo(ctx context.Context, base *url.URL, httpClient *http.Client, selector string) (resolvedRunRepo, error) {
	if base == nil {
		return resolvedRunRepo{}, errors.New("repo resolve: base url required")
	}
	if httpClient == nil {
		return resolvedRunRepo{}, errors.New("repo resolve: http client required")
	}

	namespaceRepo, ref := splitRemoteSelector(selector)
	endpoint := base.JoinPath("v1", "repos", "resolve")
	payload, err := json.Marshal(struct {
		Selector string `json:"selector"`
		Ref      string `json:"ref"`
	}{
		Selector: namespaceRepo,
		Ref:      ref,
	})
	if err != nil {
		return resolvedRunRepo{}, fmt.Errorf("repo resolve: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return resolvedRunRepo{}, fmt.Errorf("repo resolve: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return resolvedRunRepo{}, fmt.Errorf("repo resolve: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error) != "" {
			return resolvedRunRepo{}, fmt.Errorf("repo resolve: %s", strings.TrimSpace(apiErr.Error))
		}
		return resolvedRunRepo{}, fmt.Errorf("repo resolve: %s", strings.TrimSpace(string(body)))
	}

	var resolved struct {
		RepoURL  string `json:"repo_url"`
		Ref      string `json:"ref"`
		RefIsSHA bool   `json:"ref_is_sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resolved); err != nil {
		return resolvedRunRepo{}, fmt.Errorf("repo resolve: decode response: %w", err)
	}
	repoURL := strings.TrimSpace(resolved.RepoURL)
	if repoURL == "" {
		return resolvedRunRepo{}, errors.New("repo resolve: empty repo_url in response")
	}
	resolvedRef := strings.TrimSpace(resolved.Ref)
	if resolvedRef == "" {
		resolvedRef = ref
	}
	repo := resolvedRunRepo{RepoURL: repoURL, Ref: resolvedRef}
	if resolved.RefIsSHA {
		repo.CommitSHA = resolvedRef
	}
	return repo, nil
}

func splitRemoteSelector(selector string) (string, string) {
	selector = strings.TrimSpace(selector)
	ref := "master"
	if slash := strings.Index(selector, "/"); slash >= 0 {
		if colon := strings.Index(selector[slash+1:], ":"); colon >= 0 {
			idx := slash + 1 + colon
			ref = strings.TrimSpace(selector[idx+1:])
			selector = selector[:idx]
		}
	}
	if ref == "" {
		ref = "master"
	}
	return selector, ref
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
