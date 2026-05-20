package events

import (
	"context"
	"strings"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

// jobContext holds execution context extracted from job metadata.
// Used to enrich log records with node and mig information.
// Uses domain types to preserve type safety end-to-end without lossy casts.
type jobContext struct {
	NodeID  domaintypes.NodeID
	JobID   domaintypes.JobID
	JobType domaintypes.JobType
}

// loadJobContext fetches job metadata for a given job ID and extracts
// fields needed to enrich log records. Returns an empty context if the
// job ID is nil/empty or the lookup fails (logs are still published without
// enrichment in these cases).
func (s *Service) loadJobContext(ctx context.Context, jobID *domaintypes.JobID) jobContext {
	if jobID == nil || jobID.IsZero() {
		return jobContext{}
	}

	if cached, ok := s.jobCache.Get(*jobID); ok {
		return cached
	}

	if s.store == nil {
		return jobContext{}
	}

	job, err := s.store.GetJob(ctx, *jobID)
	if err != nil {
		s.logger.Debug("job lookup failed for log enrichment",
			"job_id", jobID.String(),
			"error", err)
		return jobContext{}
	}

	// Normalize and validate job metadata before enrichment. Invalid values
	// are omitted so the emitted SSE payload contract stays strict.

	jid := job.ID

	var nid domaintypes.NodeID
	if job.NodeID != nil && !job.NodeID.IsZero() {
		nid = *job.NodeID
	}

	mt := domaintypes.JobType(domaintypes.Normalize(job.JobType.String()))
	if !mt.IsZero() {
		if err := mt.Validate(); err != nil {
			s.logger.Debug("invalid job_type for log enrichment",
				"job_id", job.ID.String(),
				"job_type", job.JobType,
				"error", err,
			)
			mt = ""
		}
	}

	jctx := jobContext{
		NodeID:  nid,
		JobID:   jid,
		JobType: mt,
	}
	s.jobCache.Set(*jobID, jctx)
	return jctx
}

// timestampToString converts a pgtype.Timestamptz to RFC3339 string.
// Returns empty string if the timestamp is invalid or null.
func timestampToString(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.Format(time.RFC3339)
}

// normalizeEventLevel canonicalizes and validates event level using domain LogLevel.
// It maps unknown or empty values to "info" to keep storage/SSE streams consistent.
func normalizeEventLevel(level string) string {
	s := strings.ToLower(domaintypes.Normalize(level))
	if domaintypes.IsEmpty(s) {
		return domaintypes.LogLevelInfo.String()
	}
	l := domaintypes.LogLevel(s)
	if err := l.Validate(); err != nil {
		return domaintypes.LogLevelInfo.String()
	}
	return l.String()
}
