package api

import (
	"encoding/json"
	"time"
)

// NamedSpecSource identifies the committed repository that published a named spec.
type NamedSpecSource struct {
	Domain string `json:"domain"`
	Repo   string `json:"repo"`
}

// PublishNamedSpecRequest is the canonical request DTO for POST /v1/specs.
type PublishNamedSpecRequest struct {
	Name              string          `json:"name"`
	Description       string          `json:"description,omitempty"`
	Source            NamedSpecSource `json:"source"`
	SHA               string          `json:"sha"`
	SourceCommittedAt time.Time       `json:"source_committed_at"`
	Spec              json.RawMessage `json:"spec"`
	CreatedBy         *string         `json:"created_by,omitempty"`
}

// NamedSpecSummary is the canonical response summary for a named spec row.
type NamedSpecSummary struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Source            NamedSpecSource `json:"source"`
	SHA               string          `json:"sha"`
	SourceCommittedAt time.Time       `json:"source_committed_at"`
	CreatedBy         *string         `json:"created_by,omitempty"`
	UpdatedBy         *string         `json:"updated_by,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	ArchivedAt        *time.Time      `json:"archived_at,omitempty"`
	Skipped           bool            `json:"skipped"`
}

// NamedSpecListResponse is returned by GET /v1/specs.
type NamedSpecListResponse struct {
	Specs []NamedSpecSummary `json:"specs"`
}

// NamedSpecResolveResponse is returned by GET /v1/specs/resolve.
type NamedSpecResolveResponse struct {
	NamedSpecSummary
	Spec json.RawMessage `json:"spec"`
}

// UpdateNamedSpecRequest is the canonical request DTO for PATCH /v1/specs/{spec_id}.
type UpdateNamedSpecRequest struct {
	Archived bool `json:"archived"`
}
