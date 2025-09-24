package mods

import (
	"context"
	"time"
)

// HCLSubmitter abstracts HCL job validation and submission to ease testing.
type HCLSubmitter interface {
	Validate(hclPath string) error
	Submit(hclPath string, timeout time.Duration) error
	SubmitCtx(ctx context.Context, hclPath string, timeout time.Duration) error
}

// DefaultHCLSubmitter delegates to orchestration helpers.
type DefaultHCLSubmitter struct {
	builder BuilderSubmitter
}

// NewDefaultHCLSubmitter creates a submitter that optionally delegates to a builder submitter.
func NewDefaultHCLSubmitter(builder BuilderSubmitter) *DefaultHCLSubmitter {
	return &DefaultHCLSubmitter{builder: builder}
}

// Delegate to package-level indirections to allow unit tests to stub behavior
// without requiring a running Nomad.
func (DefaultHCLSubmitter) Validate(hclPath string) error { return validateJob(hclPath) }
func (DefaultHCLSubmitter) Submit(hclPath string, timeout time.Duration) error {
	return submitAndWaitTerminal(hclPath, timeout)
}
func (DefaultHCLSubmitter) SubmitCtx(ctx context.Context, hclPath string, timeout time.Duration) error {
	_ = ctx
	return submitAndWaitTerminal(hclPath, timeout)
}
