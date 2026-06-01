package handlers

import (
	"context"
	"net/http"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

type WaveSummary struct {
	ID         domaintypes.WaveID     `json:"id"`
	MigID      domaintypes.MigID      `json:"mig_id"`
	SpecID     domaintypes.SpecID     `json:"spec_id"`
	CreatedBy  *string                `json:"created_by,omitempty"`
	Status     domaintypes.WaveStatus `json:"status"`
	CreatedAt  string                 `json:"created_at"`
	StartedAt  *string                `json:"started_at,omitempty"`
	FinishedAt *string                `json:"finished_at,omitempty"`
	Counts     *domaintypes.RunCounts `json:"run_counts,omitempty"`
}

func waveToSummary(wave store.Wave) WaveSummary {
	summary := WaveSummary{
		ID:        wave.ID,
		MigID:     wave.MigID,
		SpecID:    wave.SpecID,
		CreatedBy: wave.CreatedBy,
		Status:    wave.Status,
		CreatedAt: wave.CreatedAt.Time.UTC().Format(http.TimeFormat),
	}
	if wave.StartedAt.Valid {
		v := wave.StartedAt.Time.UTC().Format(http.TimeFormat)
		summary.StartedAt = &v
	}
	if wave.FinishedAt.Valid {
		v := wave.FinishedAt.Time.UTC().Format(http.TimeFormat)
		summary.FinishedAt = &v
	}
	return summary
}

func getWaveCounts(ctx context.Context, st store.Store, waveID domaintypes.WaveID) (*domaintypes.RunCounts, error) {
	rows, err := st.CountRunsByWaveStatus(ctx, waveID)
	if err != nil {
		return nil, err
	}
	counts := &domaintypes.RunCounts{}
	for _, row := range rows {
		counts.Total += row.Count
		switch row.Status {
		case domaintypes.RunStatusQueued:
			counts.Queued = row.Count
		case domaintypes.RunStatusRunning:
			counts.Running = row.Count
		case domaintypes.RunStatusSuccess:
			counts.Success = row.Count
		case domaintypes.RunStatusFail:
			counts.Fail = row.Count
		case domaintypes.RunStatusCancelled:
			counts.Cancelled = row.Count
		}
	}
	counts.DerivedStatus = lifecycle.DeriveWaveStatus(counts)
	return counts, nil
}

func getWaveHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		waveID, ok := parseRequiredPathIDOrWriteError[domaintypes.WaveID](w, r, "wave_id")
		if !ok {
			return
		}
		wave, err := st.GetWave(r.Context(), waveID)
		if err != nil {
			if isNoRowsError(err) {
				writeHTTPError(w, http.StatusNotFound, "wave not found")
				return
			}
			serverError(w, "get wave", "get wave", err, "wave_id", waveID)
			return
		}
		summary := waveToSummary(wave)
		if counts, err := getWaveCounts(r.Context(), st, waveID); err == nil {
			summary.Counts = counts
		}
		writeJSON(w, http.StatusOK, summary)
	}
}

func listWaveRunsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		waveID, ok := parseRequiredPathIDOrWriteError[domaintypes.WaveID](w, r, "wave_id")
		if !ok {
			return
		}
		runs, err := st.ListRunsWithURLByWave(r.Context(), waveID)
		if err != nil {
			serverError(w, "list wave runs", "list runs", err, "wave_id", waveID)
			return
		}
		out := make([]RunResponse, 0, len(runs))
		for _, run := range runs {
			out = append(out, runToResponse(store.Run{
				ID:              run.ID,
				WaveID:          run.WaveID,
				MigID:           run.MigID,
				SpecID:          run.SpecID,
				RepoID:          run.RepoID,
				RepoBaseRef:     run.RepoBaseRef,
				SourceCommitSha: run.SourceCommitSha,
				RepoSha0:        run.RepoSha0,
				CreatedBy:       run.CreatedBy,
				Status:          run.Status,
				Attempt:         run.Attempt,
				LastError:       run.LastError,
				CreatedAt:       run.CreatedAt,
				StartedAt:       run.StartedAt,
				FinishedAt:      run.FinishedAt,
				Stats:           run.Stats,
			}, run.RepoUrl))
		}
		writeJSON(w, http.StatusOK, struct {
			Runs []RunResponse `json:"runs"`
		}{Runs: out})
	}
}

func cancelWaveHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		waveID, ok := parseRequiredPathIDOrWriteError[domaintypes.WaveID](w, r, "wave_id")
		if !ok {
			return
		}
		if err := st.CancelWave(r.Context(), waveID); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to cancel wave: %v", err)
			return
		}
		wave, err := st.GetWave(r.Context(), waveID)
		if err != nil {
			serverError(w, "cancel wave", "reload wave", err, "wave_id", waveID)
			return
		}
		writeJSON(w, http.StatusOK, waveToSummary(wave))
	}
}
