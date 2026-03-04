package types

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// JobStatus is the canonical per-job lifecycle state.
type JobStatus string

const (
	JobStatusCreated   JobStatus = "Created"
	JobStatusQueued    JobStatus = "Queued"
	JobStatusRunning   JobStatus = "Running"
	JobStatusSuccess   JobStatus = "Success"
	JobStatusFail      JobStatus = "Fail"
	JobStatusCancelled JobStatus = "Cancelled"
)

func (s JobStatus) String() string { return string(s) }

func (s JobStatus) Validate() error {
	switch s {
	case JobStatusCreated, JobStatusQueued, JobStatusRunning, JobStatusSuccess, JobStatusFail, JobStatusCancelled:
		return nil
	default:
		return fmt.Errorf("invalid job status %q", s)
	}
}

// ParseJobStatus parses and validates a canonical job status value.
func ParseJobStatus(raw string) (JobStatus, error) {
	status := JobStatus(strings.TrimSpace(raw))
	if err := status.Validate(); err != nil {
		return "", fmt.Errorf("unknown job status: %q", raw)
	}
	return status, nil
}

func (s *JobStatus) Scan(src interface{}) error {
	switch v := src.(type) {
	case []byte:
		*s = JobStatus(v)
	case string:
		*s = JobStatus(v)
	default:
		return fmt.Errorf("unsupported scan type for JobStatus: %T", src)
	}
	if err := s.Validate(); err != nil {
		return fmt.Errorf("unknown JobStatus value: %q", string(*s))
	}
	return nil
}

func (s JobStatus) Value() (driver.Value, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return string(s), nil
}

// RunRepoStatus is the canonical per-repo status within a run.
type RunRepoStatus string

const (
	RunRepoStatusQueued    RunRepoStatus = "Queued"
	RunRepoStatusRunning   RunRepoStatus = "Running"
	RunRepoStatusCancelled RunRepoStatus = "Cancelled"
	RunRepoStatusFail      RunRepoStatus = "Fail"
	RunRepoStatusSuccess   RunRepoStatus = "Success"
)

func (s RunRepoStatus) String() string { return string(s) }

func (s RunRepoStatus) Validate() error {
	switch s {
	case RunRepoStatusQueued, RunRepoStatusRunning, RunRepoStatusCancelled, RunRepoStatusFail, RunRepoStatusSuccess:
		return nil
	default:
		return fmt.Errorf("invalid run repo status %q", s)
	}
}

func (s *RunRepoStatus) Scan(src interface{}) error {
	switch v := src.(type) {
	case []byte:
		*s = RunRepoStatus(v)
	case string:
		*s = RunRepoStatus(v)
	default:
		return fmt.Errorf("unsupported scan type for RunRepoStatus: %T", src)
	}
	if err := s.Validate(); err != nil {
		return fmt.Errorf("unknown RunRepoStatus value: %q", string(*s))
	}
	return nil
}

func (s RunRepoStatus) Value() (driver.Value, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return string(s), nil
}

// RunStatus is the canonical lifecycle status for a run.
type RunStatus string

const (
	RunStatusStarted   RunStatus = "Started"
	RunStatusCancelled RunStatus = "Cancelled"
	RunStatusFinished  RunStatus = "Finished"
)

func (s RunStatus) String() string { return string(s) }

func (s RunStatus) Validate() error {
	switch s {
	case RunStatusStarted, RunStatusCancelled, RunStatusFinished:
		return nil
	default:
		return fmt.Errorf("invalid run status %q", s)
	}
}

func (s *RunStatus) Scan(src interface{}) error {
	switch v := src.(type) {
	case []byte:
		*s = RunStatus(v)
	case string:
		*s = RunStatus(v)
	default:
		return fmt.Errorf("unsupported scan type for RunStatus: %T", src)
	}
	if err := s.Validate(); err != nil {
		return fmt.Errorf("unknown RunStatus value: %q", string(*s))
	}
	return nil
}

func (s RunStatus) Value() (driver.Value, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return string(s), nil
}
