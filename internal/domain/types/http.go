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
	return RunID(val), nil
}

// ParseJobIDParam extracts and validates a JobID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseJobIDParam(r *http.Request, key string) (JobID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	return JobID(val), nil
}

// ParseNodeIDParam extracts and validates a NodeID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseNodeIDParam(r *http.Request, key string) (NodeID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	return NodeID(val), nil
}

// ParseModIDParam extracts and validates a ModID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseModIDParam(r *http.Request, key string) (ModID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	return ModID(val), nil
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
	return ModRepoID(val), nil
}

// ParseSpecIDParam extracts and validates a SpecID from a path parameter.
// Returns 400 Bad Request error if the parameter is missing or empty.
func ParseSpecIDParam(r *http.Request, key string) (SpecID, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	return SpecID(val), nil
}

// OptionalRunIDParam extracts an optional RunID from a path parameter.
// Returns nil if the parameter is missing or empty.
func OptionalRunIDParam(r *http.Request, key string) *RunID {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return nil
	}
	id := RunID(val)
	return &id
}

// OptionalJobIDParam extracts an optional JobID from a path parameter.
// Returns nil if the parameter is missing or empty.
func OptionalJobIDParam(r *http.Request, key string) *JobID {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return nil
	}
	id := JobID(val)
	return &id
}

// OptionalNodeIDParam extracts an optional NodeID from a path parameter.
// Returns nil if the parameter is missing or empty.
func OptionalNodeIDParam(r *http.Request, key string) *NodeID {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return nil
	}
	id := NodeID(val)
	return &id
}

// ParseRunIDQuery extracts and validates a RunID from a query parameter.
// Returns error if the parameter is missing or empty.
func ParseRunIDQuery(r *http.Request, key string) (RunID, error) {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		return "", fmt.Errorf("%s query parameter is required", key)
	}
	return RunID(val), nil
}

// OptionalRunIDQuery extracts an optional RunID from a query parameter.
// Returns nil if the parameter is missing or empty.
func OptionalRunIDQuery(r *http.Request, key string) *RunID {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		return nil
	}
	id := RunID(val)
	return &id
}

// OptionalJobIDQuery extracts an optional JobID from a query parameter.
// Returns nil if the parameter is missing or empty.
func OptionalJobIDQuery(r *http.Request, key string) *JobID {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		return nil
	}
	id := JobID(val)
	return &id
}
