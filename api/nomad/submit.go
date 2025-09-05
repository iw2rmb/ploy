package nomad

import (
	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
)

// Submit submits a job using the unified internal orchestration client.
func Submit(jobPath string) error {
	return orchestration.Submit(jobPath)
}

// SubmitBasic delegates to the same unified submission path.
func SubmitBasic(jobPath string) error {
	return orchestration.Submit(jobPath)
}
