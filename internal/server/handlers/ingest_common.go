package handlers

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// parseOptionalUUID parses an optional UUID string pointer into pgtype.UUID.
// Returns a zero pgtype.UUID when the pointer is nil or empty.
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
