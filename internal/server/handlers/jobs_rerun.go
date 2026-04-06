package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

const (
	rerunMetaKey      = "_rerun"
	rerunModeDriftOK  = "drift-ok"
	rerunMetaAlterKey = "alter"
)

type rerunAlter struct {
	Image string
	Envs  map[string]string
	In    []string
}

type rerunResponse struct {
	RunID           domaintypes.RunID  `json:"run_id"`
	RepoID          domaintypes.RepoID `json:"repo_id"`
	Attempt         int32              `json:"attempt"`
	RootJobID       domaintypes.JobID  `json:"root_job_id"`
	CopiedFromJobID domaintypes.JobID  `json:"copied_from_job_id"`
}

// rerunJobHandler creates a new repo attempt starting from the provided job.
// Endpoint: POST /v1/jobs/{job_id}/rerun
// MVP scope: source job type must be heal or re_gate.
func rerunJobHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := parseRequiredPathID[domaintypes.JobID](r, "job_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		var req struct {
			Alter map[string]any `json:"alter"`
		}
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		alter, err := normalizeRerunAlter(req.Alter)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "alter: %v", err)
			return
		}

		sourceJob, err := st.GetJob(r.Context(), jobID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "job not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get job: %v", err)
			return
		}

		sourceType := domaintypes.JobType(sourceJob.JobType)
		if sourceType != domaintypes.JobTypeHeal && sourceType != domaintypes.JobTypeReGate {
			writeHTTPError(w, http.StatusBadRequest, "unsupported source job_type %q (supported: heal, re_gate)", sourceJob.JobType)
			return
		}
		if !isTerminalJobStatus(sourceJob.Status) {
			writeHTTPError(w, http.StatusConflict, "can only rerun terminal jobs")
			return
		}

		run, err := st.GetRun(r.Context(), sourceJob.RunID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "run not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get run: %v", err)
			return
		}
		if run.Status == domaintypes.RunStatusCancelled || run.Status == domaintypes.RunStatusFinished {
			if err := st.UpdateRunStatus(r.Context(), store.UpdateRunStatusParams{ID: run.ID, Status: domaintypes.RunStatusStarted}); err != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to reopen run: %v", err)
				return
			}
		}

		currentRunRepo, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: sourceJob.RunID, RepoID: sourceJob.RepoID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "run repo not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get run repo: %v", err)
			return
		}

		if err := st.IncrementRunRepoAttempt(r.Context(), store.IncrementRunRepoAttemptParams{RunID: sourceJob.RunID, RepoID: sourceJob.RepoID}); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to create new attempt: %v", err)
			return
		}

		runRepo, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: sourceJob.RunID, RepoID: sourceJob.RepoID})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to load new run repo attempt: %v", err)
			return
		}
		if runRepo.Attempt <= currentRunRepo.Attempt {
			writeHTTPError(w, http.StatusInternalServerError, "failed to increment attempt")
			return
		}

		rootMeta, _, err := buildRerunMeta(sourceJob.Meta, sourceJob.ID, alter)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to build rerun metadata: %v", err)
			return
		}

		rootJobID := domaintypes.NewJobID()
		var rootNext *domaintypes.JobID
		if sourceType == domaintypes.JobTypeHeal {
			reGateID := domaintypes.NewJobID()
			rootNext = &reGateID
			if err := createRerunReGateSuccessor(r.Context(), st, sourceJob, runRepo, reGateID); err != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to create re_gate successor: %v", err)
				return
			}
		}

		rootName := rerunRootJobName(sourceType)
		rootImage := strings.TrimSpace(sourceJob.JobImage)
		if alter.Image != "" {
			rootImage = alter.Image
		}
		_, err = st.CreateJob(r.Context(), store.CreateJobParams{
			ID:          rootJobID,
			RunID:       sourceJob.RunID,
			RepoID:      sourceJob.RepoID,
			RepoBaseRef: runRepo.RepoBaseRef,
			Attempt:     runRepo.Attempt,
			Name:        rootName,
			Status:      domaintypes.JobStatusQueued,
			JobType:     sourceType,
			JobImage:    rootImage,
			NextID:      rootNext,
			Meta:        rootMeta,
			RepoShaIn:   strings.TrimSpace(strings.ToLower(sourceJob.RepoShaIn)),
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to create rerun root job: %v", err)
			return
		}

		resp := rerunResponse{
			RunID:           sourceJob.RunID,
			RepoID:          sourceJob.RepoID,
			Attempt:         runRepo.Attempt,
			RootJobID:       rootJobID,
			CopiedFromJobID: sourceJob.ID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)

	}
}

func createRerunReGateSuccessor(ctx context.Context, st store.Store, sourceJob store.Job, runRepo store.RunRepo, reGateID domaintypes.JobID) error {
	reGateMeta := []byte(`{"kind":"gate"}`)
	if sourceJob.NextID != nil {
		nextJob, err := st.GetJob(ctx, *sourceJob.NextID)
		if err == nil && domaintypes.JobType(nextJob.JobType) == domaintypes.JobTypeReGate {
			reGateMeta = nextJob.Meta
		}
	}
	_, err := st.CreateJob(ctx, store.CreateJobParams{
		ID:          reGateID,
		RunID:       sourceJob.RunID,
		RepoID:      sourceJob.RepoID,
		RepoBaseRef: runRepo.RepoBaseRef,
		Attempt:     runRepo.Attempt,
		Name:        "re-gate-rerun-followup",
		Status:      domaintypes.JobStatusCreated,
		JobType:     domaintypes.JobTypeReGate,
		JobImage:    "",
		Meta:        reGateMeta,
	})
	if err != nil {
		return err
	}
	return nil
}

func rerunRootJobName(jobType domaintypes.JobType) string {
	switch jobType {
	case domaintypes.JobTypeHeal:
		return "heal-rerun-root"
	case domaintypes.JobTypeReGate:
		return "re-gate-rerun-root"
	default:
		return "rerun-root"
	}
}

func normalizeRerunAlter(raw map[string]any) (rerunAlter, error) {
	if raw == nil {
		return rerunAlter{}, fmt.Errorf("alter is required")
	}
	alter := rerunAlter{Envs: map[string]string{}}

	for key, value := range raw {
		switch key {
		case "image":
			str, ok := value.(string)
			if !ok {
				return rerunAlter{}, fmt.Errorf("image must be a string")
			}
			alter.Image = strings.TrimSpace(str)
		case "envs":
			envs, err := normalizeRerunAlterEnvs(value)
			if err != nil {
				return rerunAlter{}, err
			}
			alter.Envs = envs
		case "in":
			inEntries, err := normalizeRerunAlterIn(value)
			if err != nil {
				return rerunAlter{}, err
			}
			alter.In = inEntries
		default:
			return rerunAlter{}, fmt.Errorf("unsupported key %q", key)
		}
	}

	if alter.Image == "" && len(alter.Envs) == 0 && len(alter.In) == 0 {
		return rerunAlter{}, fmt.Errorf("at least one of image/envs/in must be provided")
	}
	return alter, nil
}

func normalizeRerunAlterEnvs(value any) (map[string]string, error) {
	raw, ok := value.(map[string]any)
	if !ok {
		if m, ok := value.(map[string]string); ok {
			copyMap := make(map[string]string, len(m))
			for k, v := range m {
				key := strings.TrimSpace(k)
				if key == "" {
					return nil, fmt.Errorf("envs key cannot be empty")
				}
				copyMap[key] = v
			}
			return copyMap, nil
		}
		return nil, fmt.Errorf("envs must be an object")
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		key := strings.TrimSpace(k)
		if key == "" {
			return nil, fmt.Errorf("envs key cannot be empty")
		}
		out[key] = strings.TrimSpace(fmt.Sprint(v))
	}
	return out, nil
}

func normalizeRerunAlterIn(value any) ([]string, error) {
	raw, ok := value.([]any)
	if !ok {
		if ss, ok := value.([]string); ok {
			out := make([]string, 0, len(ss))
			for _, s := range ss {
				t := strings.TrimSpace(s)
				if t == "" {
					continue
				}
				out = append(out, t)
			}
			return out, nil
		}
		return nil, fmt.Errorf("in must be an array of strings")
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("in must be an array of strings")
		}
		t := strings.TrimSpace(s)
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func buildRerunMeta(sourceMeta []byte, sourceJobID domaintypes.JobID, alter rerunAlter) ([]byte, string, error) {
	metaMap := map[string]any{}
	if len(strings.TrimSpace(string(sourceMeta))) > 0 {
		if err := json.Unmarshal(sourceMeta, &metaMap); err != nil {
			return nil, "", fmt.Errorf("parse source meta: %w", err)
		}
	}

	alterMap := map[string]any{}
	if alter.Image != "" {
		alterMap["image"] = alter.Image
	}
	if len(alter.Envs) > 0 {
		keys := make([]string, 0, len(alter.Envs))
		for k := range alter.Envs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		envs := make(map[string]any, len(keys))
		for _, k := range keys {
			envs[k] = alter.Envs[k]
		}
		alterMap["envs"] = envs
	}
	if len(alter.In) > 0 {
		entries := make([]any, 0, len(alter.In))
		for _, entry := range alter.In {
			entries = append(entries, entry)
		}
		alterMap["in"] = entries
	}

	alterRaw, err := json.Marshal(alterMap)
	if err != nil {
		return nil, "", fmt.Errorf("marshal alter: %w", err)
	}
	h := sha256.Sum256(alterRaw)
	digest := hex.EncodeToString(h[:12])

	metaMap[rerunMetaKey] = map[string]any{
		"source_job_id":   sourceJobID.String(),
		"mode":            rerunModeDriftOK,
		"alter_digest":    digest,
		rerunMetaAlterKey: alterMap,
	}

	encoded, err := json.Marshal(metaMap)
	if err != nil {
		return nil, "", fmt.Errorf("marshal rerun meta: %w", err)
	}
	return encoded, digest, nil
}
