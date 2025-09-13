package mods

import (
	"context"
	"fmt"
)

// mrEmitStart emits a standardized MR creation start message.
func mrEmitStart(r *TransflowRunner, ctx context.Context, sourceBranch, targetBranch string) {
	if r == nil {
		return
	}
	r.emit(ctx, "mr", "mr", "info", fmt.Sprintf("creating MR: source=%s target=%s", sourceBranch, targetBranch))
}

// mrAppendFailure appends a non-fatal MR failure step result to the workflow result.
func mrAppendFailure(result *TransflowResult, err error) {
	if result == nil || err == nil {
		return
	}
	result.StepResults = append(result.StepResults, StepResult{StepID: "mr", Success: true, Message: fmt.Sprintf("MR creation failed: %v", err)})
}

// mrAppendSuccess appends a success step result and sets the MR URL on the workflow result.
func mrAppendSuccess(result *TransflowResult, url string, created bool) {
	if result == nil || url == "" {
		return
	}
	action := "created"
	if !created {
		action = "updated"
	}
	result.StepResults = append(result.StepResults, StepResult{StepID: "mr", Success: true, Message: fmt.Sprintf("MR %s: %s", action, url)})
	result.MRURL = url
}
