package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
)

// advanceWorkspaceBaseline commits successful mig changes in the sticky
// workspace so the next mig diff is incremental against the previous step.
func advanceWorkspaceBaseline(ctx context.Context, workspace string, runID types.RunID, jobID types.JobID, diffUploaded bool) error {
	message := fmt.Sprintf("Ploy: apply changes for run %s job %s", runID.String(), jobID.String())
	committed, err := gitpkg.EnsureCommit(ctx, workspace, "ploy-baseline", "ploy-baseline@ploy.local", message)
	if err == nil && committed {
		slog.Info("advanced sticky workspace baseline", "run_id", runID.String(), "job_id", jobID.String(), "diff_uploaded", diffUploaded)
	}
	return err
}

// --- Workspace and file utilities ---

const defaultBearerTokenPath = "/etc/ploy/bearer-token"

// bearerTokenPath returns the path to the worker bearer token file,
// overridable for tests via PLOY_NODE_BEARER_TOKEN_PATH.
func bearerTokenPath() string {
	if v := os.Getenv("PLOY_NODE_BEARER_TOKEN_PATH"); v != "" {
		return v
	}
	return defaultBearerTokenPath
}

// createWorkspaceDir creates a temporary workspace directory for a single run.
func createWorkspaceDir() (string, error) {
	base := os.Getenv("PLOYD_CACHE_HOME")
	if base == "" {
		base = os.TempDir()
	}
	if err := os.MkdirAll(base, 0o750); err != nil {
		return "", err
	}
	absBase, err := filepath.Abs(base)
	if err == nil {
		base = absBase
	}
	return os.MkdirTemp(base, "ploy-run-*")
}

// listFilesRecursive returns whether directory has any files and a slice of absolute file paths.
func listFilesRecursive(root string) (bool, []string) {
	var out []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			slog.Warn("file walk error", "path", path, "error", err)
			return nil
		}
		if info == nil || info.IsDir() {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return len(out) > 0, out
}
