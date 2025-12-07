package handlers

import "strings"

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
