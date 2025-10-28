package stepworker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/controlplane"
	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	stepruntime "github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

func TestExecutorProducesAssignmentResult(t *testing.T) {
	t.Helper()
	now := time.Date(2025, 10, 28, 12, 0, 0, 0, time.UTC)
	streams := logstream.NewHub(logstream.Options{HistorySize: 8})
	runner := &fakeRunner{
		result: stepruntime.Result{
			ContainerID: "ctr-1",
			ExitCode:    0,
			DiffArtifact: stepruntime.PublishedArtifact{
				CID:    "bafy-diff",
				Digest: "sha256:diff",
				Size:   1024,
			},
			LogArtifact: stepruntime.PublishedArtifact{
				CID:    "bafy-logs",
				Digest: "sha256:logs",
				Size:   2048,
			},
			ShiftArtifact: stepruntime.PublishedArtifact{
				CID:    "bafy-shift",
				Digest: "sha256:shift",
				Size:   512,
			},
			ShiftReport: stepruntime.ShiftResult{
				Passed:   true,
				Duration: 5 * time.Second,
			},
			Retained:     true,
			RetentionTTL: "24h",
		},
	}
	exec := Executor{
		runner:  runner,
		streams: streams,
		now:     func() time.Time { return now },
	}

	manifest := contracts.StepManifest{
		ID:    "mods-apply",
		Name:  "Mods Apply",
		Image: "ghcr.io/ploy/mods/apply:latest",
		Inputs: []contracts.StepInput{
			{Name: "overlay", Mode: contracts.StepInputModeReadWrite, DiffCID: "bafy-overlay", MountPath: "/workspace"},
		},
		Retention: contracts.StepRetentionSpec{
			RetainContainer: true,
			TTL:             "24h",
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	assign := controlplane.Assignment{
		ID:       "job-123",
		Runtime:  "local",
		Metadata: map[string]string{"step_manifest": string(data)},
	}

	result, execErr := exec.Execute(context.Background(), assign)
	if execErr != nil {
		t.Fatalf("Execute error: %v", execErr)
	}
	if result.State != string(scheduler.JobStateSucceeded) {
		t.Fatalf("unexpected state: %s", result.State)
	}
	if result.Artifacts["diff_cid"] != "bafy-diff" {
		t.Fatalf("unexpected diff cid: %s", result.Artifacts["diff_cid"])
	}
	if bundle, ok := result.Bundles["logs"]; !ok || bundle.CID != "bafy-logs" {
		t.Fatalf("unexpected bundle: %+v", result.Bundles["logs"])
	}
	if result.Shift == nil || result.Shift.Result != scheduler.ShiftResultPassed {
		t.Fatalf("expected shift result passed, got %+v", result.Shift)
	}
	if result.Artifacts["shift_report_cid"] != "bafy-shift" {
		t.Fatalf("expected shift report artifact cid, got %s", result.Artifacts["shift_report_cid"])
	}
	if bundle, ok := result.Bundles["shift_report"]; !ok || bundle.CID != "bafy-shift" {
		t.Fatalf("expected shift report bundle, got %+v", result.Bundles["shift_report"])
	}
	if result.Retention == nil || result.Retention.Bundle != "bafy-logs" {
		t.Fatalf("expected retention bundle cid, got %+v", result.Retention)
	}
	if result.Retention.Expires != now.Add(24*time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("unexpected retention expiry: %s", result.Retention.Expires)
	}
	if result.Inspection {
		t.Fatalf("inspection should be false on success")
	}
}

func TestExecutorShiftFailureSignalsInspection(t *testing.T) {
	t.Helper()
	now := time.Date(2025, 10, 28, 12, 0, 0, 0, time.UTC)
	streams := logstream.NewHub(logstream.Options{})
	runner := &fakeRunner{
		result: stepruntime.Result{
			ContainerID: "ctr-2",
			ExitCode:    1,
			LogArtifact: stepruntime.PublishedArtifact{
				CID:    "bafy-logs",
				Digest: "sha256:logs",
				Size:   2048,
			},
			ShiftArtifact: stepruntime.PublishedArtifact{
				CID:    "bafy-shift",
				Digest: "sha256:shift",
				Size:   512,
			},
			ShiftReport: stepruntime.ShiftResult{
				Passed:   false,
				Message:  "shift failed",
				Duration: 3 * time.Second,
			},
			Retained:     true,
			RetentionTTL: "12h",
		},
		err: stepruntime.ErrShiftFailed,
	}
	exec := Executor{
		runner:  runner,
		streams: streams,
		now:     func() time.Time { return now },
	}

	manifest := contracts.StepManifest{
		ID:    "mods-plan",
		Name:  "Mods Plan",
		Image: "ghcr.io/ploy/mods/plan:latest",
		Inputs: []contracts.StepInput{
			{Name: "overlay", Mode: contracts.StepInputModeReadWrite, DiffCID: "bafy-overlay", MountPath: "/workspace"},
		},
		Retention: contracts.StepRetentionSpec{
			RetainContainer: true,
			TTL:             "12h",
		},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	assign := controlplane.Assignment{
		ID:       "job-456",
		Runtime:  "local",
		Metadata: map[string]string{"step_manifest": string(data)},
	}

	result, execErr := exec.Execute(context.Background(), assign)
	if execErr == nil {
		t.Fatal("expected execute error for shift failure")
	}
	if result.State != string(scheduler.JobStateFailed) {
		t.Fatalf("expected failed state, got %s", result.State)
	}
	if result.Error == nil || result.Error.Message == "" {
		t.Fatalf("expected job error message, got %+v", result.Error)
	}
	if !result.Inspection {
		t.Fatalf("expected inspection flag true")
	}
	if result.Retention == nil || result.Retention.TTL != "12h" {
		t.Fatalf("expected retention metadata, got %+v", result.Retention)
	}
	if result.Artifacts["shift_report_cid"] != "bafy-shift" {
		t.Fatalf("expected shift report artifact in failure path")
	}
}

type fakeRunner struct {
	result stepruntime.Result
	err    error
	calls  int
	last   stepruntime.Request
}

func (f *fakeRunner) Run(ctx context.Context, req stepruntime.Request) (stepruntime.Result, error) {
	_ = ctx
	f.calls++
	f.last = req
	return f.result, f.err
}
