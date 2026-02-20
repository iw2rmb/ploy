package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
//	    httpErr(w, http.StatusBadRequest, "%s", err)
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

// httpErr writes a plain-text HTTP error response. It accepts printf-style
// formatting for the message body.
func httpErr(w http.ResponseWriter, code int, msg string, args ...any) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	http.Error(w, msg, code)
}

// DefaultMaxBodySize is the default request body size limit (1 MiB).
const DefaultMaxBodySize = 1 << 20

// DecodeJSON decodes a JSON request body with strict validation:
//   - Caps request body at maxBytes using http.MaxBytesReader
//   - Rejects unknown JSON fields (fails fast on contract drift)
//
// Returns nil on success. On error, writes an appropriate HTTP response and returns the error.
// Callers should return immediately after a non-nil error.
//
// Usage:
//
//	var req MyRequest
//	if err := DecodeJSON(w, r, &req, DefaultMaxBodySize); err != nil {
//	    return
//	}
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		// Return 413 when MaxBytesReader trips the size cap.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpErr(w, http.StatusRequestEntityTooLarge, "payload exceeds body size cap")
			return err
		}
		httpErr(w, http.StatusBadRequest, "invalid request: %v", err)
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		httpErr(w, http.StatusBadRequest, "invalid request: request body must contain exactly one JSON value")
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return fmt.Errorf("request body must contain exactly one JSON value: %w", err)
	}
	return nil
}
