package artifacts

import (
	"sort"
	"strings"
	"time"
)

// PinState describes the replication status recorded for an artifact.
type PinState string

const (
	// PinStateQueued marks a pin that has been accepted but not yet processed.
	PinStateQueued PinState = "queued"
	// PinStatePinning captures pins actively replicating to peers.
	PinStatePinning PinState = "pinning"
	// PinStatePinned indicates replication success.
	PinStatePinned PinState = "pinned"
	// PinStateFailed marks pins that require manual or automated retries.
	PinStateFailed PinState = "failed"
)

// Metadata captures persisted artifact attributes backed by etcd.
type Metadata struct {
	ID                   string
	SlotID               string
	CID                  string
	Digest               string
	Size                 int64
	JobID                string
	Stage                string
	Kind                 string
	NodeID               string
	Name                 string
	Source               string
	TTL                  string
	ExpiresAt            time.Time
	ReplicationFactorMin int
	ReplicationFactorMax int
	PinState             PinState
	PinReplicas          int
	PinUpdatedAt         time.Time
	PinRetryCount        int
	PinNextAttemptAt     time.Time
	PinError             string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Deleted              bool
	DeletedAt            time.Time
}

// ListOptions scope artifact listings.
type ListOptions struct {
	JobID          string
	Stage          string
	CID            string
	Cursor         string
	Limit          int
	IncludeDeleted bool
}

// ListResult wraps a page of artifact metadata.
type ListResult struct {
	Artifacts  []Metadata
	NextCursor string
}

func recordFromMetadata(meta Metadata) metadataRecord {
	rec := metadataRecord{
		ID:                   meta.ID,
		SlotID:               meta.SlotID,
		CID:                  meta.CID,
		Digest:               meta.Digest,
		Size:                 meta.Size,
		JobID:                meta.JobID,
		Stage:                meta.Stage,
		Kind:                 meta.Kind,
		NodeID:               meta.NodeID,
		Name:                 meta.Name,
		Source:               meta.Source,
		TTL:                  meta.TTL,
		ReplicationFactorMin: meta.ReplicationFactorMin,
		ReplicationFactorMax: meta.ReplicationFactorMax,
		PinState:             string(meta.PinState),
		PinReplicas:          meta.PinReplicas,
		PinRetryCount:        meta.PinRetryCount,
		PinError:             meta.PinError,
		CreatedAt:            meta.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:            meta.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Deleted:              meta.Deleted,
	}
	if !meta.ExpiresAt.IsZero() {
		rec.ExpiresAt = meta.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if !meta.DeletedAt.IsZero() {
		rec.DeletedAt = meta.DeletedAt.UTC().Format(time.RFC3339Nano)
	}
	if !meta.PinUpdatedAt.IsZero() {
		rec.PinUpdatedAt = meta.PinUpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	if !meta.PinNextAttemptAt.IsZero() {
		rec.PinNextAttemptAt = meta.PinNextAttemptAt.UTC().Format(time.RFC3339Nano)
	}
	return rec
}

type metadataRecord struct {
	ID                   string `json:"id"`
	SlotID               string `json:"slot_id,omitempty"`
	CID                  string `json:"cid"`
	Digest               string `json:"digest"`
	Size                 int64  `json:"size"`
	JobID                string `json:"job_id"`
	Stage                string `json:"stage,omitempty"`
	Kind                 string `json:"kind,omitempty"`
	NodeID               string `json:"node_id,omitempty"`
	Name                 string `json:"name,omitempty"`
	Source               string `json:"source,omitempty"`
	TTL                  string `json:"ttl,omitempty"`
	ExpiresAt            string `json:"expires_at,omitempty"`
	ReplicationFactorMin int    `json:"replication_factor_min,omitempty"`
	ReplicationFactorMax int    `json:"replication_factor_max,omitempty"`
	PinState             string `json:"pin_state,omitempty"`
	PinReplicas          int    `json:"pin_replicas,omitempty"`
	PinUpdatedAt         string `json:"pin_updated_at,omitempty"`
	PinRetryCount        int    `json:"pin_retry_count,omitempty"`
	PinNextAttemptAt     string `json:"pin_next_attempt_at,omitempty"`
	PinError             string `json:"pin_error,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
	Deleted              bool   `json:"deleted,omitempty"`
	DeletedAt            string `json:"deleted_at,omitempty"`
}

func (r metadataRecord) toMetadata() Metadata {
	meta := Metadata{
		ID:                   strings.TrimSpace(r.ID),
		SlotID:               strings.TrimSpace(r.SlotID),
		CID:                  strings.TrimSpace(r.CID),
		Digest:               strings.TrimSpace(r.Digest),
		Size:                 r.Size,
		JobID:                strings.TrimSpace(r.JobID),
		Stage:                strings.TrimSpace(r.Stage),
		Kind:                 strings.TrimSpace(r.Kind),
		NodeID:               strings.TrimSpace(r.NodeID),
		Name:                 strings.TrimSpace(r.Name),
		Source:               strings.TrimSpace(r.Source),
		TTL:                  strings.TrimSpace(r.TTL),
		ReplicationFactorMin: r.ReplicationFactorMin,
		ReplicationFactorMax: r.ReplicationFactorMax,
		PinState:             PinState(strings.TrimSpace(r.PinState)),
		PinReplicas:          r.PinReplicas,
		PinRetryCount:        r.PinRetryCount,
		PinError:             strings.TrimSpace(r.PinError),
		Deleted:              r.Deleted,
	}
	if ts, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err == nil {
		meta.CreatedAt = ts.UTC()
	}
	if ts, err := time.Parse(time.RFC3339Nano, r.UpdatedAt); err == nil {
		meta.UpdatedAt = ts.UTC()
	}
	if strings.TrimSpace(r.ExpiresAt) != "" {
		if ts, err := time.Parse(time.RFC3339Nano, r.ExpiresAt); err == nil {
			meta.ExpiresAt = ts.UTC()
		}
	}
	if strings.TrimSpace(r.DeletedAt) != "" {
		if ts, err := time.Parse(time.RFC3339Nano, r.DeletedAt); err == nil {
			meta.DeletedAt = ts.UTC()
		}
	}
	if strings.TrimSpace(r.PinUpdatedAt) != "" {
		if ts, err := time.Parse(time.RFC3339Nano, r.PinUpdatedAt); err == nil {
			meta.PinUpdatedAt = ts.UTC()
		}
	}
	if strings.TrimSpace(r.PinNextAttemptAt) != "" {
		if ts, err := time.Parse(time.RFC3339Nano, r.PinNextAttemptAt); err == nil {
			meta.PinNextAttemptAt = ts.UTC()
		}
	}
	return meta
}

// SortArtifactsByCreatedDesc sorts metadata in-place by CreatedAt descending.
func SortArtifactsByCreatedDesc(items []Metadata) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
}
