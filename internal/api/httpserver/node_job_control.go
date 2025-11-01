package httpserver

import "errors"

// ErrJobNotFound signals the job id is unknown to the node.
var ErrJobNotFound = errors.New("node: job not found")

// ErrJobNotRunning signals the job exists but is not currently running.
var ErrJobNotRunning = errors.New("node: job not running")

// JobController exposes controls for node-local jobs.
type JobController interface {
	// Cancel requests cancellation of a running job.
	Cancel(jobID string) error
}
