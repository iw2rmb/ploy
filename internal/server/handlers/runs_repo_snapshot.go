package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/snapshot"
	"github.com/iw2rmb/ploy/internal/store"
)

type repoSnapshotWriter interface {
	WriteTarGz(ctx context.Context, meta snapshot.Metadata, w io.Writer) error
}

func getRunSnapshotHandler(st store.Store, snapshots repoSnapshotWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if snapshots == nil {
			writeHTTPError(w, http.StatusBadGateway, "snapshot service unavailable")
			return
		}
		runID, ok := parseRequiredPathIDOrWriteError[domaintypes.RunID](w, r, "run_id")
		if !ok {
			return
		}
		repoID, ok := runRepoIDFromPathOrRun(w, r, st, runID)
		if !ok {
			return
		}
		nodeID, ok := requireNodeUUIDHeader(w, r)
		if !ok {
			return
		}

		metaRow, err := st.GetRunSnapshotMetadata(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "repo not found")
				return
			}
			serverError(w, "repo snapshot", "load metadata", err, "run_id", runID.String(), "repo_id", repoID.String())
			return
		}
		if metaRow.RepoID != repoID {
			writeHTTPError(w, http.StatusNotFound, "repo not found")
			return
		}

		authorized, err := snapshotAuthorizedForNode(r.Context(), st, runID, repoID, nodeID)
		if err != nil {
			serverError(w, "repo snapshot", "authorize", err, "run_id", runID.String(), "repo_id", repoID.String(), "node_id", nodeID.String())
			return
		}
		if !authorized {
			writeHTTPError(w, http.StatusForbidden, "node is not assigned current work for this run repo")
			return
		}

		sha := strings.TrimSpace(metaRow.SourceCommitSha)
		if !sha40Pattern.MatchString(sha) {
			writeHTTPError(w, http.StatusConflict, "run repo is not snapshot-ready")
			return
		}

		cleanURL := domaintypes.NormalizeRepoURL(metaRow.RepoUrl)
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("X-Ploy-Repo-SHA", sha)
		w.Header().Set("X-Ploy-Repo-URL", cleanURL)
		w.Header().Set("X-Ploy-Repo-Base-Ref", metaRow.RepoBaseRef)
		if err := snapshots.WriteTarGz(r.Context(), snapshot.Metadata{
			RepoURL:         cleanURL,
			BaseRef:         metaRow.RepoBaseRef,
			SourceCommitSHA: sha,
		}, w); err != nil {
			if errors.Is(err, snapshot.ErrMaterializeTimeout) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				writeHTTPError(w, http.StatusGatewayTimeout, "snapshot materialization timed out")
				return
			}
			writeHTTPError(w, http.StatusBadGateway, "snapshot materialization failed: %v", err)
			return
		}
	}
}

func snapshotAuthorizedForNode(ctx context.Context, st store.Store, runID domaintypes.RunID, repoID domaintypes.RepoID, nodeID domaintypes.NodeID) (bool, error) {
	ok, err := st.HasRunningJobForRunNode(ctx, store.HasRunningJobForRunNodeParams{
		RunID:  runID,
		NodeID: &nodeID,
	})
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	return st.HasRunningActionForRunNode(ctx, store.HasRunningActionForRunNodeParams{
		RunID:  runID,
		NodeID: &nodeID,
	})
}
