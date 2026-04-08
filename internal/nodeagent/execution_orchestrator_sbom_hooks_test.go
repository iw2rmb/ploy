package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func writeCanonicalSBOMFixture(t *testing.T, path string, name string) []byte {
	t.Helper()

	doc := canonicalSBOMDocument{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              name,
		DocumentNamespace: "https://ploy.dev/sbom/tests",
		CreationInfo: canonicalCreationInfo{
			Created:  "1970-01-01T00:00:00Z",
			Creators: []string{"Tool: sbom-test"},
		},
		Packages: []canonicalSBOMPackage{},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal canonical sbom fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir canonical sbom fixture dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write canonical sbom fixture: %v", err)
	}
	return raw
}

func TestMaterializeValidatedSBOMOutput_WritesCanonicalDocument(t *testing.T) {
	outDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)

	rawDeps := strings.Join([]string{
		"[INFO]    com.fasterxml.jackson.core:jackson-databind:jar:2.17.2:compile",
		"\\--- org.apache.commons:commons-lang3:3.17.0",
		"noise line that should be ignored",
	}, "\n")
	if err := os.WriteFile(filepath.Join(outDir, sbomDependencyOutputFileName), []byte(rawDeps), 0o644); err != nil {
		t.Fatalf("write raw dependency output: %v", err)
	}

	if err := materializeValidatedSBOMOutput(outDir, snapshotPath); err != nil {
		t.Fatalf("materializeValidatedSBOMOutput: %v", err)
	}

	canonicalPath := filepath.Join(outDir, preGateCanonicalSBOMFileName)
	canonicalRaw, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical sbom output: %v", err)
	}
	if err := validateCanonicalSBOMDocument(canonicalRaw); err != nil {
		t.Fatalf("validate canonical sbom output: %v", err)
	}

	var doc canonicalSBOMDocument
	if err := json.Unmarshal(canonicalRaw, &doc); err != nil {
		t.Fatalf("unmarshal canonical sbom: %v", err)
	}
	if got, want := doc.SPDXVersion, "SPDX-2.3"; got != want {
		t.Fatalf("spdxVersion = %q, want %q", got, want)
	}
	if len(doc.Packages) != 2 {
		t.Fatalf("packages len = %d, want 2", len(doc.Packages))
	}
	if got, want := doc.Packages[0].Name, "com.fasterxml.jackson.core:jackson-databind"; got != want {
		t.Fatalf("packages[0].name = %q, want %q", got, want)
	}
	if got, want := doc.Packages[0].VersionInfo, "2.17.2"; got != want {
		t.Fatalf("packages[0].versionInfo = %q, want %q", got, want)
	}
	if got, want := doc.Packages[1].Name, "org.apache.commons:commons-lang3"; got != want {
		t.Fatalf("packages[1].name = %q, want %q", got, want)
	}
	if got, want := doc.Packages[1].VersionInfo, "3.17.0"; got != want {
		t.Fatalf("packages[1].versionInfo = %q, want %q", got, want)
	}

	stagedRaw, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read staged sbom snapshot: %v", err)
	}
	if string(stagedRaw) != string(canonicalRaw) {
		t.Fatalf("staged snapshot mismatch with canonical output")
	}
}

func TestMaterializeValidatedSBOMOutput_ErrorsWhenDependencyOutputMissing(t *testing.T) {
	outDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)

	err := materializeValidatedSBOMOutput(outDir, snapshotPath)
	if err == nil {
		t.Fatal("expected error for missing dependency output")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
	if !strings.Contains(err.Error(), sbomDependencyOutputFileName) {
		t.Fatalf("error = %q, want mention of %s", err, sbomDependencyOutputFileName)
	}
}

func TestValidateCanonicalSBOMDocument_RejectsMalformedPayload(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
	}{
		{
			name: "invalid json",
			raw:  []byte(`{`),
		},
		{
			name: "missing packages array",
			raw: []byte(`{
				"spdxVersion":"SPDX-2.3",
				"dataLicense":"CC0-1.0",
				"SPDXID":"SPDXRef-DOCUMENT",
				"name":"x",
				"documentNamespace":"https://ploy.dev/sbom/tests",
				"creationInfo":{"created":"1970-01-01T00:00:00Z","creators":["Tool: sbom-test"]}
			}`),
		},
		{
			name: "missing package version",
			raw: []byte(`{
				"spdxVersion":"SPDX-2.3",
				"dataLicense":"CC0-1.0",
				"SPDXID":"SPDXRef-DOCUMENT",
				"name":"x",
				"documentNamespace":"https://ploy.dev/sbom/tests",
				"creationInfo":{"created":"1970-01-01T00:00:00Z","creators":["Tool: sbom-test"]},
				"packages":[{"name":"pkg"}]
			}`),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateCanonicalSBOMDocument(tc.raw)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMaterializePreGateSBOMForGate_UsesSBOMOutputWhenNoHooks(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-sbom-no-hooks")
	wantSnapshot := writeCanonicalSBOMFixture(t, preGateSBOMOutPath(runID), "pre-gate-cycle")

	workspace := t.TempDir()
	if err := materializePreGateSBOMForGate(runID, nil, workspace); err != nil {
		t.Fatalf("materializePreGateSBOMForGate: %v", err)
	}

	sbomOutPath := filepath.Join(workspace, step.BuildGateWorkspaceOutDir, preGateCanonicalSBOMFileName)
	got, err := os.ReadFile(sbomOutPath)
	if err != nil {
		t.Fatalf("expected canonical sbom output at %s: %v", sbomOutPath, err)
	}
	if string(got) != string(wantSnapshot) {
		t.Fatalf("materialized snapshot mismatch: got %q want %q", string(got), string(wantSnapshot))
	}
}

func TestMaterializePreGateSBOMForGate_UsesLastHookOutput(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-sbom-with-hooks")
	writeCanonicalSBOMFixture(t, preGateSBOMOutPath(runID), "pre-gate-cycle")

	// Simulate completed hook jobs where each writes /out/sbom.spdx.json.
	lastHookOut := preGateHookOutPath(runID, 1)
	wantSnapshot := writeCanonicalSBOMFixture(t, lastHookOut, "hook-cycle")

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
	if _, ok := meta["hook_once_skip_marked"]; ok {
		t.Fatalf("metadata.hook_once_skip_marked must not be emitted")
	}
}

func TestExecuteHookJob_SkipsHookWorkWhenHookShouldRunFalse(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-hook-skip")
	jobID := types.NewJobID()
	server, cap := newStatusCaptureServer(t, jobID.String())
	rc := newTestController(t, newAgentConfig(server.URL))

	input := writeCanonicalSBOMFixture(t, preGateSBOMOutPath(runID), "pre-gate-cycle")

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
