package transflow

import (
	"context"
	"fmt"
	"time"

	provider "github.com/iw2rmb/ploy/internal/git/provider"
)

// createOrUpdateMR attempts to create or update an MR if a git provider is configured.
// It emits reporter events and updates result.MRURL on success. Errors are logged into result steps but do not fail the workflow.
func (r *TransflowRunner) createOrUpdateMR(ctx context.Context, result *TransflowResult, branchName string) {
	if r.gitProvider == nil {
		return
	}
	if err := r.gitProvider.ValidateConfiguration(); err != nil {
		r.emit(ctx, "mr", "mr", "warn", "MR creation skipped - configuration invalid")
		result.StepResults = append(result.StepResults, StepResult{
			StepID:  "mr",
			Success: true,
			Message: fmt.Sprintf("MR creation skipped - configuration invalid: %v", err),
		})
		return
	}
	mrConfig := provider.MRConfig{
		RepoURL:      r.config.TargetRepo,
		SourceBranch: branchName,
		TargetBranch: r.config.TargetBranch,
		Title:        fmt.Sprintf("Transflow: %s", r.config.ID),
		Description:  renderMRDescription(r, result),
		Labels:       []string{"ploy", "tfl"},
	}

	mrTimeout := 2 * time.Minute
	mrCtx, cancelMR := context.WithTimeout(ctx, mrTimeout)
	defer cancelMR()
	r.emit(ctx, "mr", "mr", "info", fmt.Sprintf("creating MR: source=%s target=%s", mrConfig.SourceBranch, mrConfig.TargetBranch))
	mrResult, err := r.gitProvider.CreateOrUpdateMR(mrCtx, mrConfig)
	if err != nil {
		result.StepResults = append(result.StepResults, StepResult{
			StepID:  "mr",
			Success: true,
			Message: fmt.Sprintf("MR creation failed: %v", err),
		})
		return
	}
	if mrResult != nil && mrResult.MRURL != "" {
		action := "created"
		if !mrResult.Created {
			action = "updated"
		}
		result.StepResults = append(result.StepResults, StepResult{
			StepID:  "mr",
			Success: true,
			Message: fmt.Sprintf("MR %s: %s", action, mrResult.MRURL),
		})
		result.MRURL = mrResult.MRURL
	}
}
