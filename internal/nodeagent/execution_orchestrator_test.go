package nodeagent

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	ploydnodeassets "github.com/iw2rmb/ploy/cmd/assets/ployd-node"
	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
	"gopkg.in/yaml.v3"
)

func mustGateProfileSchemaJSON(t *testing.T) string {
	t.Helper()
	raw, err := contracts.ReadGateProfileSchemaJSON()
	if err != nil {
		t.Fatalf("ReadGateProfileSchemaJSON: %v", err)
	}
	return string(raw)
}

func mustTrimmerSchemaJSON(t *testing.T, name string) string {
	t.Helper()
	raw, err := ploydnodeassets.ReadTrimmerSchema(name)
	if err != nil {
		t.Fatalf("ReadTrimmerSchema(%q): %v", name, err)
	}
	return string(raw)
}

func TestPopulateHealingInDir(t *testing.T) {
	const profile = `{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"java","tool":"maven"},"targets":{"active":"build","build":{"status":"passed","command":"mvn -q -DskipTests compile","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`

	type testCase struct {
		name         string
		seedFiles    map[string]string // relative to runDir -> content
		recovery     *contracts.RecoveryClaimContext
		schemaJSON   string            // "" = don't pass schema; "auto" = mustGateProfileSchemaJSON
		wantFiles    map[string]string // relative to inDir -> expected content ("" = just check existence)
		wantAbsent   []string          // files that must NOT exist in inDir
		wantErr      bool
		customAssert func(t *testing.T, inDir string)
	}

	cases := []testCase{
		{
			name: "CopiesGateLog",
			recovery: &contracts.RecoveryClaimContext{
				BuildGateLog: "trimmed failure log\n",
			},
			wantFiles: map[string]string{"build-gate.log": "trimmed failure log\n"},
		},
		{
			name: "CopiesGateProfileForInfra",
			seedFiles: map[string]string{
				"build-gate-profile.json": profile,
			},
			recovery: &contracts.RecoveryClaimContext{
				BuildGateLog: "failure\n",
			},
			schemaJSON: "auto",
			wantFiles: map[string]string{
				"build-gate.log":           "failure\n",
				"gate_profile.json":        profile,
				"gate_profile.schema.json": "",
			},
		},
		{
			name: "SkipsGateProfileForNonInfra",
			seedFiles: map[string]string{
				"build-gate-profile.json": `{"schema_version":1}`,
			},
			recovery: &contracts.RecoveryClaimContext{
				BuildGateLog: "failure\n",
			},
			wantFiles:  map[string]string{"build-gate.log": "failure\n"},
			wantAbsent: []string{"gate_profile.json", "gate_profile.schema.json"},
		},
		{
			name:       "InfraMissingGateProfileIsAllowed",
			recovery:   &contracts.RecoveryClaimContext{BuildGateLog: "failure\n"},
			schemaJSON: "auto",
			wantFiles: map[string]string{
				"build-gate.log":           "failure\n",
				"gate_profile.schema.json": "",
			},
		},
		{
			name:       "InfraMissingGateLogReturnsError",
			seedFiles:  map[string]string{}, // runDir exists but no files
			schemaJSON: "auto",
			wantErr:    true,
		},
		{
			name: "InfraEmptyGateLogReturnsError",
			recovery: &contracts.RecoveryClaimContext{
				BuildGateLog: "  \n",
			},
			schemaJSON: "auto",
			wantErr:    true,
		},
		{
			name: "UsesClaimRecoveryContextLog",
			recovery: &contracts.RecoveryClaimContext{
				BuildGateLog: "[ERROR] from claim payload\n",
			},
			wantFiles: map[string]string{"build-gate.log": "[ERROR] from claim payload\n"},
		},
		{
			name: "InfraUsesClaimRecoveryContextProfileAndSchema",
			recovery: &contracts.RecoveryClaimContext{
				BuildGateLog:          "claim-log\n",
				GateProfile:           json.RawMessage(profile),
				GateProfileSchemaJSON: "auto", // resolved below
			},
			wantFiles: map[string]string{
				"gate_profile.json":        profile,
				"gate_profile.schema.json": "auto", // resolved below
				"build-gate.log":           "claim-log\n",
			},
		},
		{
			name: "HydratesStructuredErrorsYAML",
			recovery: &contracts.RecoveryClaimContext{
				BuildGateLog:  "failure\n",
				DetectedStack: contracts.MigStackJavaGradle,
				Errors:        json.RawMessage(`{"task":"compileJava","errors":[{"message":"cannot find symbol"}]}`),
			},
			wantFiles: map[string]string{
				"build-gate.log":                  "failure\n",
				"gradle.java.trimmer.schema.json": "auto_trimmer",
			},
			customAssert: func(t *testing.T, inDir string) {
				t.Helper()
				raw, err := os.ReadFile(filepath.Join(inDir, "errors.yaml"))
				if err != nil {
					t.Fatalf("read /in/errors.yaml: %v", err)
				}
				var payload map[string]any
				if err := yaml.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode /in/errors.yaml: %v", err)
				}
				if _, hasMode := payload["mode"]; hasMode {
					t.Fatalf("errors.yaml unexpectedly contains mode=%v", payload["mode"])
				}
				if got, want := payload["task"], "compileJava"; got != want {
					t.Fatalf("errors.yaml task=%v, want %q", got, want)
				}
				if got, want := payload["$schema"], "/in/gradle.java.trimmer.schema.json"; got != want {
					t.Fatalf("errors.yaml $schema=%v, want %q", got, want)
				}
			},
		},
		{
			name: "HydratesStructuredErrorsYAMLArrayWithoutSchemaInjection",
			recovery: &contracts.RecoveryClaimContext{
				BuildGateLog:  "failure\n",
				DetectedStack: contracts.MigStackJavaGradle,
				Errors:        json.RawMessage(`[{"message":"cannot find symbol"}]`),
			},
			wantFiles: map[string]string{
				"build-gate.log":                  "failure\n",
				"gradle.java.trimmer.schema.json": "auto_trimmer",
			},
			customAssert: func(t *testing.T, inDir string) {
				t.Helper()
				raw, err := os.ReadFile(filepath.Join(inDir, "errors.yaml"))
				if err != nil {
					t.Fatalf("read /in/errors.yaml: %v", err)
				}
				var payload []map[string]any
				if err := yaml.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode /in/errors.yaml array: %v", err)
				}
				if len(payload) != 1 {
					t.Fatalf("errors.yaml len=%d, want 1", len(payload))
				}
				if _, has := payload[0]["$schema"]; has {
					t.Fatalf("errors.yaml array item unexpectedly contains $schema")
				}
			},
		},
		{
			name: "MalformedStructuredErrorsReturnsError",
			recovery: &contracts.RecoveryClaimContext{
				BuildGateLog: "failure\n",
				Errors:       json.RawMessage(`{"mode":`),
			},
			wantErr: true,
		},
		{
			name: "DepsCompatHydrationWritesInputs",
			recovery: func() *contracts.RecoveryClaimContext {
				ver := "2.0.13"
				return &contracts.RecoveryClaimContext{
					BuildGateLog:       "deps failure\n",
					DepsCompatEndpoint: "/v1/sboms/compat?lang=java&release=17&tool=maven&libs=",
					DepsBumps: map[string]*string{
						"org.slf4j:slf4j-api": &ver,
						"legacy:shim":         nil,
					},
				}
			}(),
			wantFiles: map[string]string{"build-gate.log": "deps failure\n"},
			customAssert: func(t *testing.T, inDir string) {
				t.Helper()
				gotCompat, err := os.ReadFile(filepath.Join(inDir, "deps-compat-url.txt"))
				if err != nil {
					t.Fatalf("read /in/deps-compat-url.txt: %v", err)
				}
				if got, want := string(gotCompat), "/v1/sboms/compat?lang=java&release=17&tool=maven&libs="; got != want {
					t.Fatalf("/in/deps-compat-url.txt = %q, want %q", got, want)
				}

				gotBumpsRaw, err := os.ReadFile(filepath.Join(inDir, "deps-bumps.json"))
				if err != nil {
					t.Fatalf("read /in/deps-bumps.json: %v", err)
				}
				var gotBumps map[string]any
				if err := json.Unmarshal(gotBumpsRaw, &gotBumps); err != nil {
					t.Fatalf("decode /in/deps-bumps.json: %v", err)
				}
				if got := gotBumps["org.slf4j:slf4j-api"]; got != "2.0.13" {
					t.Fatalf("deps-bumps.org.slf4j:slf4j-api = %v, want 2.0.13", got)
				}
				if got, ok := gotBumps["legacy:shim"]; !ok || got != nil {
					t.Fatalf("deps-bumps.legacy:shim = %v (present=%v), want null", got, ok)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cacheHome := t.TempDir()
			t.Setenv("PLOYD_CACHE_HOME", cacheHome)

			rc := &runController{cfg: Config{}}
			runID := types.RunID("run-" + tc.name)

			// Resolve "auto" schemaJSON.
			schemaJSON := tc.schemaJSON
			if schemaJSON == "auto" {
				schemaJSON = mustGateProfileSchemaJSON(t)
			}

			// Resolve "auto" values in recovery and wantFiles that depend on schema.
			if tc.recovery != nil && tc.recovery.GateProfileSchemaJSON == "auto" {
				tc.recovery.GateProfileSchemaJSON = mustGateProfileSchemaJSON(t)
			}
			resolvedWantFiles := make(map[string]string, len(tc.wantFiles))
			for k, v := range tc.wantFiles {
				if v == "auto" {
					resolvedWantFiles[k] = mustGateProfileSchemaJSON(t)
				} else if v == "auto_trimmer" {
					resolvedWantFiles[k] = mustTrimmerSchemaJSON(t, k)
				} else {
					resolvedWantFiles[k] = v
				}
			}

			// Create runDir and seed files.
			if tc.seedFiles != nil {
				runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
				if err := os.MkdirAll(runDir, 0o755); err != nil {
					t.Fatalf("mkdir runDir: %v", err)
				}
				for name, content := range tc.seedFiles {
					if err := os.WriteFile(filepath.Join(runDir, name), []byte(content), 0o644); err != nil {
						t.Fatalf("write seed file %s: %v", name, err)
					}
				}
			}

			inDir := t.TempDir()

			err := rc.populateHealingInDir(runID, inDir, tc.recovery, schemaJSON)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("populateHealingInDir error: %v", err)
			}

			// Check expected files.
			for name, want := range resolvedWantFiles {
				data, err := os.ReadFile(filepath.Join(inDir, name))
				if err != nil {
					t.Fatalf("failed to read /in/%s: %v", name, err)
				}
				if want != "" && string(data) != want {
					t.Fatalf("/in/%s = %q, want %q", name, string(data), want)
				}
			}

			// Check absent files.
			for _, name := range tc.wantAbsent {
				if _, err := os.Stat(filepath.Join(inDir, name)); !os.IsNotExist(err) {
					t.Fatalf("/in/%s should not exist, err=%v", name, err)
				}
			}

			// Run custom assertions if provided.
			if tc.customAssert != nil {
				tc.customAssert(t, inDir)
			}
		})
	}
}

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

func TestGateCycleNameFromSBOMContext(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ctx     *contracts.SBOMJobMetadata
		want    string
		wantErr bool
	}{
		{name: "explicit cycle name", ctx: &contracts.SBOMJobMetadata{CycleName: "re-gate-2", Phase: contracts.SBOMPhasePost, Role: contracts.SBOMRoleRetry}, want: "re-gate-2"},
		{name: "pre", ctx: &contracts.SBOMJobMetadata{Phase: contracts.SBOMPhasePre, Role: contracts.SBOMRoleInitial}, want: "pre-gate"},
		{name: "post", ctx: &contracts.SBOMJobMetadata{Phase: contracts.SBOMPhasePost, Role: contracts.SBOMRoleRetry}, want: "post-gate"},
		{name: "invalid phase", ctx: &contracts.SBOMJobMetadata{Phase: "oops"}, wantErr: true},
		{name: "nil", ctx: nil, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := gateCycleNameFromSBOMContext(tc.ctx)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for context=%#v", tc.ctx)
				}
				return
			}
			if err != nil {
				t.Fatalf("gateCycleNameFromSBOMContext(%#v): %v", tc.ctx, err)
			}
			if got != tc.want {
				t.Fatalf("gateCycleNameFromSBOMContext(%#v)=%q want %q", tc.ctx, got, tc.want)
			}
		})
	}
}

func TestGateCycleHookIndexFromJobName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		jobName string
		hooks   int
		wantID  string
		wantIdx int
		wantErr bool
	}{
		{name: "pre", jobName: "pre-gate-hook-001", hooks: 2, wantID: "pre-gate", wantIdx: 1},
		{name: "post", jobName: "post-gate-hook-000", hooks: 1, wantID: "post-gate", wantIdx: 0},
		{name: "regate", jobName: "re-gate-3-hook-002", hooks: 3, wantID: "re-gate-3", wantIdx: 2},
		{name: "invalid", jobName: "hook-1", hooks: 2, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotID, gotIdx, err := gateCycleHookIndexFromJobName(tc.jobName, tc.hooks)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for job_name=%q", tc.jobName)
				}
				return
			}
			if err != nil {
				t.Fatalf("gateCycleHookIndexFromJobName(%q,%d): %v", tc.jobName, tc.hooks, err)
			}
			if gotID != tc.wantID || gotIdx != tc.wantIdx {
				t.Fatalf("gateCycleHookIndexFromJobName(%q,%d)=(%q,%d) want (%q,%d)", tc.jobName, tc.hooks, gotID, gotIdx, tc.wantID, tc.wantIdx)
			}
		})
	}
}

func TestMaterializeGateSBOMForGate_UsesPostAndReGateCycleSnapshots(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.RunID("run-cycle-materialize")
	postSnapshot := []byte(`{"spdxVersion":"SPDX-2.3","name":"post-gate-cycle"}`)
	postLastHookOut := gateCycleHookOutPath(runID, postGateCycleName, 1)
	if err := os.MkdirAll(filepath.Dir(postLastHookOut), 0o755); err != nil {
		t.Fatalf("mkdir post hook out: %v", err)
	}
	if err := os.WriteFile(postLastHookOut, postSnapshot, 0o644); err != nil {
		t.Fatalf("write post hook snapshot: %v", err)
	}

	postWorkspace := t.TempDir()
	if err := materializeGateSBOMForGate(runID, postGateCycleName, []string{"./hooks/a.yaml", "./hooks/b.yaml"}, postWorkspace); err != nil {
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

	reGateCycle := "re-gate-2"
	reGateSnapshot := []byte(`{"spdxVersion":"SPDX-2.3","name":"re-gate-cycle"}`)
	reGateSBOMOut := gateCycleSBOMOutPath(runID, reGateCycle)
	if err := os.MkdirAll(filepath.Dir(reGateSBOMOut), 0o755); err != nil {
		t.Fatalf("mkdir re-gate sbom dir: %v", err)
	}
	if err := os.WriteFile(reGateSBOMOut, reGateSnapshot, 0o644); err != nil {
		t.Fatalf("write re-gate sbom snapshot: %v", err)
	}

	reGateWorkspace := t.TempDir()
	if err := materializeGateSBOMForGate(runID, reGateCycle, nil, reGateWorkspace); err != nil {
		t.Fatalf("materialize re-gate sbom: %v", err)
	}
	reGateOutPath := filepath.Join(reGateWorkspace, step.BuildGateWorkspaceOutDir, preGateCanonicalSBOMFileName)
	reGateOut, err := os.ReadFile(reGateOutPath)
	if err != nil {
		t.Fatalf("read re-gate out snapshot: %v", err)
	}
	if string(reGateOut) != string(reGateSnapshot) {
		t.Fatalf("re-gate snapshot mismatch: got %q want %q", string(reGateOut), string(reGateSnapshot))
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
	err := materializeGateSBOMForGate(runID, postGateCycleName, nil, postWorkspace)
	if err == nil {
		t.Fatalf("expected error when post-gate snapshot is missing")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected os.IsNotExist for missing post-gate snapshot, got: %v", err)
	}

	reGateWorkspace := t.TempDir()
	err = materializeGateSBOMForGate(runID, "re-gate-1", nil, reGateWorkspace)
	if err == nil {
		t.Fatalf("expected error when re-gate snapshot is missing")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected os.IsNotExist for missing re-gate snapshot, got: %v", err)
	}
}
