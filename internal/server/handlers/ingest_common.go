package handlers

import "strings"

// normalizeOptionalID normalizes an optional string ID (run_id, job_id).
// Returns nil when the pointer is nil or whitespace-only, otherwise returns
// a pointer to the trimmed string. Since run/job IDs are now KSUID-backed
// strings, no UUID validation is performed; IDs are treated as opaque.
// Note: build_id removed as part of builds table removal; logs/artifacts now use job-level grouping only.
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
