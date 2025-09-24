package mods

import (
	"context"
	"errors"
	"testing"

	gitapi "github.com/iw2rmb/ploy/api/git"
)

func TestRunCommitStep_NoChangesHeadMoved(t *testing.T) {
	r, _ := NewModRunner(&ModConfig{ID: "w"}, t.TempDir())
	// Override getHeadHashFn and hasRepoChangesFn
	oldGet := getHeadHashFn
	oldHas := hasRepoChangesFn
	getHeadHashFn = func(string) (string, error) { return "after", nil }
	hasRepoChangesFn = func(string) (bool, error) { return false, nil }
	defer func() { getHeadHashFn = oldGet; hasRepoChangesFn = oldHas }()

	committed, msg, err := r.runCommitStep(context.Background(), "/repo", "before")
	if err != nil || !committed || msg == "" {
		t.Fatalf("unexpected result: committed=%v msg=%q err=%v", committed, msg, err)
	}
}

func TestRunCommitStep_NoChangesHeadSame(t *testing.T) {
	r, _ := NewModRunner(&ModConfig{ID: "w"}, t.TempDir())
	oldGet := getHeadHashFn
	oldHas := hasRepoChangesFn
	getHeadHashFn = func(string) (string, error) { return "same", nil }
	hasRepoChangesFn = func(string) (bool, error) { return false, nil }
	defer func() { getHeadHashFn = oldGet; hasRepoChangesFn = oldHas }()

	committed, _, err := r.runCommitStep(context.Background(), "/repo", "same")
	if err == nil || committed {
		t.Fatalf("expected error and no commit, got committed=%v err=%v", committed, err)
	}
}

type errGitOps struct{ MockGitOperations }

func (e *errGitOps) CommitChanges(ctx context.Context, repoPath, message string) error {
	return errors.New("commit error")
}

func TestRunCommitStep_DoCommitError(t *testing.T) {
	cfg := &ModConfig{ID: "w", TargetRepo: "https://example/repo", BaseRef: "main", Steps: []ModStep{{Type: "orw-apply", ID: "s", Recipes: []RecipeEntry{{Name: "r", Coords: RecipeCoordinates{Group: "g", Artifact: "a", Version: "v"}}}}}}
	r, _ := NewModRunner(cfg, t.TempDir())
	r.SetGitOperations(&errGitOps{})
	oldHas := hasRepoChangesFn
	hasRepoChangesFn = func(string) (bool, error) { return true, nil }
	defer func() { hasRepoChangesFn = oldHas }()
	committed, _, err := r.runCommitStep(context.Background(), "/repo", "before")
	if err == nil || committed {
		t.Fatalf("expected commit error, got committed=%v err=%v", committed, err)
	}
}

type errPushGitOps struct{ MockGitOperations }

func (e *errPushGitOps) PushBranch(ctx context.Context, repoPath, remoteURL, branchName string) error {
	return errors.New("push error")
}

func (e *errPushGitOps) PushBranchAsync(ctx context.Context, repoPath, remoteURL, branchName string) *gitapi.Operation {
	e.PushError = errors.New("push error")
	return e.MockGitOperations.PushBranchAsync(ctx, repoPath, remoteURL, branchName)
}

func TestRunPushStep_Error(t *testing.T) {
	cfg := &ModConfig{ID: "w", TargetRepo: "https://example/repo", BaseRef: "main", Steps: []ModStep{{Type: "orw-apply", ID: "s", Recipes: []RecipeEntry{{Name: "r", Coords: RecipeCoordinates{Group: "g", Artifact: "a", Version: "v"}}}}}}
	r, _ := NewModRunner(cfg, t.TempDir())
	r.SetGitOperations(&errPushGitOps{})
	if err := r.runPushStep(context.Background(), "/repo", "branch"); err == nil {
		t.Fatalf("expected push error")
	}
}

func TestRunPushWithEvents_Error(t *testing.T) {
	cfg := &ModConfig{ID: "w", TargetRepo: "https://example/repo", BaseRef: "main", Steps: []ModStep{{Type: "orw-apply", ID: "s", Recipes: []RecipeEntry{{Name: "r", Coords: RecipeCoordinates{Group: "g", Artifact: "a", Version: "v"}}}}}}
	r, _ := NewModRunner(cfg, t.TempDir())
	r.SetGitOperations(&errPushGitOps{})
	sr, err := runPushWithEvents(r, context.Background(), "/repo", "branch")
	if err == nil || sr.Success {
		t.Fatalf("expected push error with failed step, got err=%v sr=%+v", err, sr)
	}
}

type panicGitOpsOnPush struct {
	MockGitOperations
	t *testing.T
}

func (p *panicGitOpsOnPush) PushBranchAsync(ctx context.Context, repoPath, remoteURL, branchName string) *gitapi.Operation {
	p.t.Fatalf("unexpected call to gitOps.PushBranchAsync with branch=%s", branchName)
	return nil
}

type fakeGitPushOp struct {
	events chan gitapi.Event
	err    error
}

func (f *fakeGitPushOp) Events() <-chan gitapi.Event { return f.events }
func (f *fakeGitPushOp) Err() error                  { return f.err }

type fakeGitPusher struct {
	called bool
}

func (f *fakeGitPusher) PushBranchAsync(ctx context.Context, repoPath, remoteURL, branchName string) GitPushOperation {
	f.called = true
	events := make(chan gitapi.Event, 1)
	events <- gitapi.Event{Type: gitapi.EventCompleted, Message: "done"}
	close(events)
	return &fakeGitPushOp{events: events}
}

func TestRunPushStepUsesGitPusher(t *testing.T) {
	cfg := &ModConfig{ID: "w", TargetRepo: "https://example/repo", BaseRef: "main", Steps: []ModStep{{ID: "s", Type: string(StepTypeORWApply), Recipes: []RecipeEntry{{Name: "r", Coords: RecipeCoordinates{Group: "g", Artifact: "a", Version: "v"}}}}}}
	r, _ := NewModRunner(cfg, t.TempDir())

	panicOps := &panicGitOpsOnPush{t: t}
	r.SetGitOperations(panicOps)

	fakePusher := &fakeGitPusher{}
	r.SetGitPusher(fakePusher)

	if err := r.runPushStep(context.Background(), "/repo", "branch"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fakePusher.called {
		t.Fatalf("expected git pusher to be invoked")
	}
}
