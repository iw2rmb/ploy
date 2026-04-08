package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/jobchain"
)

// listRunRepoJobsHandler returns jobs for a specific repo execution within a run.
// GET /v1/runs/{run_id}/repos/{repo_id}/jobs
// Query params: ?attempt=N (optional, defaults to current attempt)
func listRunRepoJobsHandler(st store.Store) http.HandlerFunc {
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

		rr, err := st.GetRunRepo(r.Context(), store.GetRunRepoParams{RunID: runID, RepoID: repoID})
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				writeHTTPError(w, http.StatusNotFound, "repo not found")
			default:
				writeHTTPError(w, http.StatusInternalServerError, "failed to get repo: %v", err)
				slog.Error("list run repo jobs: get repo failed", "run_id", runID.String(), "repo_id", repoID.String(), "err", err)
			}
			return
		}

		// Use attempt from query param if provided, otherwise use current attempt.
		attempt := rr.Attempt
		if q := r.URL.Query().Get("attempt"); q != "" {
			parsed, err := strconv.ParseInt(q, 10, 32)
			if err != nil {
				writeHTTPError(w, http.StatusBadRequest, "invalid attempt parameter")
				return
			}
			attempt = int32(parsed)
		}

		jobs, err := st.ListJobsByRunRepoAttempt(r.Context(), store.ListJobsByRunRepoAttemptParams{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: attempt,
		})
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list jobs: %v", err)
			slog.Error("list run repo jobs: list jobs failed", "run_id", runID.String(), "repo_id", repoID.String(), "attempt", attempt, "err", err)
			return
		}
		jobs = jobchain.Order(
			jobs,
			func(job store.Job) domaintypes.JobID { return job.ID },
			func(job store.Job) *domaintypes.JobID { return job.NextID },
		)

		resp := migsapi.ListRunRepoJobsResponse{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: attempt,
			Jobs:    make([]migsapi.RunRepoJob, 0, len(jobs)),
		}
		hookJobsByCycle := make(map[string]int)
		hookConditionsByCycle := make(map[string][]json.RawMessage)
		sbomCycleByJobID := make(map[domaintypes.JobID]string)

		for _, job := range jobs {
			jr := migsapi.RunRepoJob{
				JobID:      job.ID,
				Name:       job.Name,
				JobType:    job.JobType,
				JobImage:   strings.TrimSpace(job.JobImage),
				RepoShaIn:  job.RepoShaIn,
				RepoShaOut: job.RepoShaOut,
				NextID:     job.NextID,
				NodeID:     job.NodeID,
				Status:     job.Status,
				ExitCode:   job.ExitCode,
				DurationMs: job.DurationMs,
			}

			jr.HookConditionResult, jr.HookPlanReason = extractRawHookEvidence(job.Meta)

			// Extract projection fields from structured job metadata.
			if len(job.Meta) > 0 {
				meta, err := contracts.UnmarshalJobMeta(job.Meta)
				if err == nil {
					if meta.MigStepName != "" {
						jr.DisplayName = meta.MigStepName
					}
					if meta.Heal != nil {
						jr.BugSummary = strings.TrimSpace(meta.Heal.BugSummary)
						jr.ActionSummary = strings.TrimSpace(meta.Heal.ActionSummary)
						jr.ErrorKind = strings.TrimSpace(meta.Heal.ErrorKind)
					}
					if jr.ActionSummary == "" && meta.ActionSummary != "" {
						jr.ActionSummary = strings.TrimSpace(meta.ActionSummary)
					}
					if jr.BugSummary == "" && meta.GateMetadata != nil && strings.TrimSpace(meta.GateMetadata.BugSummary) != "" {
						jr.BugSummary = strings.TrimSpace(meta.GateMetadata.BugSummary)
					}
					if meta.GateMetadata != nil && meta.GateMetadata.StackGate != nil {
						if runtimeImage := strings.TrimSpace(meta.GateMetadata.StackGate.RuntimeImage); runtimeImage != "" {
							// Prefer the runtime-resolved gate image when available.
							jr.JobImage = runtimeImage
						}
					}
					if meta.RecoveryMetadata != nil {
						jr.Recovery = newRecoveryView(meta.RecoveryMetadata)
					} else if meta.GateMetadata != nil && meta.GateMetadata.Recovery != nil {
						jr.Recovery = newRecoveryView(meta.GateMetadata.Recovery)
					}
					if meta.GateMetadata != nil {
						if exp := meta.GateMetadata.DetectedStackExpectation(); exp != nil {
							jr.Lang = exp.Language
							jr.Tooling = exp.Tool
							jr.Version = exp.Release
						}
					}
					if job.JobType == domaintypes.JobTypeHook {
						actionSummary := strings.TrimSpace(meta.ActionSummary)
						if actionSummary != "" {
							if jr.HookPlanReason == "" {
								jr.HookPlanReason = actionSummary
							}
							if jr.HookConditionResult == "" {
								if conditionJSON, ok := parseHookConditionResultFromSummary(actionSummary); ok {
									jr.HookConditionResult = conditionJSON
								}
							}
						}
					}
				}
			}

			if job.JobType == domaintypes.JobTypeSBOM {
				jr.Name = "sbom"
				if sbomCtx, ok := sbomCycleContextFromJob(job); ok {
					sbomCycleByJobID[job.ID] = sbomCycleNameFromContext(sbomCtx)
				}
				jr.SBOMEvidence = loadSBOMEvidence(r, st, runID, job.ID)
			}

			if job.JobType == domaintypes.JobTypeHook {
				if cycleName, ok := cycleNameFromHookJobName(job.Name); ok {
					hookJobsByCycle[cycleName]++
					if raw := parseSerializedJSON(jr.HookConditionResult); len(raw) > 0 {
						hookConditionsByCycle[cycleName] = append(hookConditionsByCycle[cycleName], raw)
					}
				}
			}

			// Set timestamps.
			if job.StartedAt.Valid {
				t := job.StartedAt.Time.UTC()
				jr.StartedAt = &t
			}
			if job.FinishedAt.Valid {
				t := job.FinishedAt.Time.UTC()
				jr.FinishedAt = &t
			}

			resp.Jobs = append(resp.Jobs, jr)
		}
		attachSBOMHookPlanningEvidence(resp.Jobs, hookJobsByCycle, hookConditionsByCycle, sbomCycleByJobID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list run repo jobs: encode response failed", "err", err)
		}
	}
}

func loadSBOMEvidence(r *http.Request, st store.Store, runID domaintypes.RunID, jobID domaintypes.JobID) *migsapi.RunRepoJobSBOMEvidence {
	var evidence migsapi.RunRepoJobSBOMEvidence
	hasEvidence := false

	bundles, err := st.ListArtifactBundlesByRunAndJob(r.Context(), store.ListArtifactBundlesByRunAndJobParams{
		RunID: runID,
		JobID: &jobID,
	})
	if err != nil {
		slog.Warn("list run repo jobs: load sbom artifact evidence failed", "run_id", runID.String(), "job_id", jobID.String(), "err", err)
	} else {
		artifactPresent := len(bundles) > 0
		evidence.ArtifactPresent = &artifactPresent
		hasEvidence = true
	}

	sbomRows, err := st.ListSBOMRowsByJob(r.Context(), jobID)
	if err != nil {
		slog.Warn("list run repo jobs: load sbom package-count evidence failed", "run_id", runID.String(), "job_id", jobID.String(), "err", err)
	} else {
		parsedPackageCount := len(sbomRows)
		evidence.ParsedPackageCount = &parsedPackageCount
		hasEvidence = true
	}

	if !hasEvidence {
		return nil
	}
	return &evidence
}

func attachSBOMHookPlanningEvidence(
	jobs []migsapi.RunRepoJob,
	hookJobsByCycle map[string]int,
	hookConditionsByCycle map[string][]json.RawMessage,
	sbomCycleByJobID map[domaintypes.JobID]string,
) {
	for i := range jobs {
		job := &jobs[i]
		if job.JobType != domaintypes.JobTypeSBOM || job.Status != domaintypes.JobStatusSuccess {
			continue
		}
		cycleName := strings.TrimSpace(sbomCycleByJobID[job.JobID])
		if cycleName == "" {
			continue
		}
		plannedHookJobs := hookJobsByCycle[cycleName]
		if job.HookPlanReason == "" {
			if plannedHookJobs == 0 {
				job.HookPlanReason = fmt.Sprintf("no hook jobs planned for cycle %q", cycleName)
			} else {
				job.HookPlanReason = fmt.Sprintf("planned %d hook job(s) for cycle %q", plannedHookJobs, cycleName)
			}
		}
		if job.HookConditionResult != "" {
			continue
		}
		payload := struct {
			Evaluated   bool              `json:"evaluated"`
			PlannedJobs int               `json:"planned_jobs"`
			Hooks       []json.RawMessage `json:"hooks,omitempty"`
		}{
			Evaluated:   true,
			PlannedJobs: plannedHookJobs,
			Hooks:       hookConditionsByCycle[cycleName],
		}
		if raw, err := json.Marshal(payload); err == nil {
			job.HookConditionResult = string(raw)
		}
	}
}

func extractRawHookEvidence(metaRaw []byte) (hookConditionResult string, hookPlanReason string) {
	var raw map[string]json.RawMessage
	if len(metaRaw) == 0 || json.Unmarshal(metaRaw, &raw) != nil {
		return "", ""
	}
	return decodeSerializedField(raw["hook_condition_result"]), decodeSerializedField(raw["hook_plan_reason"])
}

func decodeSerializedField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return strings.TrimSpace(plain)
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if raw := parseSerializedJSON(s); len(raw) > 0 {
		return string(raw)
	}
	return s
}

func parseHookConditionResultFromSummary(summary string) (string, bool) {
	summary = strings.TrimSpace(summary)
	if !strings.HasPrefix(summary, "hook_match ") {
		return "", false
	}
	fields := strings.Fields(summary)
	kv := make(map[string]string, len(fields))
	for _, f := range fields[1:] {
		key, value, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		kv[key] = value
	}
	shouldRun, err := strconv.ParseBool(kv["should_run"])
	if err != nil {
		return "", false
	}
	stackMatched, err := strconv.ParseBool(kv["stack"])
	if err != nil {
		return "", false
	}
	sbomMatched, err := strconv.ParseBool(kv["sbom"])
	if err != nil {
		return "", false
	}
	onMatch, err := strconv.ParseBool(kv["on_match"])
	if err != nil {
		return "", false
	}
	onAdd, err := strconv.ParseBool(kv["on_add"])
	if err != nil {
		return "", false
	}
	onRemove, err := strconv.ParseBool(kv["on_remove"])
	if err != nil {
		return "", false
	}
	onChange, err := strconv.ParseBool(kv["on_change"])
	if err != nil {
		return "", false
	}
	payload := struct {
		Evaluated    bool `json:"evaluated"`
		ShouldRun    bool `json:"should_run"`
		StackMatched bool `json:"stack_matched"`
		SBOMMatched  bool `json:"sbom_matched"`
		Predicates   struct {
			OnMatch  bool `json:"on_match"`
			OnAdd    bool `json:"on_add"`
			OnRemove bool `json:"on_remove"`
			OnChange bool `json:"on_change"`
		} `json:"predicates"`
	}{
		Evaluated:    kv["eval"] == "planned",
		ShouldRun:    shouldRun,
		StackMatched: stackMatched,
		SBOMMatched:  sbomMatched,
	}
	payload.Predicates.OnMatch = onMatch
	payload.Predicates.OnAdd = onAdd
	payload.Predicates.OnRemove = onRemove
	payload.Predicates.OnChange = onChange
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func parseSerializedJSON(raw string) json.RawMessage {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(trimmed)); err != nil {
		return nil
	}
	return json.RawMessage(compact.String())
}

func cycleNameFromHookJobName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	idx := strings.LastIndex(name, "-hook-")
	if idx <= 0 {
		return "", false
	}
	cycleName := strings.TrimSpace(name[:idx])
	if cycleName == "" {
		return "", false
	}
	return cycleName, true
}

func newRecoveryView(meta *contracts.BuildGateRecoveryMetadata) *migsapi.RunRepoJobRecovery {
	if meta == nil {
		return nil
	}
	return &migsapi.RunRepoJobRecovery{
		LoopKind:                  meta.LoopKind,
		StrategyID:                meta.StrategyID,
		Confidence:                meta.Confidence,
		Reason:                    meta.Reason,
		Expectations:              meta.Expectations,
		CandidateSchemaID:         meta.CandidateSchemaID,
		CandidateArtifactPath:     meta.CandidateArtifactPath,
		CandidateValidationStatus: meta.CandidateValidationStatus,
		CandidateValidationError:  meta.CandidateValidationError,
		CandidatePromoted:         meta.CandidatePromoted,
	}
}
