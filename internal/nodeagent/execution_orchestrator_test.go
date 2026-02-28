package nodeagent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestPopulateHealingInDirCopiesGateLog verifies that populateHealingInDir copies
// the persisted gate log into the healing job's /in directory as build-gate.log.
func TestPopulateHealingInDirCopiesGateLog(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-copy-log")

	// Seed the persisted gate log.
	runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	srcPath := filepath.Join(runDir, "build-gate-first.log")
	const contents = "trimmed failure log\n"
	if err := os.WriteFile(srcPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write src gate log: %v", err)
	}

	inDir := t.TempDir()

	if err := rc.populateHealingInDir(runID, inDir); err != nil {
		t.Fatalf("populateHealingInDir error: %v", err)
	}

	destPath := filepath.Join(inDir, "build-gate.log")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read /in/build-gate.log: %v", err)
	}
	if string(data) != contents {
		t.Fatalf("healing /in/build-gate.log = %q, want %q", string(data), contents)
	}
}

func TestModStepIndexFromJobName_MultiStep(t *testing.T) {
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

			got, err := modStepIndexFromJobName(tc.jobName, tc.steps)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for job_name=%q", tc.jobName)
				}
				return
			}
			if err != nil {
				t.Fatalf("modStepIndexFromJobName(%q,%d) returned error: %v", tc.jobName, tc.steps, err)
			}
			if got != tc.want {
				t.Fatalf("modStepIndexFromJobName(%q,%d)=%d want %d", tc.jobName, tc.steps, got, tc.want)
			}
		})
	}
}
