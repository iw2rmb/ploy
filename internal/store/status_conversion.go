package store

import (
	"fmt"
)

// ConvertToJobStatus converts a job status string to the canonical JobStatus.
// Only v1 canonical values are accepted; non-canonical aliases are rejected.
func ConvertToJobStatus(status string) (JobStatus, error) {
	switch status {
	case "Created":
		return JobStatusCreated, nil
	case "Queued":
		return JobStatusQueued, nil
	case "Running":
		return JobStatusRunning, nil
	case "Success":
		return JobStatusSuccess, nil
	case "Fail":
		return JobStatusFail, nil
	case "Cancelled":
		return JobStatusCancelled, nil
	default:
		return "", fmt.Errorf("unknown job status: %q", status)
	}
}
