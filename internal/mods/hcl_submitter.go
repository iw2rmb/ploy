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

func (s *DefaultHCLSubmitter) builderClient() BuilderSubmitter {
	if s == nil {
		return nil
	}
	return s.builder
}

// Delegate to package-level indirections to allow unit tests to stub behavior
// without requiring a running Nomad.
func (s *DefaultHCLSubmitter) Validate(hclPath string) error {
	if b := s.builderClient(); b != nil {
		return b.Validate(context.Background(), hclPath)
	}
	return validateJob(hclPath)
}

func (s *DefaultHCLSubmitter) Submit(hclPath string, timeout time.Duration) error {
	return s.SubmitCtx(context.Background(), hclPath, timeout)
}

func (s *DefaultHCLSubmitter) SubmitCtx(ctx context.Context, hclPath string, timeout time.Duration) error {
	if b := s.builderClient(); b != nil {
		return b.Submit(ctx, hclPath, timeout)
	}
	return submitAndWaitTerminal(hclPath, timeout)
}

func (s *DefaultHCLSubmitter) SetBuilder(builder BuilderSubmitter) {
	if s != nil {
		s.builder = builder
	}
}

// NomadBuilderSubmitter delegates to Nomad orchestration helpers for validation and submission.
type NomadBuilderSubmitter struct{}

func NewNomadBuilderSubmitter() *NomadBuilderSubmitter { return &NomadBuilderSubmitter{} }

func (NomadBuilderSubmitter) Validate(ctx context.Context, hclPath string) error {
	_ = ctx
	return validateJob(hclPath)
}

func (NomadBuilderSubmitter) Submit(ctx context.Context, hclPath string, timeout time.Duration) error {
	return submitAndWaitTerminal(hclPath, timeout)
}
