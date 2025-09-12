package transflow

import (
    "context"
    "time"
)

// reportJobSubmittedAsync reports a job submission with optional alloc ID after a brief delay.
func reportJobSubmittedAsync(ctx context.Context, rep EventReporter, jobName, phase, step string) {
    if rep == nil || jobName == "" {
        return
    }
    go func(job string) {
        // small delay to allow registration
        select {
        case <-time.After(1 * time.Second):
        case <-ctx.Done():
            return
        }
        if id := findFirstAllocID(job); id != "" {
            _ = rep.Report(ctx, Event{Phase: phase, Step: step, Level: "info", Message: "job submitted", JobName: job, AllocID: id, Time: time.Now()})
            return
        }
        _ = rep.Report(ctx, Event{Phase: phase, Step: step, Level: "info", Message: "job submitted", JobName: job, Time: time.Now()})
    }(jobName)
}

