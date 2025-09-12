package transflow

import (
    "context"
    "errors"
    "testing"
    "time"
)

func TestRunApplyAndBuildWithEvents_Success(t *testing.T) {
    r := &TransflowRunner{config: &TransflowConfig{ID: "wf"}}
    stepStart := time.Now().Add(-500 * time.Millisecond)
    sr, err := runApplyAndBuildWithEvents(context.Background(), r, "/repo", "/diff.patch", "apply", stepStart,
        func(ctx context.Context, repo, diff string) error { return nil })
    if err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
    if !sr.Success || sr.StepID != "apply" {
        t.Fatalf("unexpected step result: %+v", sr)
    }
    if sr.Message == "" { t.Fatal("missing message") }
    if sr.Duration <= 0 { t.Fatal("duration not set") }
}

func TestRunApplyAndBuildWithEvents_Failure(t *testing.T) {
    r := &TransflowRunner{config: &TransflowConfig{ID: "wf"}}
    want := errors.New("boom")
    sr, err := runApplyAndBuildWithEvents(context.Background(), r, "/repo", "/diff.patch", "apply", time.Now(),
        func(ctx context.Context, repo, diff string) error { return want })
    if err == nil || !errors.Is(err, want) {
        t.Fatalf("expected err %v, got %v", want, err)
    }
    if sr.Success {
        t.Fatalf("expected failure, got success: %+v", sr)
    }
}

