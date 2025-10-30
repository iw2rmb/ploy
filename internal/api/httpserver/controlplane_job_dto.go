package httpserver

import (
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

// jobDTO is the API representation for a job.
type jobDTO struct {
	ID             string                            `json:"id"`
	Ticket         string                            `json:"ticket"`
	StepID         string                            `json:"step_id"`
	Priority       string                            `json:"priority"`
	State          string                            `json:"state"`
	CreatedAt      string                            `json:"created_at"`
	EnqueuedAt     string                            `json:"enqueued_at"`
	ClaimedAt      string                            `json:"claimed_at,omitempty"`
	CompletedAt    string                            `json:"completed_at,omitempty"`
	ExpiresAt      string                            `json:"expires_at,omitempty"`
	LeaseID        int64                             `json:"lease_id,omitempty"`
	LeaseExpiresAt string                            `json:"lease_expires_at,omitempty"`
	ClaimedBy      string                            `json:"claimed_by,omitempty"`
	RetryAttempt   int                               `json:"retry_attempt"`
	MaxAttempts    int                               `json:"max_attempts"`
	Metadata       map[string]string                 `json:"metadata,omitempty"`
	Artifacts      map[string]string                 `json:"artifacts,omitempty"`
	Bundles        map[string]scheduler.BundleRecord `json:"bundles,omitempty"`
    Gate           *gateDTO                          `json:"gate,omitempty"`
	Retention      *scheduler.JobRetention           `json:"retention,omitempty"`
	NodeSnapshot   *nodeSnapshotDTO                  `json:"node_snapshot,omitempty"`
	Error          *scheduler.JobError               `json:"error,omitempty"`
}

// jobDTOFrom builds the jobDTO payload from a scheduler job.
func jobDTOFrom(job *scheduler.Job) jobDTO {
	return jobDTO{
		ID:             job.ID,
		Ticket:         job.Ticket,
		StepID:         job.StepID,
		Priority:       job.Priority,
		State:          string(job.State),
		CreatedAt:      job.CreatedAt.UTC().Format(time.RFC3339Nano),
		EnqueuedAt:     job.EnqueuedAt.UTC().Format(time.RFC3339Nano),
		ClaimedAt:      formatTime(job.ClaimedAt),
		CompletedAt:    formatTime(job.CompletedAt),
		ExpiresAt:      formatTime(job.ExpiresAt),
		LeaseID:        int64(job.LeaseID),
		LeaseExpiresAt: formatTime(job.LeaseExpiresAt),
		ClaimedBy:      job.ClaimedBy,
		RetryAttempt:   job.RetryAttempt,
		MaxAttempts:    job.MaxAttempts,
		Metadata:       copyMap(job.Metadata),
		Artifacts:      copyMap(job.Artifacts),
		Bundles:        copyBundles(job.Bundles),
        Gate:           copyGate(job.Gate),
		Retention:      copyRetention(job.Retention),
		NodeSnapshot:   copyNodeSnapshot(job.NodeSnapshot),
		Error:          job.Error,
	}
}

// formatTime converts zero-able timestamps into RFC3339Nano strings.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// copyMap clones string maps to keep response payloads immutable.
func copyMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// copyBundles duplicates bundle metadata to avoid leaking references.
func copyBundles(src map[string]scheduler.BundleRecord) map[string]scheduler.BundleRecord {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]scheduler.BundleRecord, len(src))
	for k, v := range src {
		out[k] = scheduler.BundleRecord{
			CID:       v.CID,
			Digest:    v.Digest,
			Size:      v.Size,
			Retained:  v.Retained,
			TTL:       v.TTL,
			ExpiresAt: v.ExpiresAt,
		}
	}
	return out
}

// gateDTO reports summarized build gate metrics for the job response.
type gateDTO struct {
    Result          string  `json:"result"`
    DurationSeconds float64 `json:"duration_seconds"`
}

// copyGate copies scheduler gate summaries into DTOs.
func copyGate(src *scheduler.GateSummary) *gateDTO {
    if src == nil {
        return nil
    }
    dst := &gateDTO{Result: src.Result}
    if src.Duration > 0 {
        dst.DurationSeconds = src.Duration.Seconds()
    }
    return dst
}

// copyRetention duplicates retention metadata for responses.
func copyRetention(src *scheduler.JobRetention) *scheduler.JobRetention {
	if src == nil {
		return nil
	}
	return &scheduler.JobRetention{
		Retained:   src.Retained,
		TTL:        src.TTL,
		ExpiresAt:  src.ExpiresAt,
		Bundle:     src.Bundle,
		BundleCID:  src.BundleCID,
		Inspection: src.Inspection,
	}
}

// nodeSnapshotDTO reports captured node telemetry for the job response.
type nodeSnapshotDTO struct {
	NodeID     string         `json:"node_id"`
	Capacity   map[string]any `json:"capacity,omitempty"`
	CapacityAt string         `json:"capacity_at,omitempty"`
	Status     map[string]any `json:"status,omitempty"`
	StatusAt   string         `json:"status_at,omitempty"`
}

// copyNodeSnapshot clones node snapshot metadata.
func copyNodeSnapshot(src *scheduler.JobNodeSnapshot) *nodeSnapshotDTO {
	if src == nil {
		return nil
	}
	dto := &nodeSnapshotDTO{NodeID: src.NodeID}
	if len(src.Capacity) > 0 {
		dto.Capacity = copyAnyMap(src.Capacity)
	}
	if !src.CapacityAt.IsZero() {
		dto.CapacityAt = src.CapacityAt.UTC().Format(time.RFC3339Nano)
	}
	if len(src.Status) > 0 {
		dto.Status = copyAnyMap(src.Status)
	}
	if !src.StatusAt.IsZero() {
		dto.StatusAt = src.StatusAt.UTC().Format(time.RFC3339Nano)
	}
	return dto
}

// copyAnyMap clones map[string]any payloads for DTO safety.
func copyAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
