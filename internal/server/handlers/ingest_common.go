package handlers

import (
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// parseOptionalUUID parses an optional UUID string pointer into pgtype.UUID.
// Returns a zero pgtype.UUID when the pointer is nil or empty.
func parseOptionalUUID(s *string) (pgtype.UUID, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return pgtype.UUID{}, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}
