package nodeagent

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestMigStepIndexFromJobName_MultiStep(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		jobName string
		steps   int
		want    int
		wantErr bool
	}{
		{name: "step0", jobName: "mig-0", steps: 3, want: 0},
		{name: "step2", jobName: "mig-2", steps: 3, want: 2},
		{name: "single step non-indexed", jobName: "mig", steps: 1, want: 0},
		{name: "invalid prefix", jobName: "pre-gate", steps: 2, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := migStepIndexFromJobName(tc.jobName, tc.steps)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for job_name=%q", tc.jobName)
				}
				return
			}
			if err != nil {
				t.Fatalf("migStepIndexFromJobName(%q,%d) returned error: %v", tc.jobName, tc.steps, err)
			}
			if got != tc.want {
				t.Fatalf("migStepIndexFromJobName(%q,%d)=%d want %d", tc.jobName, tc.steps, got, tc.want)
			}
		})
	}
}

func TestMaterializeGateSBOMForGate_UsesPostCycleSnapshot(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-cycle-materialize")
	postSnapshot := []byte(`{"spdxVersion":"SPDX-2.3","name":"post-gate-cycle"}`)
	postSBOMOut := gateCycleSBOMOutPath(runID, postGateCycleName)
	if err := os.MkdirAll(filepath.Dir(postSBOMOut), 0o755); err != nil {
		t.Fatalf("mkdir post sbom out: %v", err)
	}
	if err := os.WriteFile(postSBOMOut, postSnapshot, 0o644); err != nil {
		t.Fatalf("write post sbom snapshot: %v", err)
	}

	postWorkspace := t.TempDir()
	if err := materializeGateSBOMForGate(runID, postGateCycleName, postWorkspace); err != nil {
		t.Fatalf("materialize post-gate sbom: %v", err)
	}
	postOutPath := filepath.Join(postWorkspace, step.BuildGateWorkspaceOutDir, preGateCanonicalSBOMFileName)
	postOut, err := os.ReadFile(postOutPath)
	if err != nil {
		t.Fatalf("read post-gate out snapshot: %v", err)
	}
	if string(postOut) != string(postSnapshot) {
		t.Fatalf("post-gate snapshot mismatch: got %q want %q", string(postOut), string(postSnapshot))
	}

}

func TestMaterializeGateSBOMForGate_RequiresCycleSnapshot(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-cycle-snapshot-required")
	preSnapshot := []byte(`{"spdxVersion":"SPDX-2.3","name":"pre-gate-cycle"}`)
	preGateOut := preGateSBOMOutPath(runID)
	if err := os.MkdirAll(filepath.Dir(preGateOut), 0o755); err != nil {
		t.Fatalf("mkdir pre-gate sbom dir: %v", err)
	}
	if err := os.WriteFile(preGateOut, preSnapshot, 0o644); err != nil {
		t.Fatalf("write pre-gate sbom snapshot: %v", err)
	}

	postWorkspace := t.TempDir()
	err := materializeGateSBOMForGate(runID, postGateCycleName, postWorkspace)
	if err == nil {
		t.Fatalf("expected error when post-gate snapshot is missing")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected os.IsNotExist for missing post-gate snapshot, got: %v", err)
	}

}
