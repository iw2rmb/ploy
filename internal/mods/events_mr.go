package mods

import (
	"context"
	"fmt"
)

// mrEmitStart emits a standardized MR creation start message.
func mrEmitStart(r *ModRunner, ctx context.Context, sourceBranch, targetBranch string) {
	if r == nil {
		return
	}
	r.emit(ctx, "mr", "mr", "info", fmt.Sprintf("creating MR: source=%s target=%s", sourceBranch, targetBranch))
}

// mrAppendFailure appends a non-fatal MR failure step result to the workflow result.
func mrAppendFailure(result *ModResult, err error) {
	if result == nil || err == nil {
		return
	}
	msg := fmt.Sprintf("MR creation failed: %v", err)
	result.StepResults = append(result.StepResults, StepResult{StepID: "mr", Success: false, Message: msg, Report: &StepReportMeta{Type: "mr", ErrorSolved: msg}})
}

// mrAppendSuccess appends a success step result and sets the MR URL on the workflow result.
func mrAppendSuccess(result *ModResult, url string, created bool) {
	if result == nil || url == "" {
		return
	}
	action := "created"
	if !created {
		action = "updated"
	}
	result.StepResults = append(result.StepResults, StepResult{StepID: "mr", Success: true, Message: fmt.Sprintf("MR %s: %s", action, url), Report: &StepReportMeta{Type: "mr"}})
	result.MRURL = url
}
