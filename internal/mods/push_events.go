package transflow

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// runPushWithEvents runs push and emits start/failure events, returning a StepResult.
func runPushWithEvents(r *TransflowRunner, ctx context.Context, repoPath, branchName string) (StepResult, error) {
	start := time.Now()
	r.emit(ctx, "push", "push", "info", "Pushing branch")
	if err := r.runPushStep(ctx, repoPath, branchName); err != nil {
		msg := fmt.Sprintf("push failed: %v", err)
		if strings.Contains(msg, "rc=128") || strings.Contains(msg, "exit status 128") {
			r.emit(ctx, "push", "push-failed-rc-128", "error", "push failed (rc=128)")
		}
		r.emit(ctx, "push", "push", "error", msg)
		return StepResult{StepID: "push", Success: false, Message: fmt.Sprintf("Push failed: %v", err), Duration: time.Since(start)}, err
	}
	return StepResult{StepID: "push", Success: true, Message: fmt.Sprintf("Pushed branch %s", branchName), Duration: time.Since(start)}, nil
}
