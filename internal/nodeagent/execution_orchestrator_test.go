package nodeagent

import (
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

	if err := rc.populateHealingInDir(runID, inDir, nil, nil, ""); err != nil {
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

func TestPopulateHealingInDirCopiesGateProfileForInfra(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-copy-profile-infra")

	runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "build-gate-first.log"), []byte("failure\n"), 0o644); err != nil {
		t.Fatalf("write first gate log: %v", err)
	}
	const profile = `{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"java","tool":"maven"},"targets":{"active":"build","build":{"status":"passed","command":"mvn -q -DskipTests compile","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`
	if err := os.WriteFile(filepath.Join(runDir, "build-gate-profile.json"), []byte(profile), 0o644); err != nil {
		t.Fatalf("write gate profile snapshot: %v", err)
	}

	inDir := t.TempDir()
	schemaJSON := mustGateProfileSchemaJSON(t)
	if err := rc.populateHealingInDir(runID, inDir, &contracts.HealingSpec{SelectedErrorKind: "infra"}, nil, schemaJSON); err != nil {
		t.Fatalf("populateHealingInDir error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(inDir, "gate_profile.json"))
	if err != nil {
		t.Fatalf("failed to read /in/gate_profile.json: %v", err)
	}
	if got := string(data); got != profile {
		t.Fatalf("healing /in/gate_profile.json = %q, want %q", got, profile)
	}
	gotSchema, err := os.ReadFile(filepath.Join(inDir, "gate_profile.schema.json"))
	if err != nil {
		t.Fatalf("failed to read /in/gate_profile.schema.json: %v", err)
	}
	if got, want := string(gotSchema), schemaJSON; got != want {
		t.Fatalf("healing /in/gate_profile.schema.json mismatch")
	}
}

func TestPopulateHealingInDirSkipsGateProfileForNonInfra(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-skip-profile-code")

	runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "build-gate-first.log"), []byte("failure\n"), 0o644); err != nil {
		t.Fatalf("write first gate log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "build-gate-profile.json"), []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatalf("write gate profile snapshot: %v", err)
	}

	inDir := t.TempDir()
	if err := rc.populateHealingInDir(runID, inDir, &contracts.HealingSpec{SelectedErrorKind: "code"}, nil, ""); err != nil {
		t.Fatalf("populateHealingInDir error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inDir, "gate_profile.json")); !os.IsNotExist(err) {
		t.Fatalf("gate_profile.json exists for non-infra healing, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(inDir, "gate_profile.schema.json")); !os.IsNotExist(err) {
		t.Fatalf("gate_profile.schema.json exists for non-infra healing, err=%v", err)
	}
}

func TestPopulateHealingInDirInfraMissingGateProfileIsAllowed(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-missing-profile-infra")

	runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "build-gate-first.log"), []byte("failure\n"), 0o644); err != nil {
		t.Fatalf("write first gate log: %v", err)
	}

	inDir := t.TempDir()
	schemaJSON := mustGateProfileSchemaJSON(t)
	if err := rc.populateHealingInDir(runID, inDir, &contracts.HealingSpec{SelectedErrorKind: "infra"}, nil, schemaJSON); err != nil {
		t.Fatalf("populateHealingInDir error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inDir, "gate_profile.schema.json")); err != nil {
		t.Fatalf("expected /in/gate_profile.schema.json for infra healing, err=%v", err)
	}
}

func TestPopulateHealingInDirInfraMissingGateLogStillHydratesSchema(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-missing-log-infra")

	runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}

	inDir := t.TempDir()
	schemaJSON := mustGateProfileSchemaJSON(t)
	if err := rc.populateHealingInDir(runID, inDir, &contracts.HealingSpec{SelectedErrorKind: "infra"}, nil, schemaJSON); err != nil {
		t.Fatalf("populateHealingInDir error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inDir, "gate_profile.schema.json")); err != nil {
		t.Fatalf("expected /in/gate_profile.schema.json for infra healing, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(inDir, "build-gate.log")); !os.IsNotExist(err) {
		t.Fatalf("build-gate.log exists for missing source log, err=%v", err)
	}
}

func TestPopulateHealingInDirInfraEmptyGateLogStillHydratesSchema(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-empty-log-infra")

	runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "build-gate-first.log"), []byte("  \n"), 0o644); err != nil {
		t.Fatalf("write empty first gate log: %v", err)
	}

	inDir := t.TempDir()
	schemaJSON := mustGateProfileSchemaJSON(t)
	if err := rc.populateHealingInDir(runID, inDir, &contracts.HealingSpec{SelectedErrorKind: "infra"}, nil, schemaJSON); err != nil {
		t.Fatalf("populateHealingInDir error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inDir, "gate_profile.schema.json")); err != nil {
		t.Fatalf("expected /in/gate_profile.schema.json for infra healing, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(inDir, "build-gate.log")); !os.IsNotExist(err) {
		t.Fatalf("build-gate.log exists for empty source log, err=%v", err)
	}
}

func TestPopulateHealingInDirInfraMissingSchemaFails(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-missing-schema-infra")

	runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "build-gate-first.log"), []byte("failure\n"), 0o644); err != nil {
		t.Fatalf("write first gate log: %v", err)
	}

	inDir := t.TempDir()
	err := rc.populateHealingInDir(runID, inDir, &contracts.HealingSpec{SelectedErrorKind: "infra"}, nil, "")
	if err == nil {
		t.Fatalf("expected populateHealingInDir error for missing schema env")
	}
}

func TestPopulateHealingInDir_UsesClaimRecoveryContextLog(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-claim-log")
	inDir := t.TempDir()

	recovery := &contracts.RecoveryClaimContext{
		BuildGateLog: "[ERROR] from claim payload\n",
	}
	if err := rc.populateHealingInDir(runID, inDir, nil, recovery, ""); err != nil {
		t.Fatalf("populateHealingInDir error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(inDir, "build-gate.log"))
	if err != nil {
		t.Fatalf("read /in/build-gate.log: %v", err)
	}
	if got, want := string(data), recovery.BuildGateLog; got != want {
		t.Fatalf("/in/build-gate.log = %q, want %q", got, want)
	}
}

func TestPopulateHealingInDirInfra_UsesClaimRecoveryContextProfileAndSchema(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-claim-profile")
	inDir := t.TempDir()
	profile := []byte(`{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"java","tool":"maven"},"targets":{"active":"build","build":{"status":"passed","command":"mvn -q -DskipTests compile","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`)
	schemaJSON := mustGateProfileSchemaJSON(t)

	recovery := &contracts.RecoveryClaimContext{
		BuildGateLog:          "claim-log\n",
		GateProfile:           profile,
		GateProfileSchemaJSON: schemaJSON,
		SelectedErrorKind:     "infra",
	}
	if err := rc.populateHealingInDir(runID, inDir, &contracts.HealingSpec{SelectedErrorKind: "infra"}, recovery, ""); err != nil {
		t.Fatalf("populateHealingInDir error: %v", err)
	}

	gotProfile, err := os.ReadFile(filepath.Join(inDir, "gate_profile.json"))
	if err != nil {
		t.Fatalf("read /in/gate_profile.json: %v", err)
	}
	if got := string(gotProfile); got != string(profile) {
		t.Fatalf("/in/gate_profile.json = %q, want %q", got, string(profile))
	}

	gotSchema, err := os.ReadFile(filepath.Join(inDir, "gate_profile.schema.json"))
	if err != nil {
		t.Fatalf("read /in/gate_profile.schema.json: %v", err)
	}
	if got, want := string(gotSchema), schemaJSON; got != want {
		t.Fatalf("/in/gate_profile.schema.json = %q, want %q", got, want)
	}
}
