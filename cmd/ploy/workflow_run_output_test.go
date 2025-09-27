package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestHandleWorkflowRunPrintsBuildGateSummary(t *testing.T) {
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
				BuildGate: &meta,
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
	prevLaneLoader := laneRegistryLoader
	prevLaneDir := laneConfigDir
	defer func() {
		runnerExecutor = prevRunner
		eventsFactory = prevEvents
		manifestRegistryLoader = prevManifestLoader
		manifestConfigDir = prevManifestDir
		asterLocatorLoader = prevLocatorLoader
		asterConfigDir = prevAsterDir
		laneRegistryLoader = prevLaneLoader
		laneConfigDir = prevLaneDir
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
	laneRegistryLoader = func(dir string) (laneRegistry, error) {
		return &fakeLaneRegistry{description: lanes.Description{Lane: lanes.Spec{Name: "go-native", CacheNamespace: "go-native"}, CacheKey: "stub-cache"}}, nil
	}
	laneConfigDir = "ignored"

	if err := handleWorkflowRun([]string{"--tenant", "acme", "--ticket", "ticket-123"}, buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Build Gate Summary:") {
		t.Fatalf("expected build gate summary in output, got %q", output)
	}
	if !strings.Contains(output, "- go vet (go): FAILED (1 issue)") {
		t.Fatalf("expected static check summary, got %q", output)
	}
	if !strings.Contains(output, "unused import") {
		t.Fatalf("expected failure message in output, got %q", output)
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
}
