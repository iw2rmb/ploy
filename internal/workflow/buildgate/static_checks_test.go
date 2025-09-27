package buildgate

import (
	"context"
	"errors"
	"testing"
)

type fakeStaticCheckAdapter struct {
	meta     StaticCheckAdapterMetadata
	failures []StaticCheckFailure
	err      error
}

func (a *fakeStaticCheckAdapter) Metadata() StaticCheckAdapterMetadata {
	return a.meta
}

func (a *fakeStaticCheckAdapter) Run(ctx context.Context, req StaticCheckRequest) (StaticCheckResult, error) {
	if a.err != nil {
		return StaticCheckResult{}, a.err
	}
	failures := make([]StaticCheckFailure, len(a.failures))
	copy(failures, a.failures)
	return StaticCheckResult{Failures: failures}, nil
}

func TestStaticCheckRegistryExecuteAppliesManifestOverrides(t *testing.T) {
	registry := NewStaticCheckRegistry()
	goAdapter := &fakeStaticCheckAdapter{
		meta: StaticCheckAdapterMetadata{
			Language:        "go",
			Tool:            "go-vet",
			DefaultSeverity: SeverityError,
		},
		failures: []StaticCheckFailure{{
			RuleID:   "vet001",
			File:     "main.go",
			Line:     12,
			Column:   4,
			Severity: "warning",
			Message:  "unused variable",
		}},
	}
	if err := registry.Register(goAdapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}
	jsAdapter := &fakeStaticCheckAdapter{
		meta: StaticCheckAdapterMetadata{
			Language:        "javascript",
			Tool:            "eslint",
			DefaultSeverity: SeverityWarning,
		},
	}
	if err := registry.Register(jsAdapter); err != nil {
		t.Fatalf("register js adapter: %v", err)
	}

	spec := StaticCheckSpec{
		LaneDefaults: map[string]StaticCheckLaneConfig{
			"go": {
				Enabled:        true,
				FailOnSeverity: SeverityError,
			},
			"javascript": {
				Enabled:        true,
				FailOnSeverity: SeverityError,
			},
		},
		Manifest: StaticCheckManifest{
			Languages: map[string]StaticCheckManifestLanguage{
				"go": {
					FailOnSeverity: "warning",
				},
				"javascript": {
					Enabled: boolPtr(false),
				},
			},
		},
	}

	reports, err := registry.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("execute registry: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected single report after manifest disables javascript, got %d", len(reports))
	}
	report := reports[0]
	if report.Language != "golang" {
		t.Fatalf("expected golang report, got %q", report.Language)
	}
	if report.Tool != "go-vet" {
		t.Fatalf("expected tool go-vet, got %q", report.Tool)
	}
	if report.Passed {
		t.Fatalf("expected report to fail due to warning threshold")
	}
	if len(report.Failures) != 1 {
		t.Fatalf("expected single failure, got %d", len(report.Failures))
	}
	failure := report.Failures[0]
	if failure.Severity != "warning" {
		t.Fatalf("expected failure severity warning, got %q", failure.Severity)
	}
}

func TestStaticCheckRegistryExecuteRespectsSeverityThreshold(t *testing.T) {
	registry := NewStaticCheckRegistry()
	adapter := &fakeStaticCheckAdapter{
		meta: StaticCheckAdapterMetadata{
			Language:        "go",
			Tool:            "go-vet",
			DefaultSeverity: SeverityError,
		},
		failures: []StaticCheckFailure{{
			RuleID:   "vet001",
			File:     "main.go",
			Line:     5,
			Severity: "warning",
			Message:  "unused variable",
		}},
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	spec := StaticCheckSpec{
		LaneDefaults: map[string]StaticCheckLaneConfig{
			"go": {
				Enabled:        true,
				FailOnSeverity: SeverityError,
			},
		},
	}

	reports, err := registry.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("execute registry: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected single report, got %d", len(reports))
	}
	if !reports[0].Passed {
		t.Fatalf("expected report to pass when only warnings present and threshold is error")
	}
}

func TestStaticCheckRegistryExecuteErrorsWhenAdapterMissing(t *testing.T) {
	registry := NewStaticCheckRegistry()

	spec := StaticCheckSpec{
		LaneDefaults: map[string]StaticCheckLaneConfig{
			"go": {
				Enabled:        true,
				FailOnSeverity: SeverityError,
			},
		},
	}

	_, err := registry.Execute(context.Background(), spec)
	if !errors.Is(err, ErrStaticCheckAdapterNotFound) {
		t.Fatalf("expected ErrStaticCheckAdapterNotFound, got %v", err)
	}
}

func TestStaticCheckRegistrySkipLanguages(t *testing.T) {
	registry := NewStaticCheckRegistry()
	adapter := &fakeStaticCheckAdapter{
		meta: StaticCheckAdapterMetadata{
			Language:        "go",
			Tool:            "go-vet",
			DefaultSeverity: SeverityError,
		},
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	spec := StaticCheckSpec{
		LaneDefaults: map[string]StaticCheckLaneConfig{
			"go": {
				Enabled: true,
			},
		},
		SkipLanguages: []string{"GO"},
	}

	reports, err := registry.Execute(context.Background(), spec)
	if err != nil {
		t.Fatalf("execute registry: %v", err)
	}
	if len(reports) != 0 {
		t.Fatalf("expected no reports when language skipped, got %d", len(reports))
	}
}

func boolPtr(v bool) *bool {
	return &v
}
