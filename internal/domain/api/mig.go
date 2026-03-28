// Package api defines canonical domain-level DTO types for shared API payloads.
// These types are the authoritative definitions for wire shapes that cross the
// server/client/cli boundary. Handlers encode them; clients and CLI decode them.
//
// Ownership rules:
//   - Types here are stable across server, client, and CLI boundaries.
//   - JSON field names must not change without a coordinated wire-format migration.
//   - Handler-local and CLI-local mirrors of these shapes are superseded by this package.
package api

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// MigSummary is the canonical DTO for a single mig project entry.
// It is the authoritative response shape for:
//   - GET /v1/migs — individual items in the mig list
//   - POST /v1/migs — the created mig in the 201 response
//
// Wire shape is stable: JSON field names must not change.
type MigSummary struct {
	ID        domaintypes.MigID   `json:"id"`
	Name      string              `json:"name"`
	SpecID    *domaintypes.SpecID `json:"spec_id,omitempty"`
	CreatedBy *string             `json:"created_by,omitempty"`
	Archived  bool                `json:"archived"`
	CreatedAt time.Time           `json:"created_at"`
}

// MigListResponse is the canonical response envelope for GET /v1/migs.
type MigListResponse struct {
	Migs []MigSummary `json:"migs"`
}
