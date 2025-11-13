package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/worker/hydration"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// BuildGateExecutor handles execution of build gate validation jobs.
type BuildGateExecutor struct {
	cfg Config
}

// NewBuildGateExecutor creates a new build gate executor.
func NewBuildGateExecutor(cfg Config) *BuildGateExecutor {
	return &BuildGateExecutor{
		cfg: cfg,
	}
}

// Execute runs a build gate validation job.
func (e *BuildGateExecutor) Execute(ctx context.Context, jobID string, req contracts.BuildGateValidateRequest) (*contracts.BuildGateStageMetadata, error) {
	slog.Info("executing buildgate job", "job_id", jobID)

	// Create ephemeral workspace directory.
	workspaceRoot, err := createWorkspaceDir()
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(workspaceRoot)
	}()

	// Populate workspace based on request type.
	if req.RepoURL != "" && req.Ref != "" {
		// Ref-based: clone repository.
		if err := e.cloneRepo(ctx, req.RepoURL, req.Ref, workspaceRoot); err != nil {
			return nil, fmt.Errorf("clone repo: %w", err)
		}
	} else if len(req.ContentArchive) > 0 {
		// Content-based: extract tarball.
		if err := e.extractArchive(ctx, req.ContentArchive, workspaceRoot); err != nil {
			return nil, fmt.Errorf("extract archive: %w", err)
		}
	} else {
		return nil, fmt.Errorf("invalid request: neither repo_url nor content_archive provided")
	}

	// Create gate executor.
	containerRuntime, err := step.NewDockerContainerRuntime(step.DockerContainerRuntimeOptions{
		PullImage: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create container runtime: %w", err)
	}

	gateExecutor := step.NewDockerGateExecutor(containerRuntime)

	// Build gate spec.
	gateSpec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: req.Profile,
		Env:     make(map[string]string),
	}

	// Execute gate validation.
	metadata, err := gateExecutor.Execute(ctx, gateSpec, workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("execute gate: %w", err)
	}

	slog.Info("buildgate job completed",
		"job_id", jobID,
		"passed", len(metadata.StaticChecks) > 0 && metadata.StaticChecks[0].Passed,
	)

	return metadata, nil
}

// cloneRepo clones a git repository into the workspace.
func (e *BuildGateExecutor) cloneRepo(ctx context.Context, repoURL, ref, workspace string) error {
	slog.Info("cloning repository", "repo_url", repoURL, "ref", ref, "workspace", workspace)

	// Create git fetcher.
	gitFetcher, err := hydration.NewGitFetcher(hydration.GitFetcherOptions{
		PublishSnapshot: false,
	})
	if err != nil {
		return fmt.Errorf("create git fetcher: %w", err)
	}

	// Prepare repo materialization.
	repo := &contracts.RepoMaterialization{
		URL:       types.RepoURL(repoURL),
		TargetRef: types.GitRef(ref),
	}

	// Fetch the repository.
	if err := gitFetcher.Fetch(ctx, repo, workspace); err != nil {
		return fmt.Errorf("fetch repo: %w", err)
	}

	slog.Info("repository cloned successfully", "workspace", workspace)
	return nil
}

// extractArchive extracts a gzipped tar archive into the workspace.
func (e *BuildGateExecutor) extractArchive(ctx context.Context, archiveData []byte, workspace string) error {
	slog.Info("extracting archive", "size", len(archiveData), "workspace", workspace)
	// Try gzip first, fallback to plain tar
	var tarReader *tar.Reader
	if gz, err := gzip.NewReader(bytes.NewReader(archiveData)); err == nil {
		defer gz.Close()
		tarReader = tar.NewReader(gz)
	} else {
		tarReader = tar.NewReader(bytes.NewReader(archiveData))
	}

	// Extract tar contents.
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		// Sanitise header name and construct target path.
		name := strings.TrimPrefix(header.Name, "/")
		clean := filepath.Clean(name)
		target := filepath.Join(workspace, clean)
		// Ensure target resides within workspace (prevent path traversal).
		ws := filepath.Clean(workspace)
		if !(strings.HasPrefix(filepath.Clean(target), ws+string(os.PathSeparator)) || filepath.Clean(target) == ws) {
			return fmt.Errorf("invalid tar entry path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory.
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("create directory %s: %w", target, err)
			}

		case tar.TypeReg:
			// Create parent directory.
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("create parent directory for %s: %w", target, err)
			}

			// Create file.
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create file %s: %w", target, err)
			}

			// Copy contents.
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return fmt.Errorf("write file %s: %w", target, err)
			}
			file.Close()

		case tar.TypeSymlink, tar.TypeLink:
			// Disallow links for safety.
			slog.Warn("skipping link entry in archive", "name", header.Name)
		default:
			// Skip other unsupported types.
			slog.Warn("skipping unsupported tar entry type", "name", header.Name, "type", header.Typeflag)
		}
	}

	slog.Info("archive extracted successfully", "workspace", workspace)
	return nil
}

// byteReaderWrapper wraps a byte slice to implement io.Reader.
type byteReaderWrapper struct {
	data   []byte
	offset int
}

func (b *byteReaderWrapper) Read(p []byte) (n int, err error) {
	if b.offset >= len(b.data) {
		return 0, io.EOF
	}
	n = copy(p, b.data[b.offset:])
	b.offset += n
	return n, nil
}
