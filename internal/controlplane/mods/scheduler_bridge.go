package mods

import (
	"context"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

// SchedulerBridge adapts the control-plane scheduler to the orchestrator submitter interface.
type SchedulerBridge struct {
	scheduler *scheduler.Scheduler
}

// NewSchedulerBridge constructs a StageJobSubmitter backed by the scheduler.
func NewSchedulerBridge(s *scheduler.Scheduler) StageJobSubmitter {
	return &SchedulerBridge{scheduler: s}
}

// SubmitStageJob enqueues a Mods stage as a scheduler job.
func (b *SchedulerBridge) SubmitStageJob(ctx context.Context, spec StageJobSpec) (StageJob, error) {
	if b.scheduler == nil {
		return StageJob{}, fmt.Errorf("mods: scheduler bridge unavailable")
	}
	priority := strings.TrimSpace(spec.Priority)
	if priority == "" {
		priority = "default"
	}
	job, err := b.scheduler.SubmitJob(ctx, scheduler.JobSpec{
		Ticket:      spec.TicketID,
		StepID:      spec.StageID,
		Priority:    priority,
		MaxAttempts: spec.MaxAttempts,
		Metadata:    cloneMap(spec.Metadata),
	})
	if err != nil {
		return StageJob{}, err
	}
	return StageJob{
		JobID:    job.ID,
		TicketID: job.Ticket,
		StageID:  job.StepID,
	}, nil
}
