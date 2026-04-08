package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
	hookIndex, err := hookIndexFromJobName(job.Name, len(migSpec.Hooks))
	if err != nil {
		return nil, err
	}

	source := strings.TrimSpace(migSpec.Hooks[hookIndex])
	if source == "" {
		return nil, fmt.Errorf("hook source is empty for index %d", hookIndex)
	}
	hookSpec, err := loadRuntimeHookSpec(source)
	if err != nil {
		// Relative hook sources are valid in mig specs but may not be resolvable
		// from the control-plane filesystem at claim time. Do not block claims
		// in that case; execute hook job and collect completion metadata as usual.
		if isRelativeLocalHookSource(source) && errors.Is(err, os.ErrNotExist) {
			return &contracts.HookRuntimeDecision{HookShouldRun: true}, nil
		}
		return nil, fmt.Errorf("load hook spec for source %q: %w", source, err)
	}
	match, err := hook.Match(hookSpec, hook.MatchInput{})
	if err != nil {
		return nil, fmt.Errorf("evaluate hook matcher for source %q: %w", source, err)
	}
	hash := strings.TrimSpace(match.HookHash)
	if hash == "" {
		return nil, fmt.Errorf("hook matcher returned empty hash for source %q", source)
	}

	decision := &contracts.HookRuntimeDecision{
		HookHash:      hash,
		HookShouldRun: true,
	}
	if !match.Once.Enabled {
		return decision, nil
	}

	exists, err := st.HasHookOnceLedger(ctx, store.HasHookOnceLedgerParams{
		RunID:    job.RunID,
		RepoID:   job.RepoID,
		HookHash: hash,
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
		HookHash: hash,
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

func loadRuntimeHookSpec(source string) (hook.Spec, error) {
	specs, err := hook.NewLoader(nil).LoadFromMigSpec(contracts.MigSpec{
		Hooks: []string{source},
	}, ".")
	if err != nil {
		return hook.Spec{}, err
	}
	if len(specs) != 1 {
		return hook.Spec{}, fmt.Errorf("expected exactly 1 resolved hook spec, got %d", len(specs))
	}
	return specs[0], nil
}

func isRelativeLocalHookSource(source string) bool {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return false
	}
	if filepathLikeRemoteURL(trimmed) {
		return false
	}
	return !filepath.IsAbs(trimmed)
}

func filepathLikeRemoteURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return false
	}
	if (u.Scheme != "http" && u.Scheme != "https") || strings.TrimSpace(u.Host) == "" {
		return false
	}
	return true
}
