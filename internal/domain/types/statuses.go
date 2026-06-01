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
	JobStatusError     JobStatus = "Error"
	JobStatusCancelled JobStatus = "Cancelled"
)

func (s JobStatus) String() string { return string(s) }

func (s JobStatus) Validate() error {
	switch s {
	case JobStatusCreated, JobStatusQueued, JobStatusRunning, JobStatusSuccess, JobStatusFail, JobStatusError, JobStatusCancelled:
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

// RunStatus is the canonical lifecycle status for one repo execution.
type RunStatus string

const (
	RunStatusQueued    RunStatus = "Queued"
	RunStatusRunning   RunStatus = "Running"
	RunStatusCancelled RunStatus = "Cancelled"
	RunStatusFail      RunStatus = "Fail"
	RunStatusSuccess   RunStatus = "Success"
)

func (s RunStatus) String() string { return string(s) }

func (s RunStatus) Validate() error {
	switch s {
	case RunStatusQueued, RunStatusRunning, RunStatusCancelled, RunStatusFail, RunStatusSuccess:
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

// WaveStatus is the canonical lifecycle status for a launch wave.
type WaveStatus string

const (
	WaveStatusStarted   WaveStatus = "Started"
	WaveStatusCancelled WaveStatus = "Cancelled"
	WaveStatusFinished  WaveStatus = "Finished"
)

func (s WaveStatus) String() string { return string(s) }

func (s WaveStatus) Validate() error {
	switch s {
	case WaveStatusStarted, WaveStatusCancelled, WaveStatusFinished:
		return nil
	default:
		return fmt.Errorf("invalid wave status %q", s)
	}
}

func (s *WaveStatus) Scan(src interface{}) error {
	switch v := src.(type) {
	case []byte:
		*s = WaveStatus(v)
	case string:
		*s = WaveStatus(v)
	default:
		return fmt.Errorf("unsupported scan type for WaveStatus: %T", src)
	}
	if err := s.Validate(); err != nil {
		return fmt.Errorf("unknown WaveStatus value: %q", string(*s))
	}
	return nil
}

func (s WaveStatus) Value() (driver.Value, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return string(s), nil
}
