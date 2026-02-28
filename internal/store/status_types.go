package store

import (
	"database/sql/driver"
	"fmt"
)

type JobStatus string

const (
	JobStatusCreated   JobStatus = "Created"
	JobStatusQueued    JobStatus = "Queued"
	JobStatusRunning   JobStatus = "Running"
	JobStatusSuccess   JobStatus = "Success"
	JobStatusFail      JobStatus = "Fail"
	JobStatusCancelled JobStatus = "Cancelled"
)

func (e *JobStatus) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = JobStatus(s)
	case string:
		*e = JobStatus(s)
	default:
		return fmt.Errorf("unsupported scan type for JobStatus: %T", src)
	}
	switch *e {
	case JobStatusCreated, JobStatusQueued, JobStatusRunning, JobStatusSuccess, JobStatusFail, JobStatusCancelled:
		return nil
	default:
		return fmt.Errorf("unknown JobStatus value: %q", string(*e))
	}
}

func (e JobStatus) Value() (driver.Value, error) {
	return string(e), nil
}

type RunRepoStatus string

const (
	RunRepoStatusQueued    RunRepoStatus = "Queued"
	RunRepoStatusRunning   RunRepoStatus = "Running"
	RunRepoStatusCancelled RunRepoStatus = "Cancelled"
	RunRepoStatusFail      RunRepoStatus = "Fail"
	RunRepoStatusSuccess   RunRepoStatus = "Success"
)

func (e *RunRepoStatus) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = RunRepoStatus(s)
	case string:
		*e = RunRepoStatus(s)
	default:
		return fmt.Errorf("unsupported scan type for RunRepoStatus: %T", src)
	}
	switch *e {
	case RunRepoStatusQueued, RunRepoStatusRunning, RunRepoStatusCancelled, RunRepoStatusFail, RunRepoStatusSuccess:
		return nil
	default:
		return fmt.Errorf("unknown RunRepoStatus value: %q", string(*e))
	}
}

func (e RunRepoStatus) Value() (driver.Value, error) {
	return string(e), nil
}

type RunStatus string

const (
	RunStatusStarted   RunStatus = "Started"
	RunStatusCancelled RunStatus = "Cancelled"
	RunStatusFinished  RunStatus = "Finished"
)

func (e *RunStatus) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = RunStatus(s)
	case string:
		*e = RunStatus(s)
	default:
		return fmt.Errorf("unsupported scan type for RunStatus: %T", src)
	}
	switch *e {
	case RunStatusStarted, RunStatusCancelled, RunStatusFinished:
		return nil
	default:
		return fmt.Errorf("unknown RunStatus value: %q", string(*e))
	}
}

func (e RunStatus) Value() (driver.Value, error) {
	return string(e), nil
}
