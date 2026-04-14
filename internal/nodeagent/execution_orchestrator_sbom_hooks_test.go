package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/hook"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func mustTarGzEntries(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, payload := range entries {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(payload)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %q: %v", name, err)
		}
		if _, err := tw.Write(payload); err != nil {
			t.Fatalf("write tar payload %q: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

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
	classpathOutPath := filepath.Join(outDir, sbomJavaClasspathFileName)
	classpathSnapshotPath := filepath.Join(filepath.Dir(snapshotPath), sbomJavaClasspathFileName)

	rawDeps := strings.Join([]string{
		"[INFO]    com.fasterxml.jackson.core:jackson-databind:jar:2.17.2:compile",
		"\\--- org.apache.commons:commons-lang3:3.17.0",
		"+--- org.openapi.generator:org.openapi.generator.gradle.plugin:6.6.0",
		"noise line that should be ignored",
	}, "\n")
	if err := os.WriteFile(filepath.Join(outDir, sbomDependencyOutputFileName), []byte(rawDeps), 0o644); err != nil {
		t.Fatalf("write raw dependency output: %v", err)
	}
	rawClasspath := []byte("/home/gradle/.gradle/caches/modules-2/files-2.1/a/b/c/a.jar\n/home/gradle/.gradle/caches/modules-2/files-2.1/x/y/z/b.jar\n")
	if err := os.WriteFile(classpathOutPath, rawClasspath, 0o644); err != nil {
		t.Fatalf("write java classpath output: %v", err)
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
	if len(doc.Packages) != 3 {
		t.Fatalf("packages len = %d, want 3", len(doc.Packages))
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
	if got, want := doc.Packages[2].Name, "org.openapi.generator:org.openapi.generator.gradle.plugin"; got != want {
		t.Fatalf("packages[2].name = %q, want %q", got, want)
	}
	if got, want := doc.Packages[2].VersionInfo, "6.6.0"; got != want {
		t.Fatalf("packages[2].versionInfo = %q, want %q", got, want)
	}

	stagedRaw, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read staged sbom snapshot: %v", err)
	}
	if string(stagedRaw) != string(canonicalRaw) {
		t.Fatalf("staged snapshot mismatch with canonical output")
	}
	stagedClasspathRaw, err := os.ReadFile(classpathSnapshotPath)
	if err != nil {
		t.Fatalf("read staged java classpath snapshot: %v", err)
	}
	if string(stagedClasspathRaw) != string(rawClasspath) {
		t.Fatalf("staged java classpath mismatch with output")
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

func TestMaterializeValidatedSBOMOutput_ErrorsWhenJavaClasspathMissing(t *testing.T) {
	outDir := t.TempDir()
	snapshotPath := filepath.Join(t.TempDir(), "gate-cycle", "sbom", "out", preGateCanonicalSBOMFileName)
	rawDeps := "[INFO]    com.fasterxml.jackson.core:jackson-databind:jar:2.17.2:compile\n"
	if err := os.WriteFile(filepath.Join(outDir, sbomDependencyOutputFileName), []byte(rawDeps), 0o644); err != nil {
		t.Fatalf("write raw dependency output: %v", err)
	}

	err := materializeValidatedSBOMOutput(outDir, snapshotPath)
	if err == nil {
		t.Fatal("expected error for missing java classpath output")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
	if !strings.Contains(err.Error(), sbomJavaClasspathFileName) {
		t.Fatalf("error = %q, want mention of %s", err, sbomJavaClasspathFileName)
	}
}

func TestMaterializeHookSnapshot_CopiesInputSnapshot(t *testing.T) {
	inputSnapshotPath := filepath.Join(t.TempDir(), "hook-input", preGateCanonicalSBOMFileName)
	snapshotPath := filepath.Join(t.TempDir(), "hook-output", preGateCanonicalSBOMFileName)

	inputRaw := writeCanonicalSBOMFixture(t, inputSnapshotPath, "input-cycle")
	if err := materializeHookSnapshot(inputSnapshotPath, snapshotPath); err != nil {
		t.Fatalf("materializeHookSnapshot: %v", err)
	}

	got, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(got) != string(inputRaw) {
		t.Fatalf("snapshot mismatch")
	}
	if err := validateCanonicalSBOMDocument(got); err != nil {
		t.Fatalf("validate snapshot: %v", err)
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

	// Simulate completed hook snapshots for each hook index.
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

func TestGateCycleHookInputSnapshotPath_FallsBackToLatestExistingHookOutput(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-hook-sparse-input")
	cycleName := "pre-gate"
	sbomPath := gateCycleSBOMOutPath(runID, cycleName)
	writeCanonicalSBOMFixture(t, sbomPath, "base-sbom")
	hookZeroOut := gateCycleHookOutPath(runID, cycleName, 0)
	writeCanonicalSBOMFixture(t, hookZeroOut, "hook-0")

	got := gateCycleHookInputSnapshotPath(runID, cycleName, 2)
	if got != hookZeroOut {
		t.Fatalf("gateCycleHookInputSnapshotPath()=%q, want latest existing %q", got, hookZeroOut)
	}

	got = gateCycleHookInputSnapshotPath(runID, cycleName, 1)
	if got != hookZeroOut {
		t.Fatalf("gateCycleHookInputSnapshotPath()=%q, want previous hook output %q", got, hookZeroOut)
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

func TestAddHookRuntimeMetadata_EmitsMatchedTransitionKeys(t *testing.T) {
	builder := types.NewRunStatsBuilder()
	addHookRuntimeMetadata(builder, &contracts.HookRuntimeDecision{
		HookHash:         "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		HookShouldRun:    true,
		MatchedPredicate: "on_change",
		MatchedPackage:   "org.openapi.generator:org.openapi.generator.gradle.plugin",
		PreviousVersion:  "4.3.0",
		CurrentVersion:   "6.6.0",
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
	if got := meta["hook_matched_predicate"]; got != "on_change" {
		t.Fatalf("metadata.hook_matched_predicate=%v, want on_change", got)
	}
	if got := meta["hook_matched_package"]; got != "org.openapi.generator:org.openapi.generator.gradle.plugin" {
		t.Fatalf("metadata.hook_matched_package=%v, want package name", got)
	}
	if got := meta["hook_previous_version"]; got != "4.3.0" {
		t.Fatalf("metadata.hook_previous_version=%v, want 4.3.0", got)
	}
	if got := meta["hook_current_version"]; got != "6.6.0" {
		t.Fatalf("metadata.hook_current_version=%v, want 6.6.0", got)
	}
}

func TestMergeHookRuntimeDecisionEnv_InjectsRuntimeContext(t *testing.T) {
	merged := mergeHookRuntimeDecisionEnv(map[string]string{"A": "1"}, &contracts.HookRuntimeDecision{
		MatchedPredicate: "on_change",
		MatchedPackage:   "org.openapi.generator:org.openapi.generator.gradle.plugin",
		PreviousVersion:  "4.3.0",
		CurrentVersion:   "6.6.0",
	})
	if got := merged["A"]; got != "1" {
		t.Fatalf("merged[A]=%q want 1", got)
	}
	if got := merged["PLOY_HOOK_MATCHED_PREDICATE"]; got != "on_change" {
		t.Fatalf("PLOY_HOOK_MATCHED_PREDICATE=%q want on_change", got)
	}
	if got := merged["PLOY_HOOK_MATCHED_PACKAGE"]; got != "org.openapi.generator:org.openapi.generator.gradle.plugin" {
		t.Fatalf("PLOY_HOOK_MATCHED_PACKAGE=%q want package", got)
	}
	if got := merged["PLOY_HOOK_PREVIOUS_VERSION"]; got != "4.3.0" {
		t.Fatalf("PLOY_HOOK_PREVIOUS_VERSION=%q want 4.3.0", got)
	}
	if got := merged["PLOY_HOOK_CURRENT_VERSION"]; got != "6.6.0" {
		t.Fatalf("PLOY_HOOK_CURRENT_VERSION=%q want 6.6.0", got)
	}
}

func TestExecuteHookJob_FailsWhenHookShouldRunFalseReachesExecution(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-hook-skip")
	jobID := types.NewJobID()
	server, cap := newStatusCaptureServer(t, jobID.String())
	rc := newTestController(t, newAgentConfig(server.URL))

	writeCanonicalSBOMFixture(t, preGateSBOMOutPath(runID), "pre-gate-cycle")

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

	if got := cap.Status; got != types.JobStatusError.String() {
		t.Fatalf("status=%q, want %q", got, types.JobStatusError.String())
	}

	inPath := preGateHookInPath(runID, 0)
	if _, err := os.Stat(inPath); !os.IsNotExist(err) {
		t.Fatalf("expected invariant-fail path to avoid /in materialization, err=%v", err)
	}

	outPath := preGateHookOutPath(runID, 0)
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("expected invariant-fail path to avoid /out materialization, err=%v", err)
	}
}

func TestHookRuntimeStepCA_MergesCyclePhaseCA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cycleName string
		wantCA    []string
	}{
		{name: "pre gate cycle merges pre phase CA", cycleName: preGateCycleName, wantCA: []string{"step-ca", "pre-ca"}},
		{name: "post gate cycle merges post phase CA", cycleName: postGateCycleName, wantCA: []string{"step-ca", "post-ca"}},
		{name: "re gate cycle merges post phase CA", cycleName: "re-gate-2", wantCA: []string{"step-ca", "post-ca"}},
		{name: "unknown cycle keeps step CA only", cycleName: "other", wantCA: []string{"step-ca"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			typed := RunOptions{
				BuildGate: BuildGateOptions{
					Pre:  &contracts.BuildGatePhaseConfig{CA: []string{"pre-ca", "step-ca"}},
					Post: &contracts.BuildGatePhaseConfig{CA: []string{"post-ca", "step-ca"}},
				},
			}

			phase := sbomPhaseConfigForCycle(tc.cycleName, typed)
			phaseCA := []string(nil)
			if phase != nil {
				phaseCA = append(phaseCA, phase.CA...)
			}

			stepSpec := hook.Step{Image: contracts.JobImage{Universal: "hook:latest"}, CA: []string{"step-ca"}}
			runtimeStep := stepSpec
			runtimeStep.CA = mergeUniqueStringEntries(append([]string(nil), stepSpec.CA...), phaseCA)

			if len(runtimeStep.CA) != len(tc.wantCA) {
				t.Fatalf("runtime step ca len=%d, want %d (%v)", len(runtimeStep.CA), len(tc.wantCA), runtimeStep.CA)
			}
			for i := range tc.wantCA {
				if runtimeStep.CA[i] != tc.wantCA[i] {
					t.Fatalf("runtime step ca[%d]=%q, want %q (%v)", i, runtimeStep.CA[i], tc.wantCA[i], runtimeStep.CA)
				}
			}
		})
	}
}

func TestRestoreSBOMOutFilesFromBundle_RestoresSBOMOutputsOnly(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	validCanonical := []byte(`{"spdxVersion":"SPDX-2.3","packages":[]}`)
	bundle := mustTarGzEntries(t, map[string][]byte{
		"out/sbom.spdx.json":        validCanonical,
		"out/sbom.dependencies.txt": []byte("org.example:lib:1.0.0"),
		"out/java.classpath":        []byte("/root/.m2/repository/org/example/lib/1.0.0/lib-1.0.0.jar\n"),
		"out/other.txt":             []byte("ignore"),
	})

	count, err := restoreSBOMOutFilesFromBundle(bundle, outDir)
	if err != nil {
		t.Fatalf("restoreSBOMOutFilesFromBundle() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("restored count = %d, want 3", count)
	}
	if _, err := os.Stat(filepath.Join(outDir, "sbom.spdx.json")); err != nil {
		t.Fatalf("expected canonical sbom to be restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "sbom.dependencies.txt")); err != nil {
		t.Fatalf("expected dependency output to be restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, sbomJavaClasspathFileName)); err != nil {
		t.Fatalf("expected java classpath output to be restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "other.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected non-sbom output not to be restored, err=%v", err)
	}
}

func TestRestoreSBOMOutFilesFromBundle_ErrorsWhenJavaClasspathMissing(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	bundle := mustTarGzEntries(t, map[string][]byte{
		"out/sbom.spdx.json":        []byte(`{"spdxVersion":"SPDX-2.3","packages":[]}`),
		"out/sbom.dependencies.txt": []byte("org.example:lib:1.0.0"),
	})

	_, err := restoreSBOMOutFilesFromBundle(bundle, outDir)
	if err == nil {
		t.Fatal("restoreSBOMOutFilesFromBundle() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), sbomJavaClasspathFileName) {
		t.Fatalf("error = %v, want mention of %s", err, sbomJavaClasspathFileName)
	}
}

func TestEnsureHookSBOMInputSnapshot_RestoresMissingCycleSnapshotFromArtifact(t *testing.T) {
	t.Parallel()

	artifactID := "11111111-1111-1111-1111-111111111111"
	bundle := mustTarGzEntries(t, map[string][]byte{
		"out/sbom.spdx.json":        []byte(`{"spdxVersion":"SPDX-2.3","packages":[]}`),
		"out/sbom.dependencies.txt": []byte("org.example:lib:1.0.0"),
		"out/java.classpath":        []byte("/home/gradle/.gradle/caches/modules-2/files-2.1/a/b/c/a.jar\n"),
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

	cfg := newAgentConfig(server.URL)
	rc := newTestController(t, cfg)
	runID := types.NewRunID()
	t.Cleanup(func() { _ = os.RemoveAll(runCacheDir(runID)) })

	inputPath := gateCycleSBOMOutPath(runID, postGateCycleName)
	req := StartRunRequest{
		RunID: runID,
		JobID: types.NewJobID(),
		HookContext: &contracts.HookClaimContext{
			CycleName:              postGateCycleName,
			Source:                 "https://hooks.example/hook.yaml",
			Index:                  0,
			UpstreamSBOMArtifactID: artifactID,
		},
	}
	if err := rc.ensureHookSBOMInputSnapshot(context.Background(), req, postGateCycleName, inputPath); err != nil {
		t.Fatalf("ensureHookSBOMInputSnapshot() error = %v", err)
	}
	if _, err := os.Stat(inputPath); err != nil {
		t.Fatalf("expected restored cycle sbom snapshot at %q: %v", inputPath, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(inputPath), sbomJavaClasspathFileName)); err != nil {
		t.Fatalf("expected restored cycle java classpath snapshot: %v", err)
	}
}

func TestEnsureHookSBOMInputSnapshot_MissingArtifactIDReturnsError(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	t.Cleanup(func() { _ = os.RemoveAll(runCacheDir(runID)) })
	rc := &runController{}
	inputPath := gateCycleSBOMOutPath(runID, postGateCycleName)
	req := StartRunRequest{
		RunID:       runID,
		JobID:       types.NewJobID(),
		HookContext: &contracts.HookClaimContext{CycleName: postGateCycleName, Index: 0},
	}
	err := rc.ensureHookSBOMInputSnapshot(context.Background(), req, postGateCycleName, inputPath)
	if err == nil {
		t.Fatal("ensureHookSBOMInputSnapshot() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "upstream sbom artifact id is empty") {
		t.Fatalf("error = %v, want upstream artifact id message", err)
	}
}
