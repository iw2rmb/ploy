package types

import (
	"fmt"
	"net/http"
	"strings"
)

// ParseRunIDParam extracts and validates a RunID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseRunIDParam(r *http.Request, key string) (RunID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	var id RunID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// ParseJobIDParam extracts and validates a JobID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseJobIDParam(r *http.Request, key string) (JobID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	var id JobID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// ParseNodeIDParam extracts and validates a NodeID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseNodeIDParam(r *http.Request, key string) (NodeID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	var id NodeID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// ParseModIDParam extracts and validates a ModID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseModIDParam(r *http.Request, key string) (ModID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	var id ModID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// ParseModRefParam extracts and validates a ModRef from a path parameter.
// Returns 400 Bad Request error if the parameter is missing, empty, or invalid.
func ParseModRefParam(r *http.Request, key string) (ModRef, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	ref := ModRef(val)
	if err := ref.Validate(); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	return ref, nil
}

// ParseModRepoIDParam extracts and validates a ModRepoID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseModRepoIDParam(r *http.Request, key string) (ModRepoID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	var id ModRepoID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// ParseSpecIDParam extracts and validates a SpecID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseSpecIDParam(r *http.Request, key string) (SpecID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	var id SpecID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// OptionalRunIDParam extracts an optional RunID from a path parameter.
// Returns nil if the parameter is missing or empty.
func OptionalRunIDParam(r *http.Request, key string) (*RunID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return nil, nil
	}
	var id RunID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &id, nil
}

// OptionalJobIDParam extracts an optional JobID from a path parameter.
// Returns nil if the parameter is missing or empty.
func OptionalJobIDParam(r *http.Request, key string) (*JobID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return nil, nil
	}
	var id JobID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &id, nil
}

// OptionalNodeIDParam extracts an optional NodeID from a path parameter.
// Returns nil if the parameter is missing or empty.
func OptionalNodeIDParam(r *http.Request, key string) (*NodeID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return nil, nil
	}
	var id NodeID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &id, nil
}

// ParseRunIDQuery extracts and validates a RunID from a query parameter.
// Returns error if the parameter is missing or empty.
func ParseRunIDQuery(r *http.Request, key string) (RunID, error) {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		return "", fmt.Errorf("%s query parameter is required", key)
	}
	var id RunID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// OptionalRunIDQuery extracts an optional RunID from a query parameter.
// Returns nil if the parameter is missing or empty.
func OptionalRunIDQuery(r *http.Request, key string) (*RunID, error) {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		return nil, nil
	}
	var id RunID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &id, nil
}

// OptionalJobIDQuery extracts an optional JobID from a query parameter.
// Returns nil if the parameter is missing or empty.
func OptionalJobIDQuery(r *http.Request, key string) (*JobID, error) {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		return nil, nil
	}
	var id JobID
	if err := id.UnmarshalText([]byte(val)); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &id, nil
}
