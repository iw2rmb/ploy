package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// Allow room for the packages wrapper and the execution resume count.
const maxSBOMEnvelopeBytes = 128

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
		if err := decodeRequestJSON(w, r, &req, migsapi.MaxGateOutcomeBytes+maxSBOMEnvelopeBytes); err != nil {
			return
		}

		ctx := r.Context()
		var rowCount int
		status := http.StatusInternalServerError
		err := st.WithJobExecution(ctx, jobID, req.ResumeCount, func(scoped store.Store, job store.Job) error {
			if job.NodeID == nil || *job.NodeID != nodeIDHeader {
				status = http.StatusForbidden
				return errors.New("job not assigned to this node")
			}
			if job.Status != domaintypes.JobStatusRunning || !lifecycle.IsGateJobType(job.JobType) {
				status = http.StatusConflict
				return fmt.Errorf("expected a Running gate job, got %s %s", job.Status, job.JobType)
			}
			rows := jobSBOMRows(job, req.Packages)
			if err := scoped.DeleteSBOMRowsByJob(ctx, job.ID); err != nil {
				return err
			}
			count, err := scoped.InsertSBOMRows(ctx, rows)
			rowCount = int(count)
			return err
		})
		if errors.Is(err, store.ErrJobExecutionStale) {
			writeHTTPError(w, http.StatusConflict, "SBOM upload belongs to an earlier execution")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			writeHTTPError(w, http.StatusNotFound, "job not found")
			return
		}
		if err != nil {
			writeHTTPError(w, status, "%v", err)
			if status == http.StatusInternalServerError {
				slog.Error("save job sbom: persist failed", "job_id", jobID, "node_id", nodeIDHeader, "err", err)
			}
			return
		}

		writeJSON(w, http.StatusOK, migsapi.JobSBOMUploadResponse{
			JobID:    jobID,
			RowCount: rowCount,
		})
	}
}

func jobSBOMRows(job store.Job, packages []migsapi.RunSBOMPackage) []store.InsertSBOMRowsParams {
	rows := make([]store.InsertSBOMRowsParams, 0, len(packages))
	seen := make(map[migsapi.RunSBOMPackage]struct{}, len(packages))
	for _, pkg := range packages {
		pkg.Package = strings.ToLower(strings.TrimSpace(pkg.Package))
		pkg.Version = strings.TrimSpace(pkg.Version)
		if pkg.Package == "" || pkg.Version == "" {
			continue
		}
		if _, exists := seen[pkg]; exists {
			continue
		}
		seen[pkg] = struct{}{}
		param := store.InsertSBOMRowsParams{JobID: job.ID, RepoID: job.RepoID, Lib: pkg.Package, Ver: pkg.Version}
		if pkg.ModuleID != "" {
			param.ModuleID = &pkg.ModuleID
		}
		rows = append(rows, param)
	}
	return rows
}
