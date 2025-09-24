package mods

import (
	"context"
	"testing"
	"time"
)

type fakeBuilderSubmitter struct {
	submitCalled bool
}

func (f *fakeBuilderSubmitter) Validate(ctx context.Context, hclPath string) error {
	return nil
}

func (f *fakeBuilderSubmitter) Submit(ctx context.Context, hclPath string, timeout time.Duration) error {
	f.submitCalled = true
	return nil
}

func TestDefaultHCLSubmitterUsesBuilderSubmitter(t *testing.T) {
	fake := &fakeBuilderSubmitter{}
	s := NewDefaultHCLSubmitter(fake)

	origSubmit := submitAndWaitTerminal
	defer func() { submitAndWaitTerminal = origSubmit }()
	submitAndWaitTerminal = func(string, time.Duration) error {
		t.Fatalf("submitAndWaitTerminal should not be called when builder submitter is present")
		return nil
	}

	if err := s.SubmitCtx(context.Background(), "job.hcl", time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.submitCalled {
		t.Fatalf("expected builder submitter to be invoked")
	}
}
