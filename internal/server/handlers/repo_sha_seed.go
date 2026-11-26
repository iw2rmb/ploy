package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/gitexec"
)

type resolvedSourceCommit struct {
	BaseRef string
	SHA     string
}

var shortCommitPrefix = regexp.MustCompile(`^[0-9a-fA-F]{4,39}$`)

type sourceCommitSHAResolverFunc func(context.Context, string, string) (resolvedSourceCommit, error)

type sourceCommitSHAResolverContextKey struct{}

var sourceCommitSHAResolver sourceCommitSHAResolverFunc
var sourceCommitResolveTimeout = 20 * time.Second

func withSourceCommitSHAResolver(ctx context.Context, resolver sourceCommitSHAResolverFunc) context.Context {
	return context.WithValue(ctx, sourceCommitSHAResolverContextKey{}, resolver)
}

func resolveSourceCommitSHAFromContext(ctx context.Context, repoURL, ref string, auth gitauth.Options) (resolvedSourceCommit, error) {
	resolveCtx, cancel := withSourceCommitResolveTimeout(ctx)
	defer cancel()
	if resolver, ok := ctx.Value(sourceCommitSHAResolverContextKey{}).(sourceCommitSHAResolverFunc); ok && resolver != nil {
		return resolver(resolveCtx, repoURL, ref)
	}
	if sourceCommitSHAResolver != nil {
		return sourceCommitSHAResolver(resolveCtx, repoURL, ref)
	}
	return resolveSourceCommitSHA(resolveCtx, repoURL, ref, auth)
}

func withSourceCommitResolveTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if sourceCommitResolveTimeout <= 0 {
		return ctx, func() {}
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= sourceCommitResolveTimeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, sourceCommitResolveTimeout)
}

func resolveSourceCommitSHA(ctx context.Context, repoURL, ref string, auth gitauth.Options) (resolvedSourceCommit, error) {
	repoURL = strings.TrimSpace(repoURL)
	ref = strings.TrimSpace(ref)
	if repoURL == "" {
		return resolvedSourceCommit{}, fmt.Errorf("repo_url is empty")
	}
	if ref == "" {
		return resolvedSourceCommit{}, fmt.Errorf("base_ref is empty")
	}

	if sha, ok := domaintypes.NormalizeFullCommitSHA(ref); ok {
		return resolvedSourceCommit{BaseRef: sha.String(), SHA: sha.String()}, nil
	}
	candidates := []string{"refs/heads/" + ref, "refs/tags/" + ref + "^{}", "refs/tags/" + ref}
	if strings.HasPrefix(ref, "refs/tags/") {
		candidates = append(candidates, ref+"^{}")
	}
	candidates = append(candidates, ref)
	sha, err := gitLSRemote(ctx, repoURL, auth, candidates...)
	if err != nil {
		return resolvedSourceCommit{}, err
	}
	if sha != "" {
		return resolvedSourceCommit{BaseRef: ref, SHA: sha}, nil
	}
	if shortCommitPrefix.MatchString(ref) {
		sha, err = resolveGitLabShortSHA(ctx, repoURL, ref, auth)
		if err != nil {
			return resolvedSourceCommit{}, err
		}
		return resolvedSourceCommit{BaseRef: sha, SHA: sha}, nil
	}
	return resolvedSourceCommit{}, fmt.Errorf("ref %q not found on remote", ref)
}

func gitLSRemote(ctx context.Context, repoURL string, auth gitauth.Options, refs ...string) (string, error) {
	prepared := gitauth.PrepareURL(repoURL, auth)
	result, err := gitexec.Execute(ctx, gitexec.Request{Env: prepared.Env, Args: append([]string{"ls-remote", "--", prepared.URL}, refs...)})
	out := append(result.Stdout, result.Stderr...)
	if err != nil {
		failureErr := err
		if ctxErr := ctx.Err(); ctxErr != nil {
			failureErr = ctxErr
		}
		return "", fmt.Errorf("git ls-remote failed (%s)", classifyGitLSRemoteFailure(failureErr, out))
	}
	// ls-remote patterns also match ref suffixes. Compare complete ref names
	// and honor caller precedence even when the remote lists tags first.
	matches := make(map[string]string)
	for _, line := range bytes.Split(result.Stdout, []byte("\n")) {
		fields := bytes.Fields(line)
		if len(fields) == 2 {
			if sha, ok := domaintypes.NormalizeFullCommitSHA(string(fields[0])); ok {
				matches[string(fields[1])] = sha.String()
			}
		}
	}
	for _, ref := range refs {
		if sha := matches[ref]; sha != "" {
			return sha, nil
		}
	}
	return "", nil
}

func classifyGitLSRemoteFailure(err error, out []byte) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, exec.ErrNotFound), strings.Contains(err.Error(), "executable file not found"):
		return "git executable not found in server runtime"
	}
	msg := strings.ToLower(strings.TrimSpace(string(out)))
	switch {
	case strings.Contains(msg, "authentication failed"),
		strings.Contains(msg, "http basic: access denied"),
		strings.Contains(msg, "access denied"),
		strings.Contains(msg, "invalid username or password"),
		strings.Contains(msg, "the requested url returned error: 401"),
		strings.Contains(msg, "the requested url returned error: 403"),
		strings.Contains(msg, "could not read username"),
		strings.Contains(msg, "unable to update url base from redirection"),
		strings.Contains(msg, "/users/sign_in"):
		return "authentication failed or token rejected"
	case strings.Contains(msg, "couldn't find remote ref"),
		strings.Contains(msg, "remote ref does not exist"):
		return "ref not found on remote"
	case strings.Contains(msg, "repository not found"),
		strings.Contains(msg, "project not found"),
		strings.Contains(msg, "the requested url returned error: 404"):
		return "repository not found or access denied"
	case strings.Contains(msg, "could not resolve host"):
		return "dns lookup failed"
	case strings.Contains(msg, "failed to connect"),
		strings.Contains(msg, "connection timed out"),
		strings.Contains(msg, "operation timed out"),
		strings.Contains(msg, "network is unreachable"),
		strings.Contains(msg, "no route to host"):
		return "network connection failed"
	case strings.Contains(msg, "ssl certificate problem"),
		strings.Contains(msg, "certificate verify failed"),
		strings.Contains(msg, "tls handshake"),
		strings.Contains(msg, "gnutls_handshake"):
		return "tls or certificate validation failed"
	default:
		return "remote query failed"
	}
}
