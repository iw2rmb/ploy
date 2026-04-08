package nodeagent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestSchedulePreGateSBOMAndHooks_WritesCanonicalSBOMAndHookInputs(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	workspace := t.TempDir()
	req := StartRunRequest{
		RunID:   types.RunID("run-sbom-hook"),
		JobID:   types.JobID("job-sbom-hook"),
		JobType: types.JobTypePreGate,
		TypedOptions: RunOptions{
			Hooks: []string{
				"./hooks/lint.yaml",
				"https://hooks.example.com/sbom-policy/v1.yaml",
			},
		},
	}

	rc := &runController{}
	if err := rc.schedulePreGateSBOMAndHooks(req, workspace); err != nil {
		t.Fatalf("schedulePreGateSBOMAndHooks: %v", err)
	}

	sbomOutPath := filepath.Join(workspace, step.BuildGateWorkspaceOutDir, preGateCanonicalSBOMFileName)
	sbomRaw, err := os.ReadFile(sbomOutPath)
	if err != nil {
		t.Fatalf("read pre-gate sbom output: %v", err)
	}

	var doc struct {
		SPDXVersion string `json:"spdxVersion"`
		Packages    []any  `json:"packages"`
	}
	if err := json.Unmarshal(sbomRaw, &doc); err != nil {
		t.Fatalf("unmarshal sbom output: %v", err)
	}
	if got, want := doc.SPDXVersion, "SPDX-2.3"; got != want {
		t.Fatalf("spdxVersion = %q, want %q", got, want)
	}
	if len(doc.Packages) != 0 {
		t.Fatalf("packages len = %d, want 0", len(doc.Packages))
	}

	for i := range req.TypedOptions.Hooks {
		inPath := filepath.Join(
			runCacheDir(req.RunID),
			"pre-gate-hooks",
			fmt.Sprintf("%03d", i),
			"in",
			preGateCanonicalSBOMFileName,
		)
		hookInRaw, err := os.ReadFile(inPath)
		if err != nil {
			t.Fatalf("read hook input snapshot for hook[%d]: %v", i, err)
		}
		if string(hookInRaw) != string(sbomRaw) {
			t.Fatalf("hook[%d] /in/%s does not match canonical sbom output", i, preGateCanonicalSBOMFileName)
		}
	}
}

func TestSchedulePreGateSBOMAndHooks_NoHooksWritesCanonicalSBOMOnly(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	workspace := t.TempDir()
	req := StartRunRequest{
		RunID:   types.RunID("run-sbom-no-hooks"),
		JobID:   types.JobID("job-sbom-no-hooks"),
		JobType: types.JobTypePreGate,
	}

	rc := &runController{}
	if err := rc.schedulePreGateSBOMAndHooks(req, workspace); err != nil {
		t.Fatalf("schedulePreGateSBOMAndHooks: %v", err)
	}

	sbomOutPath := filepath.Join(workspace, step.BuildGateWorkspaceOutDir, preGateCanonicalSBOMFileName)
	if _, err := os.Stat(sbomOutPath); err != nil {
		t.Fatalf("expected canonical sbom output at %s: %v", sbomOutPath, err)
	}

	hooksRoot := filepath.Join(runCacheDir(req.RunID), "pre-gate-hooks")
	if _, err := os.Stat(hooksRoot); !os.IsNotExist(err) {
		t.Fatalf("expected no pre-gate hook staging dir, stat err=%v", err)
	}
}
