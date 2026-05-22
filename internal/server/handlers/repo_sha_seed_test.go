package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/gitauth"
)

func TestClassifyGitLSRemoteFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		out  string
		want string
	}{
		{
			name: "auth denied",
			err:  errors.New("exit status 128"),
			out:  "fatal: Authentication failed for 'https://gitlab.example.com/group/repo.git/'",
			want: "authentication failed or token rejected",
		},
		{
			name: "gitlab sign in redirect",
			err:  errors.New("exit status 128"),
			out:  "fatal: unable to update url base from redirection:\n  redirect: https://gitlab.example.com/users/sign_in",
			want: "authentication failed or token rejected",
		},
		{
			name: "missing ref",
			err:  errors.New("exit status 128"),
			out:  "fatal: couldn't find remote ref refs/heads/missing",
			want: "ref not found on remote",
		},
		{
			name: "missing project",
			err:  errors.New("exit status 128"),
			out:  "remote: The project you were looking for could not be found.\nfatal: repository not found",
			want: "repository not found or access denied",
		},
		{
			name: "http 404",
			err:  errors.New("exit status 128"),
			out:  "fatal: unable to access 'https://gitlab.example.com/group/repo.git/': The requested URL returned error: 404",
			want: "repository not found or access denied",
		},
		{
			name: "dns failure",
			err:  errors.New("exit status 128"),
			out:  "fatal: unable to access 'https://gitlab.example.com/group/repo.git/': Could not resolve host: gitlab.example.com",
			want: "dns lookup failed",
		},
		{
			name: "network failure",
			err:  errors.New("exit status 128"),
			out:  "fatal: unable to access 'https://gitlab.example.com/group/repo.git/': Failed to connect to gitlab.example.com port 443",
			want: "network connection failed",
		},
		{
			name: "tls failure",
			err:  errors.New("exit status 128"),
			out:  "fatal: unable to access 'https://gitlab.example.com/group/repo.git/': SSL certificate problem: unable to get local issuer certificate",
			want: "tls or certificate validation failed",
		},
		{
			name: "timeout",
			err:  context.DeadlineExceeded,
			want: "timed out",
		},
		{
			name: "canceled",
			err:  context.Canceled,
			want: "canceled",
		},
		{
			name: "missing git",
			err:  errors.New(`exec: "git": executable file not found in $PATH`),
			want: "git executable not found in server runtime",
		},
		{
			name: "fallback",
			err:  errors.New("exit status 128"),
			out:  "fatal: unexpected remote failure",
			want: "remote query failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyGitLSRemoteFailure(tt.err, []byte(tt.out))
			if got != tt.want {
				t.Fatalf("classifyGitLSRemoteFailure() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveSourceCommitSHAFromContextAppliesTimeout(t *testing.T) {
	oldTimeout := sourceCommitResolveTimeout
	sourceCommitResolveTimeout = 10 * time.Millisecond
	t.Cleanup(func() { sourceCommitResolveTimeout = oldTimeout })

	ctx := withSourceCommitSHAResolver(context.Background(), func(ctx context.Context, _, _ string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	start := time.Now()
	_, err := resolveSourceCommitSHAFromContext(ctx, "https://gitlab.example.com/group/repo.git", "main", gitauth.Options{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("resolveSourceCommitSHAFromContext() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("resolveSourceCommitSHAFromContext() elapsed = %s, want bounded timeout", elapsed)
	}
}
