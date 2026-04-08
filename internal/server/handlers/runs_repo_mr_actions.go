package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func enqueueAutoMRCreateAction(ctx context.Context, st store.Store, runID domaintypes.RunID, runRepo store.RunRepo) error {
	_, _, _, err := ensureMRCreateAction(ctx, st, runID, runRepo, false)
	return err
}

func ensureMRCreateAction(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	runRepo store.RunRepo,
	force bool,
) (store.RunRepoAction, bool, string, error) {
	if !lifecycle.IsTerminalRunRepoStatus(runRepo.Status) {
		return store.RunRepoAction{}, false, "", fmt.Errorf("run repo is not terminal")
	}

	run, err := st.GetRun(ctx, runID)
	if err != nil {
		return store.RunRepoAction{}, false, "", fmt.Errorf("get run: %w", err)
	}
	specRow, err := st.GetSpec(ctx, run.SpecID)
	if err != nil {
		return store.RunRepoAction{}, false, "", fmt.Errorf("get spec: %w", err)
	}
	spec, err := contracts.ParseMigSpecJSON(specRow.Spec)
	if err != nil {
		return store.RunRepoAction{}, false, "", fmt.Errorf("parse spec: %w", err)
	}

	if !force && !mrActionEnabledForRepoStatus(runRepo.Status, spec) {
		return store.RunRepoAction{}, false, "", nil
	}

	actionType := domaintypes.RunRepoActionTypeMRCreate.String()
	existing, existingErr := st.GetRunRepoActionByKey(ctx, store.GetRunRepoActionByKeyParams{
		RunID:      runID,
		RepoID:     runRepo.RepoID,
		Attempt:    runRepo.Attempt,
		ActionType: actionType,
	})
	if existingErr == nil {
		return existing, false, actionMRURL(existing.Meta), nil
	}
	if !errors.Is(existingErr, pgx.ErrNoRows) {
		return store.RunRepoAction{}, false, "", fmt.Errorf("get existing action: %w", existingErr)
	}

	created, createErr := st.CreateRunRepoAction(ctx, store.CreateRunRepoActionParams{
		ID:         domaintypes.NewJobID(),
		RunID:      runID,
		RepoID:     runRepo.RepoID,
		Attempt:    runRepo.Attempt,
		ActionType: actionType,
		Status:     domaintypes.JobStatusQueued,
		Meta:       []byte(`{}`),
	})
	if createErr != nil {
		return store.RunRepoAction{}, false, "", fmt.Errorf("create action: %w", createErr)
	}
	return created, true, "", nil
}

func mrActionEnabledForRepoStatus(status domaintypes.RunRepoStatus, spec *contracts.MigSpec) bool {
	if spec == nil {
		return false
	}
	switch status {
	case domaintypes.RunRepoStatusSuccess:
		return spec.MROnSuccess != nil && *spec.MROnSuccess
	case domaintypes.RunRepoStatusFail:
		return spec.MROnFail != nil && *spec.MROnFail
	default:
		return false
	}
}

func actionMRURL(meta []byte) string {
	var payload struct {
		MRURL string `json:"mr_url"`
	}
	if err := json.Unmarshal(meta, &payload); err != nil {
		return ""
	}
	return payload.MRURL
}

type createRunRepoMRActionResponse struct {
	ActionID   domaintypes.JobID `json:"action_id"`
	ActionType string            `json:"action_type"`
	Status     string            `json:"status"`
	MRURL      string            `json:"mr_url,omitempty"`
}

func createRunRepoMRActionHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID, err := parseRequiredPathID[domaintypes.RunID](r, "run_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}
		repoID, err := parseRequiredPathID[domaintypes.RepoID](r, "repo_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		runRepo, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "run repo not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to load run repo: %v", err)
			return
		}
		action, _, mrURL, ensureErr := ensureMRCreateAction(r.Context(), st, runID, runRepo, true)
		if ensureErr != nil {
			writeHTTPError(w, http.StatusConflict, "failed to enqueue mr action: %v", ensureErr)
			return
		}

		resp := createRunRepoMRActionResponse{
			ActionID:   action.ID,
			ActionType: action.ActionType,
			Status:     action.Status.String(),
			MRURL:      mrURL,
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
