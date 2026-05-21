package nodeagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func (r *runController) materializeMigInFromInputs(
	ctx context.Context,
	req StartRunRequest,
	inDir string,
) error {
	if req.JobType != types.JobTypeMig || req.MigContext == nil || len(req.MigContext.InFrom) == 0 {
		return nil
	}
	if strings.TrimSpace(inDir) == "" {
		return fmt.Errorf("cross-step /in directory is required")
	}
	if err := os.MkdirAll(inDir, 0o750); err != nil {
		return fmt.Errorf("mkdir /in directory: %w", err)
	}

	cleanInDir := filepath.Clean(inDir)
	for i := range req.MigContext.InFrom {
		ref := req.MigContext.InFrom[i]
		sourceOutPath := strings.TrimSpace(ref.SourceOutPath)
		if sourceOutPath == "" {
			parsed, err := contracts.ParseInFromURI(ref.From)
			if err != nil {
				return fmt.Errorf("mig_context.in_from[%d].from: %w", i, err)
			}
			sourceOutPath = parsed.OutPath
		}
		targetPath, err := contracts.NormalizeInFromTarget(ref.To, sourceOutPath)
		if err != nil {
			return fmt.Errorf("mig_context.in_from[%d].to: %w", i, err)
		}
		rel := strings.TrimPrefix(targetPath, "/in/")
		destPath := filepath.Clean(filepath.Join(inDir, filepath.FromSlash(rel)))
		if destPath != cleanInDir && !strings.HasPrefix(destPath, cleanInDir+string(filepath.Separator)) {
			return fmt.Errorf("mig_context.in_from[%d].to: resolved path %s escapes /in", i, destPath)
		}

		sourceJobID := ref.SourceJobID
		if sourceJobID.IsZero() {
			return fmt.Errorf("mig_context.in_from[%d].source_job_id: required", i)
		}
		sourcePath, err := runRepoJobOutFile(req.RunID, req.RepoID, sourceJobID, sourceOutPath)
		if err != nil {
			return fmt.Errorf("mig_context.in_from[%d].source_out_path: %w", i, err)
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			return fmt.Errorf("materialize mig_context.in_from[%d] from %q: %w", i, sourcePath, err)
		}
		if info.IsDir() {
			return fmt.Errorf("materialize mig_context.in_from[%d] from %q: source is a directory", i, sourcePath)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
			return fmt.Errorf("mkdir in_from destination dir: %w", err)
		}
		if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove existing in_from destination %s: %w", destPath, err)
		}
		if err := os.Symlink(sourcePath, destPath); err != nil {
			return fmt.Errorf("link in_from destination %s: %w", destPath, err)
		}
	}

	return nil
}
