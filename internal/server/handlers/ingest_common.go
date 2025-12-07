package handlers

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// parseOptionalUUID parses an optional UUID string pointer into pgtype.UUID.
// Returns a zero pgtype.UUID when the pointer is nil or empty.
// NOTE: This function is used for node_id fields which remain as UUID.
// For run/job/build IDs, use normalizeOptionalID instead.
func parseOptionalUUID(s *string) (pgtype.UUID, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return pgtype.UUID{}, nil
	}
	id := domaintypes.ToPGUUID(*s)
	if !id.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid")
	}
	return id, nil
}

// normalizeOptionalID normalizes an optional string ID (run_id, job_id, build_id).
// Returns nil when the pointer is nil or whitespace-only, otherwise returns
// a pointer to the trimmed string. Since run/job/build IDs are now KSUID-backed
// strings, no UUID validation is performed; IDs are treated as opaque.
func normalizeOptionalID(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
