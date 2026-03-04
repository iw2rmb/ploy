package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var sha40Pattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type sourceCommitSHAResolverFunc func(context.Context, string, string) (string, error)

type sourceCommitSHAResolverContextKey struct{}

var sourceCommitSHAResolver sourceCommitSHAResolverFunc = resolveSourceCommitSHA

func withSourceCommitSHAResolver(ctx context.Context, resolver sourceCommitSHAResolverFunc) context.Context {
	return context.WithValue(ctx, sourceCommitSHAResolverContextKey{}, resolver)
}

func resolveSourceCommitSHAFromContext(ctx context.Context, repoURL, ref string) (string, error) {
	if resolver, ok := ctx.Value(sourceCommitSHAResolverContextKey{}).(sourceCommitSHAResolverFunc); ok && resolver != nil {
		return resolver(ctx, repoURL, ref)
	}
	return sourceCommitSHAResolver(ctx, repoURL, ref)
}

func resolveSourceCommitSHA(ctx context.Context, repoURL, ref string) (string, error) {
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

	for _, candidate := range candidates {
		sha, err := gitLSRemote(ctx, repoURL, candidate)
		if err == nil {
			return sha, nil
		}
	}
	return "", fmt.Errorf("resolve source commit sha for ref %q", ref)
}

func gitLSRemote(ctx context.Context, repoURL, ref string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", repoURL, ref)
	out, err := cmd.Output()
	if err != nil {
		return "", err
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
