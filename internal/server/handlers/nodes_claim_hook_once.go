package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/hook"
)

func resolveHookRuntimeDecision(
	ctx context.Context,
	st store.Store,
	job store.Job,
	mergedSpec json.RawMessage,
	jobType domaintypes.JobType,
) (*contracts.HookRuntimeDecision, error) {
	if jobType != domaintypes.JobTypeHook {
		return nil, nil
	}
	migSpec, err := contracts.ParseMigSpecJSON(mergedSpec)
	if err != nil {
		return nil, fmt.Errorf("parse merged spec for hook runtime: %w", err)
	}
	source := hookSourceFromJobMeta(job.Meta)
	hookIndex := -1
	if source == "" {
		var err error
		hookIndex, err = hookIndexFromJobName(job.Name, len(migSpec.Hooks))
		if err != nil {
			return nil, err
		}
	}
	if source == "" {
		source = strings.TrimSpace(migSpec.Hooks[hookIndex])
	}
	if source == "" {
		return nil, fmt.Errorf("hook source is empty for index %d", hookIndex)
	}
	hookSpec, err := loadRuntimeHookSpec(source, ".")
	if err != nil {
		return nil, fmt.Errorf("load hook spec for source %q: %w", source, err)
	}
	matchInput, err := buildHookMatchInput(ctx, st, job)
	if err != nil {
		return nil, err
	}
	match, err := hook.Match(hookSpec, matchInput)
	if err != nil {
		return nil, fmt.Errorf("evaluate hook matcher for source %q: %w", source, err)
	}
	hash := strings.TrimSpace(match.Once.PersistenceKey)
	if hash == "" {
		hash = strings.TrimSpace(match.HookHash)
	}
	if hash == "" {
		return nil, fmt.Errorf("hook matcher returned empty hash for source %q", source)
	}

	decision := &contracts.HookRuntimeDecision{
		HookHash:      hash,
		HookShouldRun: match.ShouldRun,
	}
	if !match.Once.Enabled || !match.Once.Eligible {
		return decision, nil
	}
	return applyHookOnceLedgerDecision(ctx, st, job, decision)
}

func buildHookMatchInput(ctx context.Context, st store.Store, job store.Job) (hook.MatchInput, error) {
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   job.RunID,
		RepoID:  job.RepoID,
		Attempt: job.Attempt,
	})
	if err != nil {
		return hook.MatchInput{}, fmt.Errorf("list jobs for hook runtime input: %w", err)
	}

	current, previous, err := resolveHookSBOMSnapshots(ctx, st, jobs)
	if err != nil {
		return hook.MatchInput{}, err
	}

	return hook.MatchInput{
		Stack:        resolveHookRuntimeStack(jobs),
		CurrentSBOM:  current,
		PreviousSBOM: previous,
	}, nil
}

func resolveHookRuntimeStack(jobs []store.Job) hook.RuntimeStack {
	var (
		selected store.Job
		found    bool
		stack    hook.RuntimeStack
	)
	for _, candidate := range jobs {
		if !isGateJobTypeForClaim(domaintypes.JobType(candidate.JobType)) || candidate.Status != domaintypes.JobStatusSuccess || len(candidate.Meta) == 0 {
			continue
		}
		meta, err := contracts.UnmarshalJobMeta(candidate.Meta)
		if err != nil || meta.GateMetadata == nil {
			continue
		}
		exp := meta.GateMetadata.DetectedStackExpectation()
		if exp == nil {
			continue
		}
		if !found || sbomJobIsMoreRecent(candidate, selected) {
			selected = candidate
			stack = hook.RuntimeStack{
				Language: strings.TrimSpace(exp.Language),
				Tool:     strings.TrimSpace(exp.Tool),
				Release:  strings.TrimSpace(exp.Release),
			}
			found = true
		}
	}
	return stack
}

func resolveHookSBOMSnapshots(
	ctx context.Context,
	st store.Store,
	jobs []store.Job,
) ([]hook.SBOMPackage, []hook.SBOMPackage, error) {
	latest, previous := latestTwoSuccessfulSBOMJobs(jobs)
	if latest == nil {
		return nil, nil, nil
	}

	currentRows, err := st.ListSBOMRowsByJob(ctx, latest.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("list current sbom rows for job %s: %w", latest.ID, err)
	}
	current := toHookSBOMPackages(currentRows)

	if previous == nil {
		return current, nil, nil
	}
	previousRows, err := st.ListSBOMRowsByJob(ctx, previous.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("list previous sbom rows for job %s: %w", previous.ID, err)
	}
	return current, toHookSBOMPackages(previousRows), nil
}

func latestTwoSuccessfulSBOMJobs(jobs []store.Job) (*store.Job, *store.Job) {
	var latest *store.Job
	var previous *store.Job
	for i := range jobs {
		if jobs[i].JobType != domaintypes.JobTypeSBOM || jobs[i].Status != domaintypes.JobStatusSuccess {
			continue
		}
		candidate := &jobs[i]
		if latest == nil || sbomJobIsMoreRecent(*candidate, *latest) {
			previous = latest
			latest = candidate
			continue
		}
		if previous == nil || sbomJobIsMoreRecent(*candidate, *previous) {
			previous = candidate
		}
	}
	return latest, previous
}

func toHookSBOMPackages(rows []store.Sbom) []hook.SBOMPackage {
	if len(rows) == 0 {
		return nil
	}
	out := make([]hook.SBOMPackage, 0, len(rows))
	for _, row := range rows {
		out = append(out, hook.SBOMPackage{
			Name:    strings.TrimSpace(row.Lib),
			Version: strings.TrimSpace(row.Ver),
		})
	}
	return out
}

func applyHookOnceLedgerDecision(
	ctx context.Context,
	st store.Store,
	job store.Job,
	decision *contracts.HookRuntimeDecision,
) (*contracts.HookRuntimeDecision, error) {
	exists, err := st.HasHookOnceLedger(ctx, store.HasHookOnceLedgerParams{
		RunID:    job.RunID,
		RepoID:   job.RepoID,
		HookHash: decision.HookHash,
	})
	if err != nil {
		return nil, fmt.Errorf("check hook once ledger: %w", err)
	}
	if !exists {
		return decision, nil
	}

	ledger, err := st.GetHookOnceLedger(ctx, store.GetHookOnceLedgerParams{
		RunID:    job.RunID,
		RepoID:   job.RepoID,
		HookHash: decision.HookHash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return decision, nil
		}
		return nil, fmt.Errorf("get hook once ledger: %w", err)
	}

	// Skip only after a successful execution for this run/repo/hash exists.
	if ledger.FirstSuccessJobID == nil {
		return decision, nil
	}
	decision.HookShouldRun = false
	decision.HookOnceSkipMarked = !ledger.OnceSkipMarked
	return decision, nil
}

func hookIndexFromJobName(jobName string, hooksLen int) (int, error) {
	name := strings.TrimSpace(jobName)
	if hooksLen <= 0 {
		return 0, fmt.Errorf("hook job requires at least one declared hook source")
	}
	idx := strings.LastIndex(name, "-hook-")
	if idx <= 0 {
		return 0, fmt.Errorf("hook job_name must contain %q, got %q", "-hook-", name)
	}
	raw := strings.TrimSpace(name[idx+len("-hook-"):])
	hookIndex, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse hook index from job_name %q: %w", name, err)
	}
	if hookIndex < 0 || hookIndex >= hooksLen {
		return 0, fmt.Errorf("hook index out of range for job_name %q: idx=%d hooks_len=%d", name, hookIndex, hooksLen)
	}
	return hookIndex, nil
}

func loadRuntimeHookSpec(source string, specRoot string) (hook.Spec, error) {
	specs, err := hook.NewLoader(nil).LoadFromMigSpec(contracts.MigSpec{
		Hooks: []string{source},
	}, specRoot)
	if err != nil {
		return hook.Spec{}, err
	}
	if len(specs) == 0 {
		return hook.Spec{}, fmt.Errorf("no resolved hook spec for source %q", source)
	}
	return specs[0], nil
}

func hookSourceFromJobMeta(metaRaw []byte) string {
	if len(metaRaw) == 0 {
		return ""
	}
	meta, err := contracts.UnmarshalJobMeta(metaRaw)
	if err != nil || meta == nil {
		return ""
	}
	return strings.TrimSpace(meta.HookSource)
}
