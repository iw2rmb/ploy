package nodeagent

import (
	"context"
	"fmt"
	"io"
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
		sourcePath, err := runJobOutFile(req.RunID, sourceJobID, sourceOutPath)
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
		if !info.Mode().IsRegular() {
			return fmt.Errorf("materialize mig_context.in_from[%d] from %q: source is not a regular file", i, sourcePath)
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
			return fmt.Errorf("mkdir in_from destination dir: %w", err)
		}
		if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove existing in_from destination %s: %w", destPath, err)
		}
		if err := copyInFromFile(sourcePath, destPath, info.Mode().Perm()); err != nil {
			return fmt.Errorf("materialize in_from destination %s: %w", destPath, err)
		}
	}

	return nil
}

func copyInFromFile(sourcePath, destPath string, mode os.FileMode) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	tmp, err := os.CreateTemp(filepath.Dir(destPath), "."+filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("copy source: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	cleanup = false
	return nil
}
