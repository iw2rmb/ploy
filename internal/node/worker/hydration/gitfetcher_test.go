package hydration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	stepruntime "github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

func TestGitFetcherUsesTokenForCloneAndFetch(t *testing.T) {
	t.Helper()
	originalRun := gitRun
	originalOutput := gitOutput
	defer func() {
		gitRun = originalRun
		gitOutput = originalOutput
	}()

	var cloneCalled bool
	var fetchCalled bool
	var checkoutCalled bool
	var cloneDest string

	gitRun = func(ctx context.Context, git string, args ...string) error {
		t.Helper()
		header := ""
		if len(args) >= 2 && args[0] == "-c" {
			header = args[1]
			args = args[2:]
		}
		if len(args) == 0 {
			return errors.New("no git arguments provided")
		}
		switch args[0] {
		case "clone":
			if !strings.Contains(header, "PRIVATE-TOKEN: glpat-token") {
				t.Fatalf("expected clone header with token, got %q", header)
			}
			cloneCalled = true
			cloneDest = args[len(args)-1]
			if err := os.MkdirAll(cloneDest, 0o755); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(cloneDest, ".git"), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(cloneDest, "main.go"), []byte("package main\n"), 0o644); err != nil {
				return err
			}
			return nil
		case "-C":
			if len(args) < 4 {
				return errors.New("invalid -C usage")
			}
			if args[1] != cloneDest {
				return errors.New("fetch path mismatch")
			}
			switch args[2] {
			case "fetch":
				if !strings.Contains(header, "PRIVATE-TOKEN: glpat-token") {
					t.Fatalf("expected fetch header with token, got %q", header)
				}
				fetchCalled = true
				return nil
			case "checkout":
				if header != "" {
					t.Fatalf("checkout should not include token header")
				}
				checkoutCalled = true
				return nil
			default:
				return errors.New("unexpected git command: " + args[2])
			}
		default:
			return errors.New("unexpected git invocation: " + args[0])
		}
	}

	gitOutput = func(ctx context.Context, git string, args ...string) (string, error) {
		return "abcdef1234567890\n", nil
	}

	publisher := &stubPublisher{}
	tokens := &stubTokenSource{token: Token{Value: "glpat-token", ExpiresAt: time.Now().Add(30 * time.Minute)}}

	fetcher, err := NewGitFetcher(GitFetcherOptions{
		Publisher:   publisher,
		TokenSource: tokens,
	})
	if err != nil {
		t.Fatalf("NewGitFetcher: %v", err)
	}

	repo := contracts.RepoMaterialization{
		URL:       "git@gitlab.example.com:group/project.git",
		TargetRef: "refs/heads/main",
		Commit:    "fedcba0987654321",
	}

	result, err := fetcher.FetchRepository(context.Background(), stepruntime.RepositoryFetchRequest{Repo: repo})
	if err != nil {
		t.Fatalf("FetchRepository error: %v", err)
	}

	if !cloneCalled || !fetchCalled || !checkoutCalled {
		t.Fatalf("expected clone=%v fetch=%v checkout=%v", cloneCalled, fetchCalled, checkoutCalled)
	}
	if tokens.calls != 1 {
		t.Fatalf("expected one token request, got %d", tokens.calls)
	}
	if result.Artifact.CID == "" {
		t.Fatalf("expected artifact CID")
	}
	if result.Commit != "abcdef1234567890" {
		t.Fatalf("unexpected commit: %s", result.Commit)
	}
	if result.Ref != repo.TargetRef {
		t.Fatalf("unexpected ref: %s", result.Ref)
	}
}

type stubPublisher struct{}

func (p *stubPublisher) Publish(ctx context.Context, req stepruntime.ArtifactRequest) (stepruntime.PublishedArtifact, error) {
	return stepruntime.PublishedArtifact{
		CID:    "bafy-clone",
		Digest: "sha256:stub",
		Size:   1024,
		Kind:   req.Kind,
	}, nil
}

type stubTokenSource struct {
	token Token
	calls int
}

func (s *stubTokenSource) IssueToken(ctx context.Context, repo contracts.RepoMaterialization) (Token, error) {
	s.calls++
	return s.token, nil
}
