package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/iw2rmb/ploy/internal/gitauth"
)

var sha40Pattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type sourceCommitSHAResolverFunc func(context.Context, string, string) (string, error)

type sourceCommitSHAResolverContextKey struct{}

var sourceCommitSHAResolver sourceCommitSHAResolverFunc

func withSourceCommitSHAResolver(ctx context.Context, resolver sourceCommitSHAResolverFunc) context.Context {
	return context.WithValue(ctx, sourceCommitSHAResolverContextKey{}, resolver)
}

func resolveSourceCommitSHAFromContext(ctx context.Context, repoURL, ref string, auth gitauth.Options) (string, error) {
	if resolver, ok := ctx.Value(sourceCommitSHAResolverContextKey{}).(sourceCommitSHAResolverFunc); ok && resolver != nil {
		return resolver(ctx, repoURL, ref)
	}
	if sourceCommitSHAResolver != nil {
		return sourceCommitSHAResolver(ctx, repoURL, ref)
	}
	return resolveSourceCommitSHA(ctx, repoURL, ref, auth)
}

func resolveSourceCommitSHA(ctx context.Context, repoURL, ref string, auth gitauth.Options) (string, error) {
	repoURL = strings.TrimSpace(repoURL)
	ref = strings.TrimSpace(ref)
	if repoURL == "" {
		return "", fmt.Errorf("repo_url is empty")
	}
	if ref == "" {
		return "", fmt.Errorf("base_ref is empty")
	}

	candidates := []string{ref}
	if !strings.HasPrefix(ref, "refs/") {
		candidates = append(candidates, "refs/heads/"+ref, "refs/tags/"+ref)
	}

	attemptErrs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		sha, err := gitLSRemote(ctx, repoURL, candidate, auth)
		if err == nil {
			return sha, nil
		}
		attemptErrs = append(attemptErrs, fmt.Sprintf("%s: %v", candidate, err))
	}
	return "", fmt.Errorf("resolve source commit sha for ref %q: %s", ref, strings.Join(attemptErrs, "; "))
}

func gitLSRemote(ctx context.Context, repoURL, ref string, auth gitauth.Options) (string, error) {
	prepared := gitauth.PrepareURL(repoURL, auth)
	cmd := exec.CommandContext(ctx, "git", "ls-remote", prepared.URL, ref)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	cmd.Env = append(cmd.Env, prepared.Env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git ls-remote failed (%s)", classifyGitLSRemoteFailure(out))
	}
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		parts := bytes.Fields(line)
		if len(parts) < 1 {
			continue
		}
		sha := strings.ToLower(strings.TrimSpace(string(parts[0])))
		if sha40Pattern.MatchString(sha) {
			return sha, nil
		}
	}
	return "", fmt.Errorf("no matching commit sha found")
}

func classifyGitLSRemoteFailure(out []byte) string {
	msg := strings.ToLower(strings.TrimSpace(string(out)))
	switch {
	case strings.Contains(msg, "authentication failed"),
		strings.Contains(msg, "http basic: access denied"),
		strings.Contains(msg, "access denied"),
		strings.Contains(msg, "invalid username or password"),
		strings.Contains(msg, "could not read username"),
		strings.Contains(msg, "unable to update url base from redirection"),
		strings.Contains(msg, "/users/sign_in"):
		return "authentication failed or token rejected"
	case strings.Contains(msg, "couldn't find remote ref"),
		strings.Contains(msg, "remote ref does not exist"):
		return "ref not found on remote"
	case strings.Contains(msg, "repository not found"),
		strings.Contains(msg, "project not found"):
		return "repository not found or access denied"
	default:
		return "remote query failed"
	}
}
