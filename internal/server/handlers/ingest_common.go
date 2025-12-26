package handlers

import (
	"fmt"
	"net/http"
	"strings"
)

// requiredPathParam extracts and validates a required path parameter from the request.
// Returns the trimmed value or an error if the parameter is missing or empty.
// This provides consistent error messages across all handlers that need required
// path parameters (run ID, repo ID, etc.).
//
// Example usage:
//
//	runID, err := requiredPathParam(r, "id")
//	if err != nil {
//	    http.Error(w, err.Error(), http.StatusBadRequest)
//	    return
//	}
func requiredPathParam(r *http.Request, key string) (string, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	return val, nil
}

// optionalPathParam extracts an optional path parameter from the request.
// Returns nil if the parameter is missing or empty, otherwise returns a pointer
// to the trimmed value. Useful for handlers where a path parameter may be optional
// or have a fallback to query parameters.
func optionalPathParam(r *http.Request, key string) *string {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return nil
	}
	return &val
}

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
