package scheduler

import (
	"errors"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	metrics "github.com/iw2rmb/ploy/internal/metrics"
)

// JobState represents persisted job lifecycle states.
type JobState string

const (
	// JobStateQueued indicates the job is waiting to be claimed.
	JobStateQueued JobState = "queued"
	// JobStateRunning indicates the job has been claimed and is executing.
	JobStateRunning JobState = "running"
	// JobStateSucceeded indicates the job finished successfully.
	JobStateSucceeded JobState = "succeeded"
	// JobStateFailed indicates the job finished with a failure.
	JobStateFailed JobState = "failed"
	// JobStateInspectionReady indicates the job is preserved for manual inspection.
	JobStateInspectionReady JobState = "inspection_ready"
)

// Shift result classifications recorded with job completions.
const (
	ShiftResultPassed = "passed"
	ShiftResultFailed = "failed"
)

// JobSpec describes a job submission.
type JobSpec struct {
	Ticket      string
	StepID      string
	Priority    string
	MaxAttempts int
	Metadata    map[string]string
}

// JobError carries error metadata persisted with a job.
type JobError struct {
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// Job describes the durable job record returned to callers.
type Job struct {
	ID             string
	Ticket         string
	StepID         string
	Priority       string
	State          JobState
	CreatedAt      time.Time
	EnqueuedAt     time.Time
	ClaimedAt      time.Time
	CompletedAt    time.Time
	LeaseID        clientv3.LeaseID
	LeaseExpiresAt time.Time
	ClaimedBy      string
	RetryAttempt   int
	MaxAttempts    int
	Metadata       map[string]string
	Artifacts      map[string]string
	Bundles        map[string]BundleRecord
	Shift          *ShiftSummary
	Retention      *JobRetention
	Error          *JobError
}

// ShiftSummary stores SHIFT execution metadata persisted with the job.
type ShiftSummary struct {
	Result   string
	Duration time.Duration
}

// ShiftMetrics captures SHIFT execution details supplied when completing a job.
type ShiftMetrics struct {
	Result   string
	Duration time.Duration
}

// ClaimRequest scopes a claim attempt.
type ClaimRequest struct {
	NodeID string
}

// ClaimResult returns claim metadata.
type ClaimResult struct {
	NodeID  string
	LeaseID clientv3.LeaseID
	Job     *Job
}

// HeartbeatRequest renews a job lease.
type HeartbeatRequest struct {
	JobID  string
	NodeID string
	Ticket string
}

// CompleteRequest transitions a job into a terminal state.
type CompleteRequest struct {
	JobID      string
	NodeID     string
	Ticket     string
	State      JobState
	Artifacts  map[string]string
	Shift      *ShiftMetrics
	Error      *JobError
	Inspection bool
	Bundles    map[string]BundleRecord
}

// Options configures the scheduler.
type Options struct {
	JobsPrefix      string
	QueuePrefix     string
	LeasesPrefix    string
	NodesPrefix     string
	GCPrefix        string
	LeaseTTL        time.Duration
	ClockSkewBuffer time.Duration
	IDGenerator     func() string
	Now             func() time.Time
	Metrics         metrics.SchedulerRecorder
}

// ErrNoJobs signals no work was available when claiming.
var ErrNoJobs = errors.New("scheduler: no jobs available")

// BundleRecord stores retention metadata for an artifact bundle.
type BundleRecord struct {
	CID       string `json:"cid,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Retained  bool   `json:"retained,omitempty"`
	TTL       string `json:"ttl,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// JobRetention summarises retention metadata per job for audit use.
type JobRetention struct {
	Retained   bool   `json:"retained"`
	TTL        string `json:"ttl,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Bundle     string `json:"bundle,omitempty"`
	BundleCID  string `json:"bundle_cid,omitempty"`
	Inspection bool   `json:"inspection,omitempty"`
}
