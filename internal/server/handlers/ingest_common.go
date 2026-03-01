package handlers

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// requiredPathParam extracts and validates a required path parameter from the request.
// Returns the trimmed value or an error if the parameter is missing or empty.
func requiredPathParam(r *http.Request, key string) (string, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	return val, nil
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

// parseParam extracts and validates a typed ID from a path parameter.
// T must implement encoding.TextUnmarshaler (all domain ID types do).
// Returns an error if the parameter is missing, empty, or fails validation.
func parseParam[T any, PT interface {
	*T
	encoding.TextUnmarshaler
}](r *http.Request, key string) (T, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		var zero T
		return zero, fmt.Errorf("%s path parameter is required", key)
	}
	var id T
	if err := PT(&id).UnmarshalText([]byte(val)); err != nil {
		var zero T
		return zero, fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// optionalParam extracts an optional typed ID from a path parameter.
// Returns nil if the parameter is missing or empty.
func optionalParam[T any, PT interface {
	*T
	encoding.TextUnmarshaler
}](r *http.Request, key string) (*T, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return nil, nil
	}
	var id T
	if err := PT(&id).UnmarshalText([]byte(val)); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &id, nil
}

// parseQuery extracts and validates a typed ID from a query parameter.
// Returns an error if the parameter is missing, empty, or fails validation.
func parseQuery[T any, PT interface {
	*T
	encoding.TextUnmarshaler
}](r *http.Request, key string) (T, error) {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		var zero T
		return zero, fmt.Errorf("%s query parameter is required", key)
	}
	var id T
	if err := PT(&id).UnmarshalText([]byte(val)); err != nil {
		var zero T
		return zero, fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// optionalQuery extracts an optional typed ID from a query parameter.
// Returns nil if the parameter is missing or empty.
func optionalQuery[T any, PT interface {
	*T
	encoding.TextUnmarshaler
}](r *http.Request, key string) (*T, error) {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		return nil, nil
	}
	var id T
	if err := PT(&id).UnmarshalText([]byte(val)); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &id, nil
}
