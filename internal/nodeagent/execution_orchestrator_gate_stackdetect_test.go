package nodeagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func testLogDigest(n int) types.Sha256Digest {
	suffix := string(rune('a' + (n % 6)))
	return types.Sha256Digest("sha256:" + strings.Repeat("0", 63) + suffix)
}

// TestPersistFirstGateFailureLog_UsesTrimmedFinding verifies that the first failing
// gate log persisted for healing prefers the trimmed LogFindings view over LogsText.
func TestPersistFirstGateFailureLog_UsesTrimmedFinding(t *testing.T) {
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-trimmed-log")

	full := "[INFO] noise\n[ERROR] important failure\nstack\n"
	trimmed := "[ERROR] important failure\nstack\n"

	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Tool: "maven", Passed: false},
		},
		LogsText: full,
		LogFindings: []contracts.BuildGateLogFinding{
			{Severity: "error", Message: trimmed},
		},
	}

	rc.persistFirstGateFailureLog(runID, meta)

	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	runDir := filepath.Join(baseRoot, "ploy", "run", runID.String())
	logPath := filepath.Join(runDir, "build-gate-first.log")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read persisted gate log: %v", err)
	}

	got := string(data)
	if got != trimmed && got != trimmed+"\n" {
		t.Fatalf("persisted gate log = %q, want trimmed log %q", got, trimmed)
	}
}

func TestPersistGateProfileSnapshot(t *testing.T) {
	type testCase struct {
		name     string
		gateSpec *contracts.StepGateSpec
		jobType  types.JobType
		gateMeta *contracts.BuildGateStageMetadata
		seedFile bool
		assertFn func(t *testing.T, path string)
	}

	cases := []testCase{
		{
			name: "DerivesFromOverride",
			gateSpec: &contracts.StepGateSpec{
				RepoID: types.MigRepoID("repo_2"),
				GateProfile: &contracts.BuildGateProfileOverride{
					Command: contracts.CommandSpec{Shell: "mvn -q -DskipTests compile"},
					Env:     map[string]string{"MAVEN_OPTS": "-Xmx2g"},
					Stack:   &contracts.GateProfileStack{Language: "java", Tool: "maven", Release: "21"},
				},
			},
			jobType: types.JobTypePreGate,
			assertFn: func(t *testing.T, path string) {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("failed to read profile snapshot: %v", err)
				}
				profile, err := contracts.ParseGateProfileJSON(data)
				if err != nil {
					t.Fatalf("snapshot profile invalid: %v", err)
				}
				if got, want := profile.Stack.Language, "java"; got != want {
					t.Fatalf("stack.language = %q, want %q", got, want)
				}
				if got, want := profile.Stack.Tool, "maven"; got != want {
					t.Fatalf("stack.tool = %q, want %q", got, want)
				}
				if got, want := profile.Targets.Active, contracts.GateProfileTargetAllTests; got != want {
					t.Fatalf("targets.active = %q, want %q", got, want)
				}
				if profile.Targets.AllTests == nil || profile.Targets.AllTests.Command != "mvn -q -DskipTests compile" {
					t.Fatalf("targets.all_tests.command = %#v, want mvn command", profile.Targets.AllTests)
				}
				if profile.Targets.AllTests.Env["MAVEN_OPTS"] != "-Xmx2g" {
					t.Fatalf("targets.all_tests.env[MAVEN_OPTS] = %q, want %q", profile.Targets.AllTests.Env["MAVEN_OPTS"], "-Xmx2g")
				}
			},
		},
		{
			name: "UsesPinnedTarget",
			gateSpec: &contracts.StepGateSpec{
				RepoID: types.MigRepoID("repo_3"),
				Target: contracts.GateProfileTargetBuild,
				GateProfile: &contracts.BuildGateProfileOverride{
					Command: contracts.CommandSpec{Shell: "mvn -q -DskipTests compile"},
					Target:  contracts.GateProfileTargetAllTests,
					Env:     map[string]string{"MAVEN_OPTS": "-Xmx2g"},
					Stack:   &contracts.GateProfileStack{Language: "java", Tool: "maven", Release: "11"},
				},
			},
			jobType: types.JobTypePreGate,
			assertFn: func(t *testing.T, path string) {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("failed to read profile snapshot: %v", err)
				}
				profile, err := contracts.ParseGateProfileJSON(data)
				if err != nil {
					t.Fatalf("snapshot profile invalid: %v", err)
				}
				if got, want := profile.Targets.Active, contracts.GateProfileTargetBuild; got != want {
					t.Fatalf("targets.active = %q, want %q", got, want)
				}
				if profile.Targets.Build == nil || profile.Targets.Build.Command != "mvn -q -DskipTests compile" {
					t.Fatalf("targets.build.command = %#v, want mvn command", profile.Targets.Build)
				}
				if got := profile.Targets.AllTests.Command; got != "" {
					t.Fatalf("targets.all_tests.command = %q, want empty", got)
				}
			},
		},
		{
			name:     "RemovesStaleSnapshot",
			seedFile: true,
			gateSpec: nil,
			jobType:  types.JobTypePreGate,
			assertFn: func(t *testing.T, path string) {
				t.Helper()
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("expected stale snapshot removed, stat err=%v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cacheHome := t.TempDir()
			t.Setenv("PLOYD_CACHE_HOME", cacheHome)

			rc := &runController{cfg: Config{}}
			runID := types.RunID("run-profile-" + tc.name)
			path := filepath.Join(cacheHome, "ploy", "run", runID.String(), "build-gate-profile.json")

			if tc.seedFile {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir run dir: %v", err)
				}
				if err := os.WriteFile(path, []byte(`{"schema_version":1}`), 0o644); err != nil {
					t.Fatalf("write seed file: %v", err)
				}
			}

			rc.persistGateProfileSnapshot(runID, tc.jobType, tc.gateSpec, tc.gateMeta)

			tc.assertFn(t, path)
		})
	}
}

func TestGateStackPersistAndLoad(t *testing.T) {
	type persistCall struct {
		meta *contracts.BuildGateStageMetadata
	}

	cases := []struct {
		name         string
		seedStack    string
		persistCalls []persistCall
		wantStack    contracts.MigStack
	}{
		{
			name: "WritesStack",
			persistCalls: []persistCall{
				{meta: &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Language: "java", Tool: "maven", Passed: true}}}},
			},
			wantStack: contracts.MigStackJavaMaven,
		},
		{
			name: "Idempotent",
			persistCalls: []persistCall{
				{meta: &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Language: "java", Tool: "maven", Passed: true}}}},
				{meta: &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Language: "java", Tool: "gradle", Passed: true}}}},
			},
			wantStack: contracts.MigStackJavaMaven,
		},
		{
			name:      "LoadsExisting",
			seedStack: "java-gradle",
			wantStack: contracts.MigStackJavaGradle,
		},
		{
			name:      "DefaultsToUnknown",
			wantStack: contracts.MigStackUnknown,
		},
		{
			name: "RoundTrip",
			persistCalls: []persistCall{
				{meta: &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Language: "java", Tool: "gradle", Passed: false}}}},
			},
			wantStack: contracts.MigStackJavaGradle,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cacheHome := t.TempDir()
			t.Setenv("PLOYD_CACHE_HOME", cacheHome)

			rc := &runController{cfg: Config{}}
			runID := types.RunID("run-stack-" + tc.name)

			if tc.seedStack != "" {
				runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
				if err := os.MkdirAll(runDir, 0o755); err != nil {
					t.Fatalf("mkdir runDir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(runDir, "build-gate-stack.txt"), []byte(tc.seedStack), 0o644); err != nil {
					t.Fatalf("write stack file: %v", err)
				}
			}

			for _, call := range tc.persistCalls {
				rc.persistGateStack(runID, call.meta)
			}

			got := rc.loadPersistedStack(runID)
			if got != tc.wantStack {
				t.Errorf("loadPersistedStack() = %q, want %q", got, tc.wantStack)
			}
		})
	}
}

// TestBuildGateJobStats_IncludesJobMeta verifies that gate job stats embed
// JobMeta so that jobs.meta can carry structured gate metadata.
func TestBuildGateJobStats_IncludesJobMeta(t *testing.T) {
	t.Parallel()

	rc := &runController{cfg: Config{}}

	gateMeta := &contracts.BuildGateStageMetadata{
		LogDigest: testLogDigest(1),
		StaticChecks: []contracts.BuildGateStaticCheckReport{
			{Tool: "maven", Passed: true},
		},
	}

	stats := rc.buildGateJobStats(gateMeta, 250*time.Millisecond)

	var decoded struct {
		JobMeta *contracts.JobMeta `json:"job_meta"`
	}
	if err := json.Unmarshal(stats, &decoded); err != nil {
		t.Fatalf("failed to unmarshal stats: %v", err)
	}
	if decoded.JobMeta == nil {
		t.Fatalf("expected job_meta key in gate stats, got nil")
	}

	if decoded.JobMeta.Kind != contracts.JobKindGate {
		t.Fatalf("job_meta.Kind = %q, want %q", decoded.JobMeta.Kind, contracts.JobKindGate)
	}
	if decoded.JobMeta.GateMetadata == nil || decoded.JobMeta.GateMetadata.LogDigest != testLogDigest(1) {
		t.Fatalf("job_meta.GateMetadata.LogDigest = %#v, want %q", decoded.JobMeta.GateMetadata, testLogDigest(1))
	}
}

func TestCleanupGateOutDir_RemovesWorkspaceOutputDir(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	gateOutDir := filepath.Join(workspace, step.BuildGateWorkspaceOutDir)
	if err := os.MkdirAll(gateOutDir, 0o755); err != nil {
		t.Fatalf("mkdir gate out dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gateOutDir, "test.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write gate out file: %v", err)
	}

	rc := &runController{cfg: Config{}}
	rc.cleanupGateOutDir(workspace)

	if _, err := os.Stat(gateOutDir); !os.IsNotExist(err) {
		t.Fatalf("expected gate out dir removed, stat err=%v", err)
	}
}

