package nodeagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestMaterializeJavaClasspathInDir_HardFailsWhenSourceArtifactMissing(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	rc := &runController{}
	req := StartRunRequest{
		RunID:   types.NewRunID(),
		JobID:   types.NewJobID(),
		JobType: types.JobTypeMig,
		JavaClasspathContext: &contracts.JavaClasspathClaimContext{
			Required: true,
		},
	}
	err := rc.materializeJavaClasspathInDir(context.Background(), req, t.TempDir())
	if err == nil {
		t.Fatal("materializeJavaClasspathInDir() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "source artifact id is empty") {
		t.Fatalf("error = %v, want source artifact id message", err)
	}
}

func TestMaterializeJavaClasspathInDir_RestoresFromArtifactWhenRunCacheMissing(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	artifactID := "11111111-1111-1111-1111-111111111111"
	classpathBytes := []byte("/root/.m2/repository/org/example/lib/1.0.0/lib-1.0.0.jar\n")
	bundle := mustTarGzEntries(t, map[string][]byte{
		"out/java.classpath": classpathBytes,
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/artifacts/"+artifactID {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("download"); got != "true" {
			t.Fatalf("download query = %q, want true", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bundle)
	}))
	defer server.Close()

	rc := newTestController(t, newAgentConfig(server.URL))
	runID := types.NewRunID()
	req := StartRunRequest{
		RunID:   runID,
		JobID:   types.NewJobID(),
		JobType: types.JobTypeMig,
		JavaClasspathContext: &contracts.JavaClasspathClaimContext{
			Required:         true,
			SourceArtifactID: artifactID,
		},
	}

	inDir := t.TempDir()
	if err := rc.materializeJavaClasspathInDir(context.Background(), req, inDir); err != nil {
		t.Fatalf("materializeJavaClasspathInDir() error = %v", err)
	}

	gotIn, err := os.ReadFile(filepath.Join(inDir, sbomJavaClasspathFileName))
	if err != nil {
		t.Fatalf("read /in java classpath: %v", err)
	}
	if string(gotIn) != string(classpathBytes) {
		t.Fatalf("/in java classpath mismatch: got %q want %q", gotIn, classpathBytes)
	}
	gotRunCache, err := os.ReadFile(runJavaClasspathPath(runID))
	if err != nil {
		t.Fatalf("read run cache java classpath: %v", err)
	}
	if string(gotRunCache) != string(classpathBytes) {
		t.Fatalf("run cache java classpath mismatch: got %q want %q", gotRunCache, classpathBytes)
	}
}

func TestCaptureJavaClasspathAfterStandardJob_PersistsForHookMigHeal(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	testCases := []struct {
		name    string
		jobType types.JobType
	}{
		{name: "hook", jobType: types.JobTypeHook},
		{name: "mig", jobType: types.JobTypeMig},
		{name: "heal", jobType: types.JobTypeHeal},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rc := &runController{}
			runID := types.NewRunID()
			inDir := t.TempDir()
			outDir := t.TempDir()
			classpathBytes := []byte("/cache/lib.jar\n")
			if err := os.WriteFile(filepath.Join(inDir, sbomJavaClasspathFileName), classpathBytes, 0o644); err != nil {
				t.Fatalf("write /in java classpath: %v", err)
			}
			req := StartRunRequest{
				RunID:   runID,
				JobID:   types.NewJobID(),
				JobType: tc.jobType,
				JavaClasspathContext: &contracts.JavaClasspathClaimContext{
					Required: true,
				},
			}

			if err := rc.captureJavaClasspathAfterStandardJob(req, inDir, outDir); err != nil {
				t.Fatalf("captureJavaClasspathAfterStandardJob() error = %v", err)
			}

			runCachePath := runJavaClasspathPath(runID)
			if _, err := os.Stat(runCachePath); err != nil {
				t.Fatalf("expected run cache java classpath at %q: %v", runCachePath, err)
			}
			outPath := filepath.Join(outDir, sbomJavaClasspathFileName)
			if _, err := os.Stat(outPath); err != nil {
				t.Fatalf("expected /out java classpath at %q: %v", outPath, err)
			}
		})
	}
}

func TestPrepareAndCaptureGateJavaClasspathInput(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	rc := &runController{}
	runID := types.NewRunID()
	req := StartRunRequest{
		RunID:   runID,
		JobID:   types.NewJobID(),
		JobType: types.JobTypePreGate,
		JavaClasspathContext: &contracts.JavaClasspathClaimContext{
			Required: true,
		},
	}
	classpathBytes := []byte("/cache/gradle/lib.jar\n")
	sourcePath := runJavaClasspathPath(runID)
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(sourcePath), err)
	}
	if err := os.WriteFile(sourcePath, classpathBytes, 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", sourcePath, err)
	}

	workspace := t.TempDir()
	if err := rc.prepareGateJavaClasspathInput(context.Background(), req, workspace); err != nil {
		t.Fatalf("prepareGateJavaClasspathInput() error = %v", err)
	}
	inPath := filepath.Join(workspace, step.BuildGateWorkspaceInDir, sbomJavaClasspathFileName)
	if _, err := os.Stat(inPath); err != nil {
		t.Fatalf("expected gate /in java classpath at %q: %v", inPath, err)
	}

	if err := rc.captureJavaClasspathAfterGateJob(req, workspace); err != nil {
		t.Fatalf("captureJavaClasspathAfterGateJob() error = %v", err)
	}
	outPath := filepath.Join(workspace, step.BuildGateWorkspaceOutDir, sbomJavaClasspathFileName)
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("expected gate /out java classpath at %q: %v", outPath, err)
	}
}
