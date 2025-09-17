package mods

import (
	"context"
	"fmt"
	"time"

	provider "github.com/iw2rmb/ploy/internal/git/provider"
)

// createOrUpdateMR attempts to create or update an MR if a git provider is configured.
// It emits reporter events and updates result.MRURL on success. Errors are logged into result steps but do not fail the workflow.
func (r *ModRunner) createOrUpdateMR(ctx context.Context, result *ModResult, branchName string) {
	if r.gitProvider == nil && r.mrManager == nil {
		return
	}
	// When MRManager is present, prefer it
	if r.mrManager != nil {
		labels := []string{"ploy", "tfl"}
		if r.config != nil && r.config.MR != nil && len(r.config.MR.Labels) > 0 {
			labels = r.config.MR.Labels
		}
		target := r.config.TargetBranch
		if target == "" {
			target = r.config.BaseRef
		}
		mrConfig := provider.MRConfig{
			RepoURL:      r.config.TargetRepo,
			SourceBranch: branchName,
			TargetBranch: target,
			Title:        fmt.Sprintf("Mods: %s", r.config.ID),
			Description:  renderMRDescription(r, result),
			Labels:       labels,
		}
		mrTimeout := 2 * time.Minute
		mrCtx, cancelMR := context.WithTimeout(ctx, mrTimeout)
		defer cancelMR()
		mrEmitStart(r, ctx, mrConfig.SourceBranch, mrConfig.TargetBranch)
		url, meta, err := r.mrManager.CreateOrUpdate(mrCtx, mrConfig)
		if err != nil {
			mrAppendFailure(result, err)
			return
		}
		if url != "" {
			created, _ := meta["created"].(bool)
			mrAppendSuccess(result, url, created)
		}
		return
	}
	if err := r.gitProvider.ValidateConfiguration(); err != nil {
		r.emit(ctx, "mr", "mr", "warn", "MR creation skipped - configuration invalid")
		result.StepResults = append(result.StepResults, StepResult{
			StepID:  "mr",
			Success: false,
			Message: fmt.Sprintf("MR creation skipped - configuration invalid: %v", err),
		})
		return
	}
	target := r.config.TargetBranch
	if target == "" {
		target = r.config.BaseRef
	}
	mrConfig := provider.MRConfig{
		RepoURL:      r.config.TargetRepo,
		SourceBranch: branchName,
		TargetBranch: target,
		Title:        fmt.Sprintf("Mods: %s", r.config.ID),
		Description:  renderMRDescription(r, result),
		Labels:       []string{"ploy", "tfl"},
	}

	mrTimeout := 2 * time.Minute
	mrCtx, cancelMR := context.WithTimeout(ctx, mrTimeout)
	defer cancelMR()
	mrEmitStart(r, ctx, mrConfig.SourceBranch, mrConfig.TargetBranch)
	// Apply MR auth/env selection before provider invocation as well
	r.applyMRAuthFromConfig(ctx)
	mrResult, err := r.gitProvider.CreateOrUpdateMR(mrCtx, mrConfig)
	if err != nil {
		mrAppendFailure(result, err)
		return
	}
	if mrResult != nil && mrResult.MRURL != "" {
		mrAppendSuccess(result, mrResult.MRURL, mrResult.Created)
	}
}
