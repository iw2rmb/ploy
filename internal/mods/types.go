package transflow

import (
	"context"
	"fmt"
	"time"
)

// Job submission types for transflow healing workflow

// JobSpec describes a job to be submitted to the orchestrator
type JobSpec struct {
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	HCLPath string                 `json:"hcl_path"`
	EnvVars map[string]string      `json:"env_vars"`
	Timeout time.Duration          `json:"timeout"`
	Inputs  map[string]interface{} `json:"inputs"`
}

// JobResult contains the result of a completed job
type JobResult struct {
	JobID     string            `json:"job_id"`
	Status    string            `json:"status"` // "completed", "failed", "timeout"
	Output    string            `json:"output"`
	Error     string            `json:"error"`
	Duration  time.Duration     `json:"duration"`
	Artifacts map[string]string `json:"artifacts"`
}

// PlanResult contains the output from a planner job
type PlanResult struct {
	PlanID  string                   `json:"plan_id"`
	Options []map[string]interface{} `json:"options"`
}

// NextAction describes what the reducer determined should happen next
type NextAction struct {
	Action string `json:"action"`
	Notes  string `json:"notes"`
	StepID string `json:"step_id,omitempty"`
}

// BranchSpec describes a healing branch to execute
type BranchSpec struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Inputs map[string]interface{} `json:"inputs"`
}

// BranchResult contains the result of executing a healing branch
type BranchResult struct {
	ID         string        `json:"id"`
	Status     string        `json:"status"`
	JobID      string        `json:"job_id"`
	Notes      string        `json:"notes"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Duration   time.Duration `json:"duration"`
}

// Interface definitions

// JobSubmissionHelper provides methods for submitting transflow jobs
type JobSubmissionHelper interface {
	SubmitPlannerJob(ctx context.Context, config *TransflowConfig, buildError string, workspace string) (*PlanResult, error)
	SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error)
}

// FanoutOrchestrator manages parallel execution of healing branches
type FanoutOrchestrator interface {
	RunHealingFanout(ctx context.Context, runCtx interface{}, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error)
}

// JobSubmitter abstracts job submission for tests and production
type JobSubmitter interface {
	SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error)
}

// NoopJobSubmitter is a minimal submitter that returns an error when used.
// It serves only as a marker to enable healing while production paths use the runner.
type NoopJobSubmitter struct{}

func (NoopJobSubmitter) SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error) {
	return JobResult{}, fmt.Errorf("NoopJobSubmitter cannot submit jobs")
}
