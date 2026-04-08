package nodeagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestWriteCanonicalSBOMOutput_WritesCanonicalDocument(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	sbomPath := preGateSBOMOutPath(types.RunID("run-sbom-hook"))

	if err := writeCanonicalSBOMOutput(sbomPath); err != nil {
		t.Fatalf("writeCanonicalSBOMOutput: %v", err)
	}

	sbomRaw, err := os.ReadFile(sbomPath)
	if err != nil {
		t.Fatalf("read canonical sbom output: %v", err)
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
}

func TestMaterializePreGateSBOMForGate_UsesSBOMOutputWhenNoHooks(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-sbom-no-hooks")
	if err := writeCanonicalSBOMOutput(preGateSBOMOutPath(runID)); err != nil {
		t.Fatalf("writeCanonicalSBOMOutput: %v", err)
	}

	workspace := t.TempDir()
	if err := materializePreGateSBOMForGate(runID, nil, workspace); err != nil {
		t.Fatalf("materializePreGateSBOMForGate: %v", err)
	}

	sbomOutPath := filepath.Join(workspace, step.BuildGateWorkspaceOutDir, preGateCanonicalSBOMFileName)
	if _, err := os.Stat(sbomOutPath); err != nil {
		t.Fatalf("expected canonical sbom output at %s: %v", sbomOutPath, err)
	}
}

func TestMaterializePreGateSBOMForGate_UsesLastHookOutput(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-sbom-with-hooks")
	if err := writeCanonicalSBOMOutput(preGateSBOMOutPath(runID)); err != nil {
		t.Fatalf("writeCanonicalSBOMOutput: %v", err)
	}

	// Simulate completed hook jobs where each writes /out/sbom.spdx.json.
	lastHookOut := preGateHookOutPath(runID, 1)
	wantSnapshot := []byte(`{"spdxVersion":"SPDX-2.3","name":"hook-output"}`)
	if err := os.MkdirAll(filepath.Dir(lastHookOut), 0o755); err != nil {
		t.Fatalf("mkdir hook out dir: %v", err)
	}
	if err := os.WriteFile(lastHookOut, wantSnapshot, 0o644); err != nil {
		t.Fatalf("write hook out snapshot: %v", err)
	}

	workspace := t.TempDir()
	if err := materializePreGateSBOMForGate(runID, []string{"./hooks/a.yaml", "./hooks/b.yaml"}, workspace); err != nil {
		t.Fatalf("materializePreGateSBOMForGate: %v", err)
	}

	sbomOutPath := filepath.Join(workspace, step.BuildGateWorkspaceOutDir, preGateCanonicalSBOMFileName)
	got, err := os.ReadFile(sbomOutPath)
	if err != nil {
		t.Fatalf("read materialized sbom output: %v", err)
	}
	if string(got) != string(wantSnapshot) {
		t.Fatalf("materialized snapshot mismatch: got %q want %q", string(got), string(wantSnapshot))
	}
}

func TestPreGateHookIndexFromJobName(t *testing.T) {
	idx, err := preGateHookIndexFromJobName("pre-gate-hook-001", 2)
	if err != nil {
		t.Fatalf("preGateHookIndexFromJobName: %v", err)
	}
	if idx != 1 {
		t.Fatalf("hook index = %d, want 1", idx)
	}

	if _, err := preGateHookIndexFromJobName("hook-1", 2); err == nil {
		t.Fatal("expected prefix validation error")
	}
	if _, err := preGateHookIndexFromJobName("pre-gate-hook-2", 2); err == nil {
		t.Fatal("expected out-of-range validation error")
	}
}

func TestAddHookRuntimeMetadata_EmitsHookOnceKeys(t *testing.T) {
	builder := types.NewRunStatsBuilder()
	addHookRuntimeMetadata(builder, &contracts.HookRuntimeDecision{
		HookHash:           "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HookShouldRun:      false,
		HookOnceSkipMarked: true,
	})
	stats := builder.MustBuild()

	var decoded map[string]any
	if err := json.Unmarshal(stats, &decoded); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	meta, ok := decoded["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing or wrong type: %T", decoded["metadata"])
	}
	if got := meta["hook_hash"]; got != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("metadata.hook_hash=%v, want 64-char hash", got)
	}
	if got := meta["hook_should_run"]; got != "false" {
		t.Fatalf("metadata.hook_should_run=%v, want false", got)
	}
	if got := meta["hook_once_skip_marked"]; got != "true" {
		t.Fatalf("metadata.hook_once_skip_marked=%v, want true", got)
	}
}

func TestExecuteHookJob_SkipsHookWorkWhenHookShouldRunFalse(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-hook-skip")
	jobID := types.NewJobID()
	server, cap := newStatusCaptureServer(t, jobID.String())
	rc := newTestController(t, newAgentConfig(server.URL))

	input := []byte(`{"spdxVersion":"SPDX-2.3","name":"input"}`)
	inputPath := preGateSBOMOutPath(runID)
	if err := os.MkdirAll(filepath.Dir(inputPath), 0o755); err != nil {
		t.Fatalf("mkdir input dir: %v", err)
	}
	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatalf("write input snapshot: %v", err)
	}

	rc.executeHookJob(context.Background(), StartRunRequest{
		RunID:   runID,
		JobID:   jobID,
		JobType: types.JobTypeHook,
		JobName: "pre-gate-hook-000",
		HookRuntime: &contracts.HookRuntimeDecision{
			HookHash:      strings.Repeat("a", 64),
			HookShouldRun: false,
		},
		TypedOptions: RunOptions{
			Hooks: []string{"./hooks/lint.yaml"},
		},
	})

	if got := cap.Status; got != types.JobStatusSuccess.String() {
		t.Fatalf("status=%q, want %q", got, types.JobStatusSuccess.String())
	}

	inPath := preGateHookInPath(runID, 0)
	if _, err := os.Stat(inPath); !os.IsNotExist(err) {
		t.Fatalf("expected skip path to avoid /in materialization, err=%v", err)
	}

	outPath := preGateHookOutPath(runID, 0)
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read /out snapshot: %v", err)
	}
	if string(out) != string(input) {
		t.Fatalf("/out snapshot mismatch: got %q want %q", string(out), string(input))
	}
}
