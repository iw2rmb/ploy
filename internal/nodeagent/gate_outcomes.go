package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func (r *runController) persistGateOutcomes(ctx context.Context, req StartRunRequest, mounts step.JobMounts) error {
	var errs []error
	// Read job-local output: the shared SBOM can belong to an earlier gate.
	for _, report := range []struct{ name, filename string }{{"sbom", gateSBOMFilename}, {"cves", "grype.json"}} {
		filename := filepath.Join(mounts.Out, report.filename)
		info, err := os.Lstat(filename)
		if err != nil {
			if !os.IsNotExist(err) {
				errs = append(errs, err)
			}
			continue
		}
		if !info.Mode().IsRegular() || info.Size() > migsapi.MaxGateOutcomeBytes {
			errs = append(errs, fmt.Errorf("invalid gate %s file size or type", report.name))
			continue
		}
		body, err := os.ReadFile(filename)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !json.Valid(body) {
			errs = append(errs, fmt.Errorf("invalid gate %s JSON", report.name))
			continue
		}

		if _, _, err := r.uploader.UploadArtifactEntries(ctx, req.RunID, req.JobID, []ArtifactBundleEntry{{SourcePath: filename, ArchivePath: report.filename}}, report.name); err != nil {
			errs = append(errs, fmt.Errorf("upload gate %s: %w", report.name, err))
		}
	}
	if err := r.persistGateSBOM(ctx, req, mounts.Out); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func clearGateOutcomes(outDir string) error {
	// Retried jobs reuse /out; only fresh gate output may be published.
	for _, filename := range []string{gateSBOMFilename, "grype.json"} {
		if err := os.Remove(filepath.Join(outDir, filename)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
