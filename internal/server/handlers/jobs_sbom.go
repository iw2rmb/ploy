package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func saveJobSBOMHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseRequiredPathIDOrWriteError[domaintypes.JobID](w, r, "job_id")
		if !ok {
			return
		}

		nodeIDHeader, ok := requireNodeUUIDHeader(w, r)
		if !ok {
			return
		}

		var req migsapi.JobSBOMUploadRequest
		if err := decodeRequestJSON(w, r, &req, ingestMaxBodySize); err != nil {
			return
		}

		job, ok := getJobOrFail(w, r, st, jobID, "save job sbom")
		if !ok {
			return
		}

		if !assertJobAssignedToNode(w, job, nodeIDHeader) {
			return
		}
		if job.Status != domaintypes.JobStatusRunning {
			writeHTTPError(w, http.StatusConflict, "job status is %s, expected Running", job.Status)
			return
		}
		if !lifecycle.IsGateJobType(job.JobType) {
			writeHTTPError(w, http.StatusConflict, "job type is %s, expected gate", job.JobType)
			return
		}

		rowCount, err := persistJobSBOMPackages(r.Context(), st, job, req.Packages)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to save job sbom: %v", err)
			slog.Error("save job sbom: persist failed", "job_id", jobID, "node_id", nodeIDHeader, "err", err)
			return
		}

		writeJSON(w, http.StatusOK, migsapi.JobSBOMUploadResponse{
			JobID:    jobID,
			RowCount: rowCount,
		})
	}
}

func persistJobSBOMPackages(ctx context.Context, st store.Store, job store.Job, packages []migsapi.RunSBOMPackage) (int, error) {
	if err := st.DeleteSBOMRowsByJob(ctx, job.ID); err != nil {
		return 0, fmt.Errorf("delete rows for job %s: %w", job.ID, err)
	}

	seen := map[string]struct{}{}
	rowCount := 0
	for _, pkg := range packages {
		lib := strings.ToLower(strings.TrimSpace(pkg.Package))
		ver := strings.TrimSpace(pkg.Version)
		if lib == "" || ver == "" {
			continue
		}
		key := lib + "\x00" + ver
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		if err := st.UpsertSBOMRow(ctx, store.UpsertSBOMRowParams{
			JobID:  job.ID,
			RepoID: job.RepoID,
			Lib:    lib,
			Ver:    ver,
		}); err != nil {
			return 0, fmt.Errorf("upsert row %s %s for job %s: %w", lib, ver, job.ID, err)
		}
		rowCount++
	}
	return rowCount, nil
}
