package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

var ErrMaterializeTimeout = errors.New("snapshot materialization timed out")

type Metadata struct {
	RepoURL         string
	BaseRef         string
	SourceCommitSHA string
}

type Service struct {
	cacheDir string
	auth     gitauth.Options
}

type Options struct {
	CacheDir string
	Auth     gitauth.Options
}

func NewService(opts Options) *Service {
	cacheDir := strings.TrimSpace(opts.CacheDir)
	if cacheDir == "" {
		cacheDir = os.TempDir()
	}
	return &Service{cacheDir: cacheDir, auth: opts.Auth}
}

func (s *Service) WriteTarGz(ctx context.Context, meta Metadata, w io.Writer) error {
	repoURL := domaintypes.NormalizeRepoURL(meta.RepoURL)
	if repoURL == "" {
		return fmt.Errorf("repo_url is empty")
	}
	if err := domaintypes.RepoURL(repoURL).Validate(); err != nil {
		return fmt.Errorf("repo_url: %w", err)
	}
	commitSHA := normalizeFullCommitSHA(meta.SourceCommitSHA)
	if commitSHA == "" {
		return fmt.Errorf("source_commit_sha must be a lowercase 40-hex sha")
	}

	parent, err := os.MkdirTemp(os.TempDir(), "ploy-snapshot-*")
	if err != nil {
		return fmt.Errorf("create snapshot temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(parent) }()
	workspace := filepath.Join(parent, "repo")

	fetcher, err := hydration.NewGitFetcher(hydration.GitFetcherOptions{CacheDir: s.cacheDir})
	if err != nil {
		return fmt.Errorf("create git fetcher: %w", err)
	}
	repo := &contracts.RepoMaterialization{
		URL:     domaintypes.RepoURL(repoURL),
		BaseRef: domaintypes.GitRef(strings.TrimSpace(meta.BaseRef)),
		Commit:  domaintypes.CommitSHA(commitSHA),
	}
	if err := fetcher.Fetch(ctx, repo, workspace, s.auth); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return ErrMaterializeTimeout
		}
		return fmt.Errorf("materialize repo snapshot: %w", err)
	}
	if err := verifyHEAD(ctx, workspace, commitSHA); err != nil {
		return err
	}
	return writeDirectoryTarGz(workspace, w)
}

func normalizeFullCommitSHA(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) != 40 || strings.ToLower(s) != s {
		return ""
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ""
		}
	}
	return s
}

func verifyHEAD(ctx context.Context, dir, want string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("verify snapshot HEAD: %w (output: %s)", err, string(out))
	}
	if got := strings.TrimSpace(string(out)); got != want {
		return fmt.Errorf("snapshot HEAD mismatch: got %s want %s", got, want)
	}
	return nil
}

func writeDirectoryTarGz(root string, w io.Writer) error {
	gz := gzip.NewWriter(w)
	defer func() { _ = gz.Close() }()
	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

	return filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
			return fmt.Errorf("unsafe snapshot path %q", rel)
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	})
}
