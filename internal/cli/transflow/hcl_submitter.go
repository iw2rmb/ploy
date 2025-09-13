package transflow

import (
	"time"

	"github.com/iw2rmb/ploy/internal/orchestration"
)

// HCLSubmitter abstracts HCL job validation and submission to ease testing.
type HCLSubmitter interface {
	Validate(hclPath string) error
	Submit(hclPath string, timeout time.Duration) error
}

// DefaultHCLSubmitter delegates to orchestration helpers.
type DefaultHCLSubmitter struct{}

func (DefaultHCLSubmitter) Validate(hclPath string) error { return orchestration.ValidateJob(hclPath) }
func (DefaultHCLSubmitter) Submit(hclPath string, timeout time.Duration) error {
	return orchestration.SubmitAndWaitTerminal(hclPath, timeout)
}
