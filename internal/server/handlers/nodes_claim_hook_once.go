package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
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
	hash := hookSourceHash(source)

	decision := &contracts.HookRuntimeDecision{
		HookHash:      hash,
		HookShouldRun: true,
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

func hookSourceHash(source string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(source)))
	return hex.EncodeToString(sum[:])
}
