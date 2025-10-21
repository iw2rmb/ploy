package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleModRunPrintsBuildGateSummary(t *testing.T) {
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridIDFallbackEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridAPIKeyFallbackEnv, "")
	t.Setenv(gridClientStateEnv, t.TempDir())
	withStubWorkspacePreparer(t)
	buf := &bytes.Buffer{}
	fakeRunner := runnerInvokerFunc(func(ctx context.Context, opts runner.Options) error {
		meta := contracts.BuildGateStageMetadata{
			LogDigest: "bafy-build-log",
			StaticChecks: []contracts.BuildGateStaticCheckReport{
				{
					Language: "go",
					Tool:     "go vet",
					Passed:   false,
					Failures: []contracts.BuildGateStaticCheckFailure{{
						RuleID:   "GOVET001",
						File:     "./main.go",
						Line:     3,
						Column:   1,
						Severity: "error",
						Message:  "unused import",
					}},
				},
				{
					Language: "javascript",
					Tool:     "ESLint",
					Passed:   false,
					Failures: []contracts.BuildGateStaticCheckFailure{{
						RuleID:   "no-console",
						File:     "src/app.js",
						Line:     10,
						Column:   5,
						Severity: "error",
						Message:  "Unexpected console.log",
					}},
				},
				{
					Language: "java",
					Tool:     "Error Prone",
					Passed:   false,
					Failures: []contracts.BuildGateStaticCheckFailure{{
						RuleID:   "DeadException",
						File:     "src/Main.java",
						Line:     12,
						Column:   0,
						Severity: "warning",
						Message:  "Exception created but not thrown",
					}},
				},
			},
			LogFindings: []contracts.BuildGateLogFinding{
				{
					Code:     "kb.git.auth",
					Severity: "error",
					Message:  "Authenticate Git fetch credentials for remote repository access.",
					Evidence: "fatal: unable to access 'https://example.com/repo'",
				},
			},
		}
		checkpoint := contracts.WorkflowCheckpoint{
			SchemaVersion: contracts.SchemaVersion,
			TicketID:      "ticket-123",
			Stage:         "build-gate",
			Status:        contracts.CheckpointStatusCompleted,
			StageMetadata: &contracts.CheckpointStage{
				Name:      "build-gate",
				Kind:      "build-gate",
				Lane:      "go-native",
				Manifest:  contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"},
				Retention: &contracts.CheckpointRetention{Retained: true, TTL: "24h"},
				BuildGate: &meta,
			},
			Artifacts: []contracts.CheckpointArtifact{
				{Name: "diff", ArtifactCID: "bafy-diff", Digest: "sha256:d00d"},
				{Name: "logs", ArtifactCID: "bafy-logs", Digest: "sha256:beef"},
			},
		}
		if err := opts.Events.PublishCheckpoint(ctx, checkpoint); err != nil {
			return err
		}
		return nil
	})

	prevRunner := runnerExecutor
	prevEvents := eventsFactory
	prevManifestLoader := manifestRegistryLoader
	prevManifestDir := manifestConfigDir
	prevLocatorLoader := asterLocatorLoader
	prevAsterDir := asterConfigDir
	prevJobComposerFactory := jobComposerFactory
	prevCacheComposerFactory := cacheComposerFactory
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevEvents
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		jobComposerFactory = prevJobComposerFactory
		cacheComposerFactory = prevCacheComposerFactory
	}()

	bus := contracts.NewInMemoryBus("acme")
	eventsFactory = func(tenant string) (runner.EventsClient, error) {
		return bus, nil
	}
	runnerExecutor = fakeRunner
	manifestRegistryLoader = func(dir string) (runner.ManifestCompiler, error) {
		return &stubManifestCompiler{compiled: defaultManifestPayload()}, nil
	}
	manifestConfigDir = "ignored"
	asterLocatorLoader = func(dir string) (aster.Locator, error) { return &recordingLocator{dir: dir}, nil }
	asterConfigDir = "ignored"
	jobComposerFactory = func() runner.JobComposer { return runner.NewStaticJobComposer() }
	cacheComposerFactory = func() runner.CacheComposer { return runner.NewDefaultCacheComposer() }

	if err := handleModRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Build Gate Summary:") {
		t.Fatalf("expected build gate summary in output, got %q", output)
	}
	if !strings.Contains(output, "- go vet (go): FAILED (1 issue)") {
		t.Fatalf("expected static check summary, got %q", output)
	}
	if !strings.Contains(output, "- ESLint (javascript): FAILED (1 issue)") {
		t.Fatalf("expected eslint summary, got %q", output)
	}
	if !strings.Contains(output, "- Error Prone (java): FAILED (1 issue)") {
		t.Fatalf("expected error prone summary, got %q", output)
	}
	if !strings.Contains(output, "unused import") {
		t.Fatalf("expected failure message in output, got %q", output)
	}
	if !strings.Contains(output, "Unexpected console.log") {
		t.Fatalf("expected eslint message in output, got %q", output)
	}
	if !strings.Contains(output, "Exception created but not thrown") {
		t.Fatalf("expected error prone message in output, got %q", output)
	}
	if !strings.Contains(output, "kb.git.auth") {
		t.Fatalf("expected knowledge base code in output, got %q", output)
	}
	if !strings.Contains(output, "fatal: unable to access") {
		t.Fatalf("expected evidence line in output, got %q", output)
	}
	if !strings.Contains(output, "Log Digest: bafy-build-log") {
		t.Fatalf("expected log digest in output, got %q", output)
	}
	if !strings.Contains(output, "Stage Artifacts:") {
		t.Fatalf("expected stage artifact summary, got %q", output)
	}
	if !strings.Contains(output, "  build-gate (retained ttl=24h):") {
		t.Fatalf("expected retention summary for build-gate, got %q", output)
	}
	if !strings.Contains(output, "    - diff: bafy-diff (sha256:d00d)") {
		t.Fatalf("expected diff artifact summary, got %q", output)
	}
	if !strings.Contains(output, "    - logs: bafy-logs (sha256:beef)") {
		t.Fatalf("expected log artifact summary, got %q", output)
	}
}
