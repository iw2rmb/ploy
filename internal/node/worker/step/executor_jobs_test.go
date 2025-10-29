package stepworker

import (
    "context"
    "encoding/json"
    "testing"
    "time"

    "github.com/iw2rmb/ploy/internal/api/controlplane"
    "github.com/iw2rmb/ploy/internal/node/jobs"
    "github.com/iw2rmb/ploy/internal/node/logstream"
    "github.com/iw2rmb/ploy/internal/workflow/contracts"
    stepruntime "github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// TestExecutorPublishesJobLifecycle verifies tracker sees running→terminal.
func TestExecutorPublishesJobLifecycle(t *testing.T) {
    t.Helper()

    streams := logstream.NewHub(logstream.Options{})
    tracker := jobs.NewStore(jobs.Options{Capacity: 4})

    runner := &fakeRunner{result: stepruntime.Result{ExitCode: 0}}
    exec, err := New(Options{Runner: runner, Streams: streams, Jobs: tracker})
    if err != nil {
        t.Fatalf("New executor: %v", err)
    }

    manifest := contracts.StepManifest{
        ID:    "run",
        Name:  "Run",
        Image: "alpine:3.19",
        Inputs: []contracts.StepInput{{
            Name:      "overlay",
            MountPath: "/workspace",
            Mode:      contracts.StepInputModeReadWrite,
            Hydration: &contracts.StepInputHydration{Repo: &contracts.RepoMaterialization{URL: "https://example.com/repo.git", TargetRef: "main"}},
        }},
    }
    data, _ := json.Marshal(manifest)
    assign := controlplane.Assignment{ID: "job-xyz", Metadata: map[string]string{"step_manifest": string(data)}}

    if _, err := exec.Execute(context.Background(), assign); err != nil {
        t.Fatalf("Execute: %v", err)
    }

    // Running snapshot should have been recorded before completion too; we just assert terminal.
    rec, ok := tracker.Get("job-xyz")
    if !ok {
        t.Fatalf("expected job-xyz in tracker")
    }
    if rec.State != jobs.StateSucceeded {
        t.Fatalf("state=%s want succeeded", rec.State)
    }
    if rec.CompletedAt.IsZero() {
        t.Fatalf("expected completion timestamp")
    }
}

// TestExecutorPublishesFailure ensures error transitions are captured.
func TestExecutorPublishesFailure(t *testing.T) {
    t.Helper()

    streams := logstream.NewHub(logstream.Options{})
    tracker := jobs.NewStore(jobs.Options{})

    runner := &fakeRunner{result: stepruntime.Result{ExitCode: 1}, err: stepruntime.ErrShiftFailed}
    exec, err := New(Options{Runner: runner, Streams: streams, Jobs: tracker})
    if err != nil {
        t.Fatalf("New executor: %v", err)
    }

    manifest := contracts.StepManifest{
        ID:    "fail",
        Name:  "Fail",
        Image: "alpine:3.19",
        Inputs: []contracts.StepInput{{
            Name:      "overlay",
            MountPath: "/workspace",
            Mode:      contracts.StepInputModeReadWrite,
            Hydration: &contracts.StepInputHydration{Repo: &contracts.RepoMaterialization{URL: "https://example.com/repo.git", TargetRef: "main"}},
        }},
    }
    data, _ := json.Marshal(manifest)
    assign := controlplane.Assignment{ID: "job-bad", Metadata: map[string]string{"step_manifest": string(data)}}

    _, _ = exec.Execute(context.Background(), assign)
    // Give a tiny tick for timestamps
    time.Sleep(1 * time.Millisecond)
    rec, ok := tracker.Get("job-bad")
    if !ok || rec.State != jobs.StateFailed {
        t.Fatalf("expected failed record, got %+v ok=%v", rec, ok)
    }
    if rec.Error == "" {
        t.Fatalf("expected error message recorded")
    }
}
