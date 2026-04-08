package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/hook"
)

// NOTE: This file uses KSUID-backed string IDs for runs and jobs.
// Run and job IDs are generated using domaintypes.NewRunID() and domaintypes.NewJobID().
// UUID parsing is no longer performed for run/job IDs; they are treated as opaque strings.

// Migs run handlers implement the Migs-style run status surface (RunSummary)
// and job materialization helpers.

// getRunStatusHandler returns an HTTP handler that fetches run status by ID.
//
// Endpoint: GET /v1/runs/{id}/status
// Response: 200 OK with RunSummary body (canonical schema, no wrapper types)
//
// Canonical contract (see docs/migs-lifecycle.md § 2.1):
//   - Returns RunSummary directly as JSON root (no envelope or wrapper types).
//   - HTTP 200 on success; 404 if run not found.
//   - run_id is a KSUID string (27 characters).
//   - stages map is keyed by job ID (KSUID), not job name; use next_id links for ordering.
//
// Run and job IDs are KSUID-backed strings; no UUID parsing is performed.
func getRunStatusHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the run ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		runID, err := parseRequiredPathID[domaintypes.RunID](r, "id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Fetch run.
		run, err := st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "run not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to get run: %v", err)
			slog.Error("get run status: fetch run failed", "run_id", runID.String(), "err", err)
			return
		}

		// Build RunSummary response with Stages and Artifacts.
		runState, convErr := migsapi.RunStatusFromDomain(run.Status)
		if convErr != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to convert run status: %v", convErr)
			slog.Error("get run status: invalid run status", "run_id", run.ID, "status", run.Status, "err", convErr)
			return
		}

		var (
			repoURL    string
			repoBase   string
			repoTarget string
		)
		runRepos, err := st.ListRunReposWithURLByRun(r.Context(), run.ID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list run repos: %v", err)
			slog.Error("get run status: list run repos failed", "run_id", run.ID, "err", err)
			return
		}
		if len(runRepos) > 0 {
			rr := runRepos[0]
			repoBase = rr.RepoBaseRef
			repoTarget = rr.RepoTargetRef
			repoURL = rr.RepoUrl
		}

		summary := migsapi.RunSummary{
			RunID:      run.ID,
			State:      runState,
			Submitter:  "",
			Repository: repoURL,
			Metadata:   map[string]string{"repo_base_ref": repoBase, "repo_target_ref": repoTarget},
			CreatedAt:  timeOrZero(run.CreatedAt),
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[domaintypes.JobID]migsapi.StageStatus),
		}

		// Surface MR URL, gate summary, and resume metadata from runs.stats if present.
		// Node stores MR URL under stats.metadata.mr_url and gate data under stats.gate.
		// Gate summary exposes gate health without requiring raw artifact inspection.
		// Resume metadata (resume_count, last_resumed_at) tracks resume history.
		if len(run.Stats) > 0 && json.Valid(run.Stats) {
			var stats domaintypes.RunStats
			if err := json.Unmarshal(run.Stats, &stats); err == nil {
				if mr := stats.MRURL(); mr != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["mr_url"] = mr
				}
				// Extract gate summary for quick gate health visibility.
				if gateSummary := stats.GateSummary(); gateSummary != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["gate_summary"] = gateSummary
				}
				// Extract resume metadata so clients can see resume history.
				if rc := stats.ResumeCount(); rc > 0 {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["resume_count"] = strconv.Itoa(rc)
				}
				if lra := stats.LastResumedAt(); lra != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["last_resumed_at"] = lra
				}
			}
		}

		// Load jobs and their artifacts using string run ID.
		jobs, err := st.ListJobsByRun(r.Context(), run.ID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to list jobs: %v", err)
			slog.Error("get run status: list jobs failed", "run_id", run.ID, "err", err)
			return
		}
		for _, job := range jobs {
			jobIDStr := job.ID.String()
			s, convErr := migsapi.StageStatusFromDomain(job.Status)
			if convErr != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to convert stage status for job %s: %v", job.ID, convErr)
				slog.Error("get run status: invalid stage status", "run_id", run.ID, "job_id", job.ID, "status", job.Status, "err", convErr)
				return
			}
			artMap := make(map[string]string)
			bundles, err := st.ListArtifactBundlesByRunAndJob(r.Context(), store.ListArtifactBundlesByRunAndJobParams{
				RunID: run.ID,
				JobID: &job.ID,
			})
			if err != nil {
				writeHTTPError(w, http.StatusInternalServerError, "failed to list artifacts: %v", err)
				slog.Error("get run status: list artifacts failed", "run_id", run.ID, "job_id", jobIDStr, "err", err)
				return
			}
			for _, b := range bundles {
				name := "artifact"
				if b.Name != nil && strings.TrimSpace(*b.Name) != "" {
					name = strings.TrimSpace(*b.Name)
				}
				if b.Cid != nil && strings.TrimSpace(*b.Cid) != "" {
					artMap[name] = strings.TrimSpace(*b.Cid)
				}
			}

			// Attempts/MaxAttempts are currently fixed at 1; future retries must
			// update these counters without changing chain semantics.
			summary.Stages[job.ID] = migsapi.StageStatus{
				State:       s,
				Attempts:    1,
				MaxAttempts: 1,
				Artifacts:   artMap,
				NextID:      job.NextID,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Encode RunSummary directly — no wrapper type.
		if err := json.NewEncoder(w).Encode(summary); err != nil {
			slog.Error("get run status: encode response failed", "err", err)
		}
	}
}

type plannedJob struct {
	ID           domaintypes.JobID
	Name         string
	JobType      domaintypes.JobType
	JobImage     string
	Status       domaintypes.JobStatus
	StepName     string
	HookSource   string
	HookDecision *hookPlanningDecision
	NextID       *domaintypes.JobID
	RepoSHAIn    string
}

type hookPlanningDecision struct {
	Evaluated bool
	Match     hook.MatchDecision
}

func (d hookPlanningDecision) ShouldRun() bool {
	return d.Match.ShouldRun
}

type plannedHookSource struct {
	SourceIndex int
	Source      string
	Decision    hookPlanningDecision
}

type preGateCreationBinding struct {
	ProfileID int64
	JobImage  string
}

// createJobsFromSpec parses the run spec and creates an explicit next_id-linked job chain.
// Queue semantics are head-only: the first job is Queued, all successors are Created.
func createJobsFromSpec(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	repoID domaintypes.RepoID,
	repoBaseRef string,
	attempt int32,
	repoSHA0 string,
	spec []byte,
	hookBlobstores ...blobstore.Store,
) error {
	migsSpec, err := contracts.ParseMigSpecJSON(spec)
	if err != nil {
		return fmt.Errorf("parse migs spec: %w", err)
	}
	repoSHA0 = strings.TrimSpace(strings.ToLower(repoSHA0))
	if !sha40Pattern.MatchString(repoSHA0) {
		return fmt.Errorf("repo_sha0 must match ^[0-9a-f]{40}$")
	}
	preGateBinding, err := resolvePreGateCreationBindingFromStore(ctx, st, repoID, repoSHA0, migsSpec)
	if err != nil {
		return fmt.Errorf("resolve pre-gate binding: %w", err)
	}
	resolvedHooks, err := resolveHookManifestSources(*migsSpec)
	if err != nil {
		return fmt.Errorf("resolve hook sources: %w", err)
	}
	preGateHookPlans, err := resolveCycleHookPlans(ctx, st, runID, repoID, attempt, migsSpec, resolvedHooks, "pre-gate", hookBlobstores...)
	if err != nil {
		return fmt.Errorf("plan pre-gate hooks: %w", err)
	}
	postGateHookPlans, err := resolveCycleHookPlans(ctx, st, runID, repoID, attempt, migsSpec, resolvedHooks, "post-gate", hookBlobstores...)
	if err != nil {
		return fmt.Errorf("plan post-gate hooks: %w", err)
	}

	type draft struct {
		name         string
		jobType      domaintypes.JobType
		jobImage     string
		stepName     string
		hookSource   string
		hookDecision *hookPlanningDecision
	}
	appendGatePreludeDrafts := func(drafts []draft, cycleName string, hookPlans []plannedHookSource) []draft {
		drafts = append(drafts, draft{name: cycleName + "-sbom", jobType: domaintypes.JobTypeSBOM})
		for _, hookPlan := range hookPlans {
			if !hookPlan.Decision.ShouldRun() {
				continue
			}
			decision := hookPlan.Decision
			drafts = append(drafts, draft{
				name:         fmt.Sprintf("%s-hook-%03d", cycleName, hookPlan.SourceIndex),
				jobType:      domaintypes.JobTypeHook,
				hookSource:   hookPlan.Source,
				hookDecision: &decision,
			})
		}
		return drafts
	}

	drafts := make([]draft, 0, len(migsSpec.Steps)+len(resolvedHooks)*2+5)
	drafts = appendGatePreludeDrafts(drafts, "pre-gate", preGateHookPlans)
	drafts = append(drafts, draft{name: "pre-gate", jobType: domaintypes.JobTypePreGate})

	if len(migsSpec.Steps) > 1 {
		for i, mig := range migsSpec.Steps {
			jobImage := ""
			if mig.Image.Universal != "" {
				jobImage = strings.TrimSpace(mig.Image.Universal)
			}
			drafts = append(drafts, draft{
				name:     fmt.Sprintf("mig-%d", i),
				jobType:  domaintypes.JobTypeMig,
				jobImage: jobImage,
				stepName: mig.Name,
			})
		}
	} else {
		migImage := ""
		stepName := ""
		if len(migsSpec.Steps) > 0 {
			if migsSpec.Steps[0].Image.Universal != "" {
				migImage = strings.TrimSpace(migsSpec.Steps[0].Image.Universal)
			}
			stepName = migsSpec.Steps[0].Name
		}
		drafts = append(drafts, draft{
			name:     "mig-0",
			jobType:  domaintypes.JobTypeMig,
			jobImage: migImage,
			stepName: stepName,
		})
	}
	drafts = appendGatePreludeDrafts(drafts, "post-gate", postGateHookPlans)
	drafts = append(drafts, draft{name: "post-gate", jobType: domaintypes.JobTypePostGate})

	planned := make([]plannedJob, 0, len(drafts))
	for i, d := range drafts {
		status := domaintypes.JobStatusCreated
		if i == 0 {
			status = domaintypes.JobStatusQueued
		}
		jobImage := d.jobImage
		if d.jobType == domaintypes.JobTypePreGate && preGateBinding != nil && strings.TrimSpace(preGateBinding.JobImage) != "" {
			jobImage = strings.TrimSpace(preGateBinding.JobImage)
		}
		planned = append(planned, plannedJob{
			ID:           domaintypes.NewJobID(),
			Name:         d.name,
			JobType:      d.jobType,
			JobImage:     jobImage,
			Status:       status,
			StepName:     d.stepName,
			HookSource:   d.hookSource,
			HookDecision: d.hookDecision,
		})
	}
	// Seed deterministic SHA chain from run_repos.repo_sha0 at chain head.
	planned[0].RepoSHAIn = repoSHA0
	for i := range planned {
		if i+1 < len(planned) {
			nextID := planned[i+1].ID
			planned[i].NextID = &nextID
		}
	}

	// Insert chain tail-first to satisfy jobs.next_id -> jobs.id FK at insert time.
	for i := len(planned) - 1; i >= 0; i-- {
		if err := createPlannedJob(ctx, st, runID, repoID, repoBaseRef, attempt, planned[i]); err != nil {
			return fmt.Errorf("create job %q: %w", planned[i].Name, err)
		}
	}
	if preGateBinding != nil && preGateBinding.ProfileID > 0 {
		preGateJobID, ok := findPlannedJobIDByType(planned, domaintypes.JobTypePreGate)
		if !ok {
			return fmt.Errorf("pre-gate job missing from planned chain")
		}
		if err := upsertPreGateCreationProfileLink(ctx, st, preGateJobID, preGateBinding.ProfileID); err != nil {
			return fmt.Errorf("link pre-gate profile: %w", err)
		}
	}
	return nil
}

func findPlannedJobIDByType(planned []plannedJob, jobType domaintypes.JobType) (domaintypes.JobID, bool) {
	for _, p := range planned {
		if p.JobType == jobType {
			return p.ID, true
		}
	}
	return "", false
}

func resolvePreGateCreationBindingFromStore(
	ctx context.Context,
	st store.Store,
	repoID domaintypes.RepoID,
	repoSHA string,
	spec *contracts.MigSpec,
) (*preGateCreationBinding, error) {
	pgStore, ok := st.(*store.PgStore)
	if !ok || pgStore == nil {
		return nil, nil
	}

	repoIDText := repoID.String()
	repoSHAText := repoSHA
	lang, tool, release := preGateStackHintsFromSpec(spec)
	if lang != "" || tool != "" || release != "" {
		row, err := pgStore.ResolvePreGateCreationBindingByRepoSHAAndStack(ctx, store.ResolvePreGateCreationBindingByRepoSHAAndStackParams{
			RepoID:  repoIDText,
			RepoSha: repoSHAText,
			Lang:    lang,
			Tool:    tool,
			Release: release,
		})
		if err == nil {
			return &preGateCreationBinding{
				ProfileID: row.ProfileID,
				JobImage:  row.JobImage,
			}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}

	row, err := pgStore.ResolvePreGateCreationBindingByRepoSHA(ctx, store.ResolvePreGateCreationBindingByRepoSHAParams{
		RepoID:  repoIDText,
		RepoSha: repoSHAText,
	})
	if err == nil {
		return &preGateCreationBinding{
			ProfileID: row.ProfileID,
			JobImage:  row.JobImage,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return nil, nil
}

func preGateStackHintsFromSpec(spec *contracts.MigSpec) (lang, tool, release string) {
	if spec == nil || spec.BuildGate == nil || spec.BuildGate.Pre == nil || spec.BuildGate.Pre.Stack == nil {
		return "", "", ""
	}
	stack := spec.BuildGate.Pre.Stack
	if !stack.Enabled {
		return "", "", ""
	}
	return strings.TrimSpace(stack.Language), strings.TrimSpace(stack.Tool), strings.TrimSpace(stack.Release)
}

func upsertPreGateCreationProfileLink(ctx context.Context, st store.Store, jobID domaintypes.JobID, profileID int64) error {
	pgStore, ok := st.(*store.PgStore)
	if !ok || pgStore == nil || profileID <= 0 {
		return nil
	}
	return pgStore.UpsertGateJobProfileLink(ctx, store.UpsertGateJobProfileLinkParams{
		JobID:     jobID.String(),
		ProfileID: profileID,
	})
}

func createPlannedJob(ctx context.Context, st store.Store, runID domaintypes.RunID, repoID domaintypes.RepoID, repoBaseRef string, attempt int32, planned plannedJob) error {
	// Build job metadata with step name for mig jobs.
	var meta *contracts.JobMeta
	if planned.StepName != "" {
		meta = contracts.NewMigJobMetaWithStepName(planned.StepName)
	} else {
		meta = contracts.NewMigJobMeta()
	}
	meta.HookSource = strings.TrimSpace(planned.HookSource)
	if planned.HookDecision != nil {
		meta.ActionSummary = summarizeHookPlanningDecision(*planned.HookDecision)
	}
	metaBytes, err := contracts.MarshalJobMeta(meta)
	if err != nil {
		return fmt.Errorf("marshal job meta: %w", err)
	}

	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:          planned.ID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: repoBaseRef,
		Attempt:     attempt,
		Name:        planned.Name,
		Status:      planned.Status,
		JobType:     planned.JobType,
		JobImage:    planned.JobImage,
		NextID:      planned.NextID,
		Meta:        metaBytes,
		RepoShaIn:   planned.RepoSHAIn,
	})
	return err
}

func resolveCycleHookPlans(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	repoID domaintypes.RepoID,
	attempt int32,
	spec *contracts.MigSpec,
	resolvedHooks []string,
	cycleName string,
	hookBlobstores ...blobstore.Store,
) ([]plannedHookSource, error) {
	if len(resolvedHooks) == 0 {
		return nil, nil
	}
	matchInput, err := buildCycleHookMatchInput(ctx, st, runID, repoID, attempt, spec, cycleName)
	if err != nil {
		return nil, err
	}
	out := make([]plannedHookSource, 0, len(resolvedHooks))
	for i, source := range resolvedHooks {
		decision, decisionErr := resolvePlannableHookDecision(ctx, st, spec, source, matchInput, hookBlobstores...)
		if decisionErr != nil {
			return nil, fmt.Errorf("source[%d] %q: %w", i, source, decisionErr)
		}
		out = append(out, plannedHookSource{
			SourceIndex: i,
			Source:      source,
			Decision:    decision,
		})
	}
	return out, nil
}

func buildCycleHookMatchInput(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	repoID domaintypes.RepoID,
	attempt int32,
	spec *contracts.MigSpec,
	cycleName string,
) (hook.MatchInput, error) {
	input, err := buildHookMatchInput(ctx, st, store.Job{
		RunID:   runID,
		RepoID:  repoID,
		Attempt: attempt,
	})
	if err != nil {
		return hook.MatchInput{}, err
	}
	input.Stack = mergeHookRuntimeStackWithFallback(input.Stack, hookRuntimeFallbackStack(spec, cycleName))
	return input, nil
}

func hookRuntimeFallbackStack(spec *contracts.MigSpec, cycleName string) hook.RuntimeStack {
	if spec == nil || spec.BuildGate == nil {
		return hook.RuntimeStack{}
	}
	var phase *contracts.BuildGatePhaseConfig
	switch cycleName {
	case "pre-gate":
		phase = spec.BuildGate.Pre
	case "post-gate", "re-gate":
		phase = spec.BuildGate.Post
	}
	if phase == nil || phase.Stack == nil || !phase.Stack.Enabled {
		return hook.RuntimeStack{}
	}
	return hook.RuntimeStack{
		Language: strings.TrimSpace(phase.Stack.Language),
		Tool:     strings.TrimSpace(phase.Stack.Tool),
		Release:  strings.TrimSpace(phase.Stack.Release),
	}
}

func mergeHookRuntimeStackWithFallback(current hook.RuntimeStack, fallback hook.RuntimeStack) hook.RuntimeStack {
	if strings.TrimSpace(current.Language) == "" {
		current.Language = fallback.Language
	}
	if strings.TrimSpace(current.Tool) == "" {
		current.Tool = fallback.Tool
	}
	if strings.TrimSpace(current.Release) == "" {
		current.Release = fallback.Release
	}
	return current
}

func resolvePlannableHookDecision(
	ctx context.Context,
	st store.Store,
	spec *contracts.MigSpec,
	source string,
	matchInput hook.MatchInput,
	hookBlobstores ...blobstore.Store,
) (hookPlanningDecision, error) {
	specDoc, err := loadHookSpecForPlanning(ctx, st, spec, source, hookBlobstores...)
	if err != nil {
		return hookPlanningDecision{}, err
	}
	match, err := hook.Match(specDoc, matchInput)
	if err != nil {
		return hookPlanningDecision{}, fmt.Errorf("evaluate hook matcher: %w", err)
	}
	return hookPlanningDecision{
		Evaluated: true,
		Match:     match,
	}, nil
}

func loadHookSpecForPlanning(
	ctx context.Context,
	st store.Store,
	spec *contracts.MigSpec,
	source string,
	hookBlobstores ...blobstore.Store,
) (hook.Spec, error) {
	trimmed := strings.TrimSpace(source)
	if isHTTPSHookSource(trimmed) {
		specDoc, err := loadRuntimeHookSpecFromLoader(trimmed, ".")
		if err != nil {
			return hook.Spec{}, fmt.Errorf("load hook spec: %w", err)
		}
		return specDoc, nil
	}
	if canonicalHookSourcePattern.MatchString(trimmed) {
		specDoc, err := loadHookSpecFromBundleHash(ctx, st, firstHookBlobStore(hookBlobstores...), trimmed, planningBundleMap(spec))
		if err != nil {
			return hook.Spec{}, fmt.Errorf("load hook spec from hash: %w", err)
		}
		return specDoc, nil
	}
	return hook.Spec{}, fmt.Errorf("unsupported hook source %q: local hook sources must be precompiled by CLI into hash entries", source)
}

func firstHookBlobStore(hookBlobstores ...blobstore.Store) blobstore.Store {
	if len(hookBlobstores) == 0 {
		return nil
	}
	return hookBlobstores[0]
}

func planningBundleMap(spec *contracts.MigSpec) map[string]string {
	if spec == nil {
		return nil
	}
	return spec.BundleMap
}

func summarizeHookPlanningDecision(decision hookPlanningDecision) string {
	return fmt.Sprintf(
		"hook_match eval=planned should_run=%t stack=%t sbom=%t on_match=%t on_add=%t on_remove=%t on_change=%t",
		decision.Match.ShouldRun,
		decision.Match.StackMatched,
		decision.Match.SBOMMatched,
		decision.Match.Predicates.OnMatch,
		decision.Match.Predicates.OnAdd,
		decision.Match.Predicates.OnRemove,
		decision.Match.Predicates.OnChange,
	)
}

func resolveHookManifestSources(spec contracts.MigSpec) ([]string, error) {
	if len(spec.Hooks) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(spec.Hooks))
	for i, raw := range spec.Hooks {
		source := strings.TrimSpace(raw)
		if source == "" {
			return nil, fmt.Errorf("hooks[%d]: empty hook source", i)
		}
		if !canonicalHookSourcePattern.MatchString(source) && !isHTTPSHookSource(source) {
			return nil, fmt.Errorf("hooks[%d] %q: local hook sources must be precompiled by CLI into hash entries", i, raw)
		}
		out = append(out, source)
	}
	return out, nil
}

var canonicalHookSourcePattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

func isHTTPSHookSource(source string) bool {
	u, err := url.Parse(source)
	if err != nil || u == nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// helpers
func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Unix(0, 0).UTC()
}
