package hydration

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	stepruntime "github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// GitFetcherOptions configures the Git-based repository fetcher.
type GitFetcherOptions struct {
    Publisher   stepruntime.ArtifactPublisher
    TokenSource TokenSource
    GitBinary   string
    // PublishSnapshot controls whether the archived repo snapshot is uploaded
    // to remote storage (IPFS Cluster). When false, the snapshot tarball path
    // is returned without a CID and the workspace hydrator will use it directly.
    PublishSnapshot bool
}

// GitFetcher clones Git repositories and publishes snapshot tarballs.
type GitFetcher struct {
    publisher stepruntime.ArtifactPublisher
    gitBinary string
    tokens    TokenSource
    publish   bool
}

// NewGitFetcher constructs a Git-backed repository fetcher.
func NewGitFetcher(opts GitFetcherOptions) (*GitFetcher, error) {
	if opts.Publisher == nil {
		return nil, errors.New("hydration: publisher required")
	}
	git := strings.TrimSpace(opts.GitBinary)
	if git == "" {
		git = "git"
	}
    publish := opts.PublishSnapshot
    // Default to publishing unless explicitly disabled via env.
    if !publish {
        if v := strings.TrimSpace(os.Getenv("PLOY_HYDRATION_PUBLISH_SNAPSHOT")); v != "" {
            publish = strings.EqualFold(v, "1") || strings.EqualFold(v, "true")
        } else {
            publish = true
        }
    }
    return &GitFetcher{
        publisher: opts.Publisher,
        gitBinary: git,
        tokens:    opts.TokenSource,
        publish:   publish,
    }, nil
}

// FetchRepository clones the repository and returns an archived snapshot artifact.
func (f *GitFetcher) FetchRepository(ctx context.Context, req stepruntime.RepositoryFetchRequest) (stepruntime.RepositoryFetchResult, error) {
	repo := req.Repo
	if strings.TrimSpace(repo.URL) == "" {
		return stepruntime.RepositoryFetchResult{}, errors.New("hydration: repo url required")
	}

	workDir, err := os.MkdirTemp("", "ploy-git-*")
	if err != nil {
		return stepruntime.RepositoryFetchResult{}, fmt.Errorf("hydration: create workdir: %w", err)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	repoDir := filepath.Join(workDir, "repo")
	cloneURL, err := normalizeRepoURL(repo.URL)
	if err != nil {
		return stepruntime.RepositoryFetchResult{}, err
	}

	var tokenValue string
	if f.tokens != nil {
		tok, err := f.tokens.IssueToken(ctx, repo)
		if err != nil {
			return stepruntime.RepositoryFetchResult{}, err
		}
		tokenValue = strings.TrimSpace(tok.Value)
	}

	if err := f.cloneRepo(ctx, cloneURL, repo, repoDir, tokenValue); err != nil {
		return stepruntime.RepositoryFetchResult{}, err
	}

	commit, err := f.resolveCommit(ctx, repoDir)
	if err != nil {
		return stepruntime.RepositoryFetchResult{}, err
	}

	if err := os.RemoveAll(filepath.Join(repoDir, ".git")); err != nil {
		return stepruntime.RepositoryFetchResult{}, fmt.Errorf("hydration: clean repo metadata: %w", err)
	}

	tarFile, digest, err := archiveRepository(repoDir)
	if err != nil {
		return stepruntime.RepositoryFetchResult{}, err
	}

    var ref contracts.StepInputArtifactRef
    if f.publish {
        artifact, err := f.publisher.Publish(ctx, stepruntime.ArtifactRequest{
            Kind: stepruntime.ArtifactKindSnapshot,
            Path: tarFile,
        })
        if err != nil {
            return stepruntime.RepositoryFetchResult{}, fmt.Errorf("hydration: publish snapshot: %w", err)
        }
        ref = contracts.StepInputArtifactRef{
            CID:    artifact.CID,
            Digest: firstNonEmpty(artifact.Digest, digest),
            Size:   artifact.Size,
        }
    } else {
        // Do not publish; return tar path only and let the hydrator use it directly.
        ref = contracts.StepInputArtifactRef{}
    }

	return stepruntime.RepositoryFetchResult{
        Artifact: ref,
        TarPath:  tarFile,
        Commit:   commit,
        Ref:      repo.TargetRef,
    }, nil
}

func normalizeRepoURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("hydration: repo url required")
	}
	if strings.HasPrefix(trimmed, "git@") {
		remainder := strings.TrimPrefix(trimmed, "git@")
		parts := strings.SplitN(remainder, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("hydration: unsupported git url %q", raw)
		}
		host := strings.TrimSpace(parts[0])
		path := strings.TrimPrefix(parts[1], "/")
		return fmt.Sprintf("https://%s/%s", host, path), nil
	}
	return trimmed, nil
}

func (f *GitFetcher) cloneRepo(ctx context.Context, repoURL string, repo contracts.RepoMaterialization, dest string, token string) error {
	args := []string{"clone", "--depth", "1"}
	if ref := strings.TrimSpace(repo.TargetRef); ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, repoURL, dest)
	if err := gitRun(ctx, f.gitBinary, withToken(token, args...)...); err != nil {
		return fmt.Errorf("hydration: git clone %s: %w", repoURL, err)
	}

	if commit := strings.TrimSpace(repo.Commit); commit != "" {
		fetchArgs := withToken(token, "-C", dest, "fetch", "--depth", "1", "origin", commit)
		if err := gitRun(ctx, f.gitBinary, fetchArgs...); err != nil {
			return fmt.Errorf("hydration: git fetch %s: %w", commit, err)
		}
		if err := gitRun(ctx, f.gitBinary, "-C", dest, "checkout", commit); err != nil {
			return fmt.Errorf("hydration: git checkout %s: %w", commit, err)
		}
	}
	return nil
}

func (f *GitFetcher) resolveCommit(ctx context.Context, repoDir string) (string, error) {
	output, err := gitOutput(ctx, f.gitBinary, "-C", repoDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("hydration: rev-parse: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func archiveRepository(src string) (string, string, error) {
	file, err := os.CreateTemp("", "ploy-snapshot-*.tar")
	if err != nil {
		return "", "", fmt.Errorf("hydration: create snapshot tar: %w", err)
	}

	hash := sha256.New()
	writer := io.MultiWriter(file, hash)
	tw := tar.NewWriter(writer)

	err = filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == src {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.ModTime = info.ModTime().UTC().Truncate(time.Second)

		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			header.Linkname = link
		}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
		return nil
	})

	closeErr := tw.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", "", fmt.Errorf("hydration: archive repo: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", "", fmt.Errorf("hydration: close snapshot tar: %w", err)
	}
	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	return file.Name(), digest, nil
}

var (
	gitRun    = runGit
	gitOutput = runGitOutput
)

func runGit(ctx context.Context, git string, args ...string) error {
	cmd := exec.CommandContext(ctx, git, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func runGitOutput(ctx context.Context, git string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, git, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func withToken(token string, args ...string) []string {
	if strings.TrimSpace(token) == "" {
		return args
	}
	header := fmt.Sprintf("http.extraheader=PRIVATE-TOKEN: %s", token)
	prefixed := []string{"-c", header}
	return append(prefixed, args...)
}
