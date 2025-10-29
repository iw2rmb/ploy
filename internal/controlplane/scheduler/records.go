package scheduler

import (
	"encoding/json"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type jobRecord struct {
	ID             string                  `json:"id"`
	Ticket         string                  `json:"ticket"`
	StepID         string                  `json:"step_id"`
	Priority       string                  `json:"priority"`
	State          JobState                `json:"state"`
	CreatedAt      string                  `json:"created_at"`
	EnqueuedAt     string                  `json:"enqueued_at"`
	ClaimedAt      string                  `json:"claimed_at,omitempty"`
	CompletedAt    string                  `json:"completed_at,omitempty"`
	ExpiresAt      string                  `json:"expires_at,omitempty"`
	LeaseID        int64                   `json:"lease_id,omitempty"`
	LeaseExpiresAt string                  `json:"lease_expires_at,omitempty"`
	ClaimedBy      string                  `json:"claimed_by,omitempty"`
	RetryAttempt   int                     `json:"retry_attempt"`
	MaxAttempts    int                     `json:"max_attempts"`
	Metadata       map[string]string       `json:"metadata,omitempty"`
	Artifacts      map[string]string       `json:"artifacts,omitempty"`
	Bundles        map[string]bundleRecord `json:"bundles,omitempty"`
	Shift          *shiftRecord            `json:"shift,omitempty"`
	Retention      *retentionRecord        `json:"retention,omitempty"`
	NodeSnapshot   *nodeSnapshotRecord     `json:"node_snapshot,omitempty"`
	Error          *JobError               `json:"error,omitempty"`
}

func (r jobRecord) toJob() *Job {
	return &Job{
		ID:             r.ID,
		Ticket:         r.Ticket,
		StepID:         r.StepID,
		Priority:       r.Priority,
		State:          r.State,
		CreatedAt:      decodeTime(r.CreatedAt),
		EnqueuedAt:     decodeTime(r.EnqueuedAt),
		ClaimedAt:      decodeTime(r.ClaimedAt),
		CompletedAt:    decodeTime(r.CompletedAt),
		ExpiresAt:      decodeTime(r.ExpiresAt),
		LeaseID:        clientv3.LeaseID(r.LeaseID),
		LeaseExpiresAt: decodeTime(r.LeaseExpiresAt),
		ClaimedBy:      r.ClaimedBy,
		RetryAttempt:   r.RetryAttempt,
		MaxAttempts:    r.MaxAttempts,
		Metadata:       cloneMap(r.Metadata),
		Artifacts:      cloneMap(r.Artifacts),
		Bundles:        exportBundleRecords(r.Bundles),
		Shift:          exportShiftSummary(r.Shift),
		Retention:      exportRetention(r.Retention),
		NodeSnapshot:   exportNodeSnapshot(r.NodeSnapshot),
		Error:          r.Error,
	}
}

func decodeJobRecord(data []byte) (jobRecord, error) {
	var record jobRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return jobRecord{}, err
	}
	return record, nil
}

type bundleRecord struct {
	CID       string `json:"cid,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Retained  bool   `json:"retained,omitempty"`
	TTL       string `json:"ttl,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type retentionRecord struct {
	Retained   bool   `json:"retained,omitempty"`
	TTL        string `json:"ttl,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Bundle     string `json:"bundle,omitempty"`
	BundleCID  string `json:"bundle_cid,omitempty"`
	Inspection bool   `json:"inspection,omitempty"`
}

type nodeSnapshotRecord struct {
	NodeID     string         `json:"node_id"`
	Capacity   map[string]any `json:"capacity,omitempty"`
	CapacityAt string         `json:"capacity_at,omitempty"`
	Status     map[string]any `json:"status,omitempty"`
	StatusAt   string         `json:"status_at,omitempty"`
}

// exportBundleRecords converts stored bundle metadata into public scheduler state.
func exportBundleRecords(src map[string]bundleRecord) map[string]BundleRecord {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]BundleRecord, len(src))
	for name, rec := range src {
		dst[name] = BundleRecord(rec)
	}
	return dst
}

func exportShiftSummary(rec *shiftRecord) *ShiftSummary {
	if rec == nil {
		return nil
	}
	summary := &ShiftSummary{Result: rec.Result}
	if rec.DurationSeconds > 0 {
		summary.Duration = time.Duration(rec.DurationSeconds * float64(time.Second))
	}
	return summary
}

func exportRetention(src *retentionRecord) *JobRetention {
	if src == nil {
		return nil
	}
	return &JobRetention{
		Retained:   src.Retained,
		TTL:        src.TTL,
		ExpiresAt:  src.ExpiresAt,
		Bundle:     src.Bundle,
		BundleCID:  src.BundleCID,
		Inspection: src.Inspection,
	}
}

func exportNodeSnapshot(rec *nodeSnapshotRecord) *JobNodeSnapshot {
	if rec == nil {
		return nil
	}
	snapshot := &JobNodeSnapshot{
		NodeID: rec.NodeID,
	}
	if len(rec.Capacity) > 0 {
		snapshot.Capacity = cloneAnyMap(rec.Capacity)
	}
	if len(rec.Status) > 0 {
		snapshot.Status = cloneAnyMap(rec.Status)
	}
	if strings.TrimSpace(rec.CapacityAt) != "" {
		snapshot.CapacityAt = decodeTime(rec.CapacityAt)
	}
	if strings.TrimSpace(rec.StatusAt) != "" {
		snapshot.StatusAt = decodeTime(rec.StatusAt)
	}
	return snapshot
}

// normalizeBundleRecords trims bundle fields and computes expiry timestamps.
func normalizeBundleRecords(src map[string]BundleRecord, completedAt time.Time) map[string]bundleRecord {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]bundleRecord, len(src))
	for name, rec := range src {
		cid := strings.TrimSpace(rec.CID)
		digest := strings.TrimSpace(rec.Digest)
		ttl := strings.TrimSpace(rec.TTL)
		expires := strings.TrimSpace(rec.ExpiresAt)
		if expires == "" && !completedAt.IsZero() && ttl != "" {
			if duration, err := time.ParseDuration(ttl); err == nil && duration > 0 {
				expires = completedAt.Add(duration).UTC().Format(time.RFC3339Nano)
			}
		}
		dst[name] = bundleRecord{
			CID:       cid,
			Digest:    digest,
			Size:      rec.Size,
			Retained:  rec.Retained,
			TTL:       ttl,
			ExpiresAt: expires,
		}
	}
	return dst
}

func cloneBundleMap(src map[string]BundleRecord) map[string]BundleRecord {
	if len(src) == 0 {
		return nil
	}
	dup := make(map[string]BundleRecord, len(src))
	for k, v := range src {
		dup[k] = v
	}
	return dup
}

func computeRetentionExpiry(bundles map[string]bundleRecord, completedAt time.Time) time.Time {
	var latest time.Time
	for _, rec := range bundles {
		if expires := decodeTime(rec.ExpiresAt); !expires.IsZero() {
			if expires.After(latest) {
				latest = expires
			}
			continue
		}
		ttl := strings.TrimSpace(rec.TTL)
		if ttl == "" {
			continue
		}
		duration, err := time.ParseDuration(ttl)
		if err != nil || duration <= 0 {
			continue
		}
		candidate := completedAt.Add(duration).UTC()
		if candidate.After(latest) {
			latest = candidate
		}
	}
	return latest
}

func deriveRetentionRecord(bundles map[string]bundleRecord, expiry time.Time, inspection bool) *retentionRecord {
	var (
		found bool
		name  string
		rec   bundleRecord
	)
	if len(bundles) > 0 {
		if candidate, ok := bundles["logs"]; ok && (candidate.Retained || strings.TrimSpace(candidate.TTL) != "" || strings.TrimSpace(candidate.ExpiresAt) != "" || strings.TrimSpace(candidate.CID) != "") {
			found = true
			name = "logs"
			rec = candidate
		} else {
			for key, candidate := range bundles {
				if candidate.Retained || strings.TrimSpace(candidate.TTL) != "" || strings.TrimSpace(candidate.ExpiresAt) != "" || strings.TrimSpace(candidate.CID) != "" {
					found = true
					name = key
					rec = candidate
					break
				}
			}
		}
	}

	if !found && !inspection {
		return nil
	}

	summary := &retentionRecord{
		Bundle:     name,
		BundleCID:  strings.TrimSpace(rec.CID),
		TTL:        strings.TrimSpace(rec.TTL),
		Retained:   rec.Retained,
		Inspection: inspection,
	}

	if expiry.IsZero() {
		summary.ExpiresAt = ""
	} else {
		summary.ExpiresAt = encodeTime(expiry)
	}
	if trimmed := strings.TrimSpace(rec.ExpiresAt); trimmed != "" {
		summary.ExpiresAt = trimmed
	}
	if summary.Retained || inspection {
		summary.Retained = true
	}
	if !found {
		summary.BundleCID = ""
		summary.TTL = ""
		summary.Bundle = ""
	}
	return summary
}

type queueEntry struct {
	JobID        string            `json:"job_id"`
	Ticket       string            `json:"ticket"`
	StepID       string            `json:"step_id"`
	Priority     string            `json:"priority"`
	RetryAttempt int               `json:"retry_attempt"`
	MaxAttempts  int               `json:"max_attempts"`
	EnqueuedAt   string            `json:"enqueued_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type shiftRecord struct {
	Result          string  `json:"result,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
}

type leaseEntry struct {
	JobID    string `json:"job_id"`
	Ticket   string `json:"ticket"`
	Priority string `json:"priority"`
}

type gcEntry struct {
	JobID      string   `json:"job_id"`
	Ticket     string   `json:"ticket"`
	State      JobState `json:"state"`
	FinalState string   `json:"final_state"`
	ExpiresAt  string   `json:"expires_at"`
}
