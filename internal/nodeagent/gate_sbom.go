package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/sbom"
)

const gateSBOMFilename = "sbom.spdx.json"
const gateModuleSBOMFilename = "sbom.modules.json"

func (r *runController) persistGateSBOM(ctx context.Context, req StartRunRequest, shareDir string) error {
	if strings.TrimSpace(shareDir) == "" {
		return nil
	}
	packages, err := readGateSBOM(shareDir)
	if err != nil || packages == nil {
		return err
	}
	if err := r.SaveJobSBOM(ctx, req.JobID, req.ResumeCount, packages); err != nil {
		return fmt.Errorf("upload gate sbom rows: %w", err)
	}
	slog.Info("persisted gate sbom rows",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"job_type", req.JobType,
		"row_count", len(packages),
	)
	return nil
}

func readGateSBOM(dir string) ([]migsapi.RunSBOMPackage, error) {
	modulePath := filepath.Join(dir, gateModuleSBOMFilename)
	raw, err := readGateSBOMFile(modulePath)
	if err == nil {
		var rows []migsapi.RunSBOMPackage
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, fmt.Errorf("parse %s: %w", modulePath, err)
		}
		if rows == nil {
			return nil, fmt.Errorf("expected array in %s", modulePath)
		}
		for i, row := range rows {
			if strings.TrimSpace(row.ModuleID) == "" || strings.TrimSpace(row.Package) == "" || strings.TrimSpace(row.Version) == "" {
				return nil, fmt.Errorf("incomplete package at index %d in %s", i, modulePath)
			}
		}
		return rows, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	// Gate images from before module attribution publish only the aggregate SPDX file.
	sbomPath := filepath.Join(dir, gateSBOMFilename)
	raw, err = readGateSBOMFile(sbomPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := sbom.ExtractPackagesFromJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("parse gate sbom %s: %w", sbomPath, err)
	}

	packages := make([]migsapi.RunSBOMPackage, 0, len(rows))
	for _, row := range rows {
		packages = append(packages, migsapi.RunSBOMPackage{
			Package: row.Name,
			Version: row.Version,
		})
	}
	return packages, nil
}

func readGateSBOMFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > migsapi.MaxGateOutcomeBytes {
		return nil, fmt.Errorf("invalid gate SBOM file size or type: %s", path)
	}
	return os.ReadFile(path)
}
