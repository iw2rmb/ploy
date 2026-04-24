package nodeagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestEnsureRequiredJavaClasspathShare_FailsWhenShareClasspathMissing(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	req := StartRunRequest{
		RunID:   types.NewRunID(),
		RepoID:  types.NewMigRepoID(),
		JobID:   types.NewJobID(),
		JobType: types.JobTypeMig,
		JavaClasspathContext: &contracts.JavaClasspathClaimContext{
			Required: true,
		},
	}
	rc := &runController{}

	err := rc.ensureRequiredJavaClasspathShare(req)
	if err == nil {
		t.Fatal("ensureRequiredJavaClasspathShare() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "validate") || !strings.Contains(err.Error(), "java.classpath") {
		t.Fatalf("error = %q, want share classpath validation error", err)
	}
}

func TestEnsureRequiredJavaClasspathShare_ValidClasspathInShare(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	sharePath := runRepoJavaClasspathPath(runID, repoID)
	if err := os.MkdirAll(filepath.Dir(sharePath), 0o755); err != nil {
		t.Fatalf("mkdir share parent: %v", err)
	}
	wantClasspath := []byte("/root/.m2/repository/org/example/lib/1.0.0/lib-1.0.0.jar\n")
	if err := os.WriteFile(sharePath, wantClasspath, 0o644); err != nil {
		t.Fatalf("write share classpath: %v", err)
	}

	req := StartRunRequest{
		RunID:   runID,
		RepoID:  repoID,
		JobID:   types.NewJobID(),
		JobType: types.JobTypeMig,
		JavaClasspathContext: &contracts.JavaClasspathClaimContext{
			Required: true,
		},
	}
	rc := &runController{}
	if err := rc.ensureRequiredJavaClasspathShare(req); err != nil {
		t.Fatalf("ensureRequiredJavaClasspathShare() error = %v", err)
	}
}

func TestEnsureRequiredJavaClasspathShare_RejectsNonPortableGradleClasspathFromShare(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	sharePath := runRepoJavaClasspathPath(runID, repoID)
	if err := os.MkdirAll(filepath.Dir(sharePath), 0o755); err != nil {
		t.Fatalf("mkdir share parent: %v", err)
	}
	if err := os.WriteFile(sharePath, []byte("/home/gradle/.gradle/caches/modules-2/files-2.1/a/b/c/lib.jar\n"), 0o644); err != nil {
		t.Fatalf("write share classpath: %v", err)
	}

	req := StartRunRequest{
		RunID:   runID,
		RepoID:  repoID,
		JobID:   types.NewJobID(),
		JobType: types.JobTypeMig,
		JavaClasspathContext: &contracts.JavaClasspathClaimContext{
			Required: true,
		},
	}
	rc := &runController{}
	err := rc.ensureRequiredJavaClasspathShare(req)
	if err == nil {
		t.Fatal("ensureRequiredJavaClasspathShare() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "non-portable gradle cache path") {
		t.Fatalf("error = %q, want non-portable gradle cache path", err)
	}
}

func TestEnsureRequiredJavaClasspathShare_HealDoesNotRequireClasspath(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	req := StartRunRequest{
		RunID:   types.NewRunID(),
		RepoID:  types.NewMigRepoID(),
		JobID:   types.NewJobID(),
		JobType: types.JobTypeHeal,
		JavaClasspathContext: &contracts.JavaClasspathClaimContext{
			Required: true,
		},
	}

	rc := &runController{}
	if err := rc.ensureRequiredJavaClasspathShare(req); err != nil {
		t.Fatalf("ensureRequiredJavaClasspathShare() error = %v", err)
	}
}
