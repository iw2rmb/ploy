package buildgate_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
)

type runnerSandboxExecutor struct {
	result buildgate.SandboxBuildResult
	err    error
}

func (s *runnerSandboxExecutor) Execute(ctx context.Context, spec buildgate.SandboxSpec) (buildgate.SandboxBuildResult, error) {
	if s.err != nil {
		return buildgate.SandboxBuildResult{}, s.err
	}
	return s.result, nil
}

type recordingStaticCheckAdapter struct {
	metadata buildgate.StaticCheckAdapterMetadata
	result   buildgate.StaticCheckResult
}

func (a *recordingStaticCheckAdapter) Metadata() buildgate.StaticCheckAdapterMetadata {
	return a.metadata
}

func (a *recordingStaticCheckAdapter) Run(ctx context.Context, req buildgate.StaticCheckRequest) (buildgate.StaticCheckResult, error) {
	return a.result, nil
}

type stubArtifactFetcher struct {
	data []byte
	err  error
}

func (s stubArtifactFetcher) Fetch(ctx context.Context, ref buildgate.ArtifactReference) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]byte(nil), s.data...), nil
}

func TestRunnerRequiresSandbox(t *testing.T) {
	t.Parallel()

	runner := &buildgate.Runner{}
	_, err := runner.Run(context.Background(), buildgate.RunSpec{})
	if !errors.Is(err, buildgate.ErrRunnerSandboxMissing) {
		t.Fatalf("expected sandbox missing error, got %v", err)
	}
}

func TestRunnerAggregatesSandboxStaticChecksAndLogs(t *testing.T) {
	t.Parallel()

	sandbox := buildgate.NewSandboxRunner(&runnerSandboxExecutor{result: buildgate.SandboxBuildResult{
		Success:   true,
		CacheHit:  true,
		LogDigest: " sha256:sandbox ",
	}}, buildgate.SandboxRunnerOptions{Clock: fixedClock{now: time.Unix(0, 0)}})

	registry := buildgate.NewStaticCheckRegistry()
	adapter := &recordingStaticCheckAdapter{
		metadata: buildgate.StaticCheckAdapterMetadata{
			Language:        "golang",
			Tool:            "govet",
			DefaultSeverity: buildgate.SeverityError,
		},
		result: buildgate.StaticCheckResult{
			Failures: []buildgate.StaticCheckFailure{{
				RuleID:   " GOVET001 ",
				File:     " main.go ",
				Line:     12,
				Column:   2,
				Severity: "ERROR",
				Message:  " pointer dereference ",
			}},
		},
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	logContent := "fatal error: unable to access repository\nno space left on device\n"
	fetcher := stubArtifactFetcher{data: []byte(logContent)}
	ingestor := &buildgate.LogIngestor{
		Retriever: &buildgate.LogRetriever{
			Primary:       fetcher,
			PrimarySource: buildgate.LogSourceStub,
		},
		Parser: buildgate.NewDefaultLogParser(),
	}

	runner := &buildgate.Runner{
		Sandbox:      sandbox,
		StaticChecks: registry,
		Logs:         ingestor,
	}

	spec := buildgate.RunSpec{
		Sandbox: buildgate.SandboxSpec{Command: []string{"go", "build"}},
		StaticChecks: &buildgate.StaticCheckSpec{
			LaneDefaults: map[string]buildgate.StaticCheckLaneConfig{
				"golang": {
					Enabled:        true,
					FailOnSeverity: buildgate.SeverityError,
				},
			},
		},
		LogArtifact: &buildgate.ArtifactReference{CID: "cid123", Description: "builder.log"},
	}

	result, err := runner.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result.Sandbox.Status != buildgate.SandboxStatusSucceeded {
		t.Fatalf("unexpected sandbox status: %v", result.Sandbox.Status)
	}
	if !result.Sandbox.CacheHit {
		t.Fatalf("expected sandbox cache hit")
	}

	if len(result.StaticChecks) != 1 {
		t.Fatalf("expected one static check report, got %d", len(result.StaticChecks))
	}
	report := result.StaticChecks[0]
	if report.Language != "golang" {
		t.Fatalf("unexpected language: %s", report.Language)
	}
	if report.Tool != "govet" {
		t.Fatalf("unexpected tool: %s", report.Tool)
	}
	if report.Passed {
		t.Fatalf("expected report to fail due to severity threshold")
	}
	if len(report.Failures) != 1 {
		t.Fatalf("expected one failure, got %d", len(report.Failures))
	}
	failure := report.Failures[0]
	if failure.RuleID != "GOVET001" {
		t.Fatalf("unexpected rule id: %q", failure.RuleID)
	}
	if failure.File != "main.go" {
		t.Fatalf("unexpected file: %q", failure.File)
	}
	if failure.Line != 12 {
		t.Fatalf("unexpected line: %d", failure.Line)
	}
	if failure.Column != 2 {
		t.Fatalf("unexpected column: %d", failure.Column)
	}
	if failure.Severity != "error" {
		t.Fatalf("unexpected severity: %s", failure.Severity)
	}
	if failure.Message != "pointer dereference" {
		t.Fatalf("unexpected message: %q", failure.Message)
	}

	if result.Log == nil {
		t.Fatalf("expected log ingestion result")
	}
	digest := sha256.Sum256([]byte(logContent))
	expectedDigest := "sha256:" + hex.EncodeToString(digest[:])
	if result.Log.Digest != expectedDigest {
		t.Fatalf("unexpected log digest: %s", result.Log.Digest)
	}
	if len(result.Log.Findings) == 0 {
		t.Fatalf("expected log findings")
	}

	if result.Metadata.LogDigest != expectedDigest {
		t.Fatalf("metadata digest mismatch: %s", result.Metadata.LogDigest)
	}
	if len(result.Metadata.StaticChecks) != 1 {
		t.Fatalf("metadata static checks missing")
	}
	if len(result.Metadata.LogFindings) != len(result.Log.Findings) {
		t.Fatalf("metadata log findings mismatch")
	}
}

func TestRunnerRequiresStaticCheckRegistryWhenSpecProvided(t *testing.T) {
	t.Parallel()

	sandbox := buildgate.NewSandboxRunner(&runnerSandboxExecutor{result: buildgate.SandboxBuildResult{Success: true}}, buildgate.SandboxRunnerOptions{})
	runner := &buildgate.Runner{Sandbox: sandbox}
	spec := buildgate.RunSpec{
		Sandbox: buildgate.SandboxSpec{Command: []string{"go", "build"}},
		StaticChecks: &buildgate.StaticCheckSpec{
			LaneDefaults: map[string]buildgate.StaticCheckLaneConfig{
				"golang": {Enabled: true},
			},
		},
	}
	if _, err := runner.Run(context.Background(), spec); !errors.Is(err, buildgate.ErrRunnerStaticCheckRegistryMissing) {
		t.Fatalf("expected static check registry missing error, got %v", err)
	}
}

func TestRunnerRequiresLogIngestorWhenArtifactProvided(t *testing.T) {
	t.Parallel()

	sandbox := buildgate.NewSandboxRunner(&runnerSandboxExecutor{result: buildgate.SandboxBuildResult{Success: true}}, buildgate.SandboxRunnerOptions{})
	registry := buildgate.NewStaticCheckRegistry()
	adapter := &recordingStaticCheckAdapter{
		metadata: buildgate.StaticCheckAdapterMetadata{
			Language:        "golang",
			Tool:            "govet",
			DefaultSeverity: buildgate.SeverityError,
		},
		result: buildgate.StaticCheckResult{},
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	runner := &buildgate.Runner{
		Sandbox:      sandbox,
		StaticChecks: registry,
	}
	spec := buildgate.RunSpec{
		Sandbox: buildgate.SandboxSpec{Command: []string{"go", "build"}},
		StaticChecks: &buildgate.StaticCheckSpec{
			LaneDefaults: map[string]buildgate.StaticCheckLaneConfig{
				"golang": {Enabled: true},
			},
		},
		LogArtifact: &buildgate.ArtifactReference{CID: "cid123"},
	}
	if _, err := runner.Run(context.Background(), spec); !errors.Is(err, buildgate.ErrRunnerLogIngestorMissing) {
		t.Fatalf("expected log ingestor missing error, got %v", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
