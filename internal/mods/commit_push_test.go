package mods

import (
	"context"
	"errors"
	"testing"
)

func TestRunCommitStep_NoChangesHeadMoved(t *testing.T) {
	r, _ := NewTransflowRunner(&TransflowConfig{ID: "w"}, t.TempDir())
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
	r, _ := NewTransflowRunner(&TransflowConfig{ID: "w"}, t.TempDir())
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
	cfg := &TransflowConfig{ID: "w", TargetRepo: "https://example/repo", BaseRef: "main", Steps: []TransflowStep{{Type: "orw-apply", ID: "s", Recipes: []string{"r"}, RecipeGroup: "g", RecipeArtifact: "a", RecipeVersion: "v"}}}
	r, _ := NewTransflowRunner(cfg, t.TempDir())
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

func TestRunPushStep_Error(t *testing.T) {
	cfg := &TransflowConfig{ID: "w", TargetRepo: "https://example/repo", BaseRef: "main", Steps: []TransflowStep{{Type: "orw-apply", ID: "s", Recipes: []string{"r"}, RecipeGroup: "g", RecipeArtifact: "a", RecipeVersion: "v"}}}
	r, _ := NewTransflowRunner(cfg, t.TempDir())
	r.SetGitOperations(&errPushGitOps{})
	if err := r.runPushStep(context.Background(), "/repo", "branch"); err == nil {
		t.Fatalf("expected push error")
	}
}

func TestRunPushWithEvents_Error(t *testing.T) {
	cfg := &TransflowConfig{ID: "w", TargetRepo: "https://example/repo", BaseRef: "main", Steps: []TransflowStep{{Type: "orw-apply", ID: "s", Recipes: []string{"r"}, RecipeGroup: "g", RecipeArtifact: "a", RecipeVersion: "v"}}}
	r, _ := NewTransflowRunner(cfg, t.TempDir())
	r.SetGitOperations(&errPushGitOps{})
	sr, err := runPushWithEvents(r, context.Background(), "/repo", "branch")
	if err == nil || sr.Success {
		t.Fatalf("expected push error with failed step, got err=%v sr=%+v", err, sr)
	}
}
