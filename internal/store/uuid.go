package store

import (
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ToPGUUID converts a string-like domain identifier to pgtype.UUID.
// Empty or invalid UUID text returns the zero-value (Valid=false).
func ToPGUUID[S ~string](id S) pgtype.UUID {
	s := strings.TrimSpace(string(id))
	if s == "" {
		return pgtype.UUID{}
	}
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}
	}
	var b [16]byte
	copy(b[:], u[:])
	return pgtype.UUID{Bytes: b, Valid: true}
}

// FromPGUUID converts a pgtype.UUID to a string-like domain identifier.
// When the input is not valid, it returns the zero value for S (empty string).
func FromPGUUID[S ~string](u pgtype.UUID) S {
	if !u.Valid {
		var zero S
		return zero
	}
	var x uuid.UUID
	copy(x[:], u.Bytes[:])
	return S(x.String())
}
