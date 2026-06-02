package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/sbom"
)

const gateSBOMFilename = "sbom.spdx.json"

func (r *runController) persistGateSBOM(ctx context.Context, req StartRunRequest, shareDir string) error {
	if strings.TrimSpace(shareDir) == "" {
		return nil
	}
	sbomPath := filepath.Join(shareDir, gateSBOMFilename)
	raw, err := os.ReadFile(sbomPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("gate sbom file not found; skipping persistence",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"job_type", req.JobType,
				"path", sbomPath,
			)
			return nil
		}
		return fmt.Errorf("read gate sbom %s: %w", sbomPath, err)
	}

	rows, err := sbom.ExtractPackagesFromJSON(raw)
	if err != nil {
		return fmt.Errorf("parse gate sbom %s: %w", sbomPath, err)
	}

	packages := make([]migsapi.RunSBOMPackage, 0, len(rows))
	for _, row := range rows {
		packages = append(packages, migsapi.RunSBOMPackage{
			Package: row.Name,
			Version: row.Version,
		})
	}
	if err := r.SaveJobSBOM(ctx, req.JobID, packages); err != nil {
		return fmt.Errorf("upload gate sbom rows: %w", err)
	}
	slog.Info("persisted gate sbom rows",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"job_type", req.JobType,
		"path", sbomPath,
		"row_count", len(packages),
	)
	return nil
}
