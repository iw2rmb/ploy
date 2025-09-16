package mods

import (
	"context"
	"log"
	"time"
)

// emit sends an event either to the configured reporter or to the local log.
func (r *ModRunner) emit(ctx context.Context, phase, step, level, message string) {
	if r.eventReporter != nil {
		_ = r.eventReporter.Report(ctx, Event{Phase: phase, Step: step, Level: level, Message: message, Time: time.Now()})
		return
	}
	// Fallback to local log output when no reporter is configured
	log.Printf("[Mods][%s/%s][%s] %s", phase, step, level, message)
}

// GetEventReporter exposes the reporter for orchestrators
func (r *ModRunner) GetEventReporter() EventReporter {
	return r.eventReporter
}

// reportLastJobAsync looks up allocation ID and reports job metadata once available
func (r *ModRunner) reportLastJobAsync(ctx context.Context, jobName, phase, step string) {
	if r.eventReporter == nil || jobName == "" {
		return
	}
	go func() {
		// brief delay to allow registration
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return
		}
		deadline := time.Now().Add(1 * time.Minute)
		for time.Now().Before(deadline) {
			if id := findFirstAllocID(jobName); id != "" {
				_ = r.eventReporter.Report(ctx, Event{Phase: phase, Step: step, Level: "info", Message: "job submitted", JobName: jobName, AllocID: id, Time: time.Now()})
				return
			}
			select {
			case <-time.After(1 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}()
}
