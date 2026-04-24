package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCommit_IgnoresRootTargetDirectory(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepoWithSingleCommit(t)
	ctx := context.Background()
	_ = prepareRepoWithTargetIgnore(t, ctx, repoDir)

	if err := os.MkdirAll(filepath.Join(repoDir, "target"), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "target", "generated.txt"), []byte("generated\n"), 0o644); err != nil {
		t.Fatalf("write target/generated.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "main.txt"), []byte("mutated\n"), 0o644); err != nil {
		t.Fatalf("write main.txt: %v", err)
	}

	created, err := EnsureCommit(ctx, repoDir, "Test User", "test@example.com", "mutate main")
	if err != nil {
		t.Fatalf("EnsureCommit() error = %v", err)
	}
	if !created {
		t.Fatalf("EnsureCommit() created = false, want true")
	}
}
