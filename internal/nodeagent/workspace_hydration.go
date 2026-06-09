package nodeagent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// prepareStickyWorkspace returns the single mutable per-run workspace
// for a linear job chain. The chain head hydrates sources directly
// into the sticky workspace; later jobs require that workspace to already exist
// on this node.
//
// Parameters:
//   - ctx: Context for cancellation and deadlines.
//   - req: StartRunRequest containing repo URL, base_ref, commit_sha, and job_name.
//   - manifest: StepManifest for this step.
//
// Returns:
//   - workspacePath: Path to the sticky workspace ready for execution.
//   - error: Non-nil if workspace preparation fails.
func (r *runController) prepareStickyWorkspace(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
) (string, error) {
	runID := req.RunID.String()
	workspacePath := workspaceDir(req.RunID)
	if hasGitDir(workspacePath) {
		slog.Info("reusing sticky workspace", "run_id", runID, "job_id", req.JobID, "workspace", workspacePath)
		return workspacePath, nil
	}

	if strings.TrimSpace(req.RepoSHAIn.String()) == "" {
		return "", fmt.Errorf("sticky workspace missing for %s job; linear repo chains must continue on the node that hydrated the chain head", req.JobType)
	}

	if _, err := os.Stat(workspacePath); err == nil {
		slog.Warn("sticky workspace is invalid; rebuilding chain head", "run_id", runID, "job_id", req.JobID, "workspace", workspacePath)
		if rmErr := os.RemoveAll(workspacePath); rmErr != nil {
			return "", fmt.Errorf("remove invalid workspace: %w", rmErr)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect sticky workspace: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o750); err != nil {
		return "", fmt.Errorf("create sticky workspace parent dir: %w", err)
	}

	// Determine repo materialization:
	// - Prefer manifest inputs that already carry hydration.Repo.
	// - Fallback to StartRunRequest repo fields for callers without hydration.
	var repo *contracts.RepoMaterialization
	for _, input := range manifest.Inputs {
		if input.Hydration != nil && input.Hydration.Repo != nil {
			repo = input.Hydration.Repo
			break
		}
	}

	if repo == nil {
		// Derive repo materialization from StartRunRequest, mirroring
		// buildMigManifest semantics.
		tmp := contracts.RepoMaterialization{
			URL:     req.RepoURL,
			BaseRef: req.BaseRef,
			Commit:  req.CommitSHA,
		}
		repo = &tmp
	}

	if err := os.RemoveAll(workspacePath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove stale sticky workspace before hydration: %w", err)
	}
	if err := r.downloadSnapshot(ctx, req, repo, workspacePath); err != nil {
		if removeErr := os.RemoveAll(workspacePath); removeErr != nil {
			slog.Warn("failed to clean up workspace after error", "path", workspacePath, "error", removeErr)
		}
		return "", fmt.Errorf("hydrate sticky workspace: %w", err)
	}

	slog.Info("sticky workspace hydrated successfully", "run_id", runID, "job_id", req.JobID, "workspace", workspacePath)
	return workspacePath, nil
}

func hasGitDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func (r *runController) downloadSnapshot(ctx context.Context, req StartRunRequest, repo *contracts.RepoMaterialization, workspacePath string) error {
	if repo == nil {
		return fmt.Errorf("repo materialization is required")
	}
	if strings.TrimSpace(req.CommitSHA.String()) == "" {
		return fmt.Errorf("commit_sha is required for snapshot hydration")
	}
	if err := os.MkdirAll(workspacePath, 0o750); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	apiPath := fmt.Sprintf("/v1/runs/%s/snapshot", req.RunID)
	u := MustBuildURL(r.cfg.ServerURL, apiPath)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("create snapshot request: %w", err)
	}
	httpReq.Header.Set("PLOY_NODE_UUID", r.cfg.NodeID.String())
	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("download snapshot: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download snapshot failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := unpackTarGz(resp.Body, workspacePath); err != nil {
		return fmt.Errorf("unpack snapshot: %w", err)
	}
	if err := verifyWorkspaceHEAD(ctx, workspacePath, req.CommitSHA.String()); err != nil {
		return err
	}
	return nil
}

func unpackTarGz(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeTarTarget(dest, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)&0o777); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(f, tr)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported tar entry %q type %d", hdr.Name, hdr.Typeflag)
		}
	}
}

func safeTarTarget(root, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.IsAbs(name) {
		return "", fmt.Errorf("unsafe tar path %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("unsafe tar path %q", name)
	}
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("unsafe tar path %q", name)
	}
	return target, nil
}

func verifyWorkspaceHEAD(ctx context.Context, workspace, want string) error {
	head, err := resolveGitHEAD(ctx, workspace)
	if err != nil {
		return fmt.Errorf("verify hydrated snapshot HEAD: %w", err)
	}
	if head != strings.TrimSpace(want) {
		return fmt.Errorf("hydrated snapshot HEAD mismatch: got %s want %s", head, strings.TrimSpace(want))
	}
	return nil
}

func resolveGitHEAD(ctx context.Context, workspace string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", workspace, "rev-parse", "HEAD")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=echo")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w (output: %s)", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}
