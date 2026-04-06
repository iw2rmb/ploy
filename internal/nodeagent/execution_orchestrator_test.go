package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func mustGateProfileSchemaJSON(t *testing.T) string {
	t.Helper()
	raw, err := contracts.ReadGateProfileSchemaJSON()
	if err != nil {
		t.Fatalf("ReadGateProfileSchemaJSON: %v", err)
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
				"build-gate.log":          "failure\n",
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
				"build-gate.log":          "failure\n",
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
