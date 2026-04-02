package nodeagent

import (
	"context"
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

// runRouterTestCase executes runRouterForGateFailure with shared boilerplate
// and returns the populated gateResult for assertion.
func runRouterTestCase(t *testing.T, routerPayload string, jobType types.JobType, routerAmata *contracts.AmataRunSpec, envCheck func(t *testing.T, env map[string]string)) *contracts.BuildGateStageMetadata {
	t.Helper()

	rc := &runController{cfg: Config{ServerURL: "http://localhost:9999"}}
	workspace := t.TempDir()

	mc := &mockContainerRuntime{}
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		if strings.Contains(spec.Image, "router") {
			if envCheck != nil {
				envCheck(t, spec.Env)
			}
			for _, m := range spec.Mounts {
				if m.Target == "/out" {
					_ = os.WriteFile(filepath.Join(m.Source, "codex-last.txt"), []byte(routerPayload+"\n"), 0o644)
				}
			}
		}
		return step.ContainerHandle("mock-" + spec.Image), nil
	}

	router := &MigContainerSpec{Image: contracts.JobImage{Universal: "test/router:latest"}}
	if routerAmata != nil {
		router.Amata = routerAmata
	}

	runner := step.Runner{Containers: mc}
	req := StartRunRequest{
		RunID:   types.RunID("run-router-test"),
		JobID:   types.JobID("job-router-test"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		JobType: jobType,
	}
	typedOpts := RunOptions{
		HealingSelector: &contracts.HealingSpec{
			ByErrorKind: map[string]contracts.HealingActionSpec{
				"infra": {Image: contracts.JobImage{Universal: "test/healer:latest"}},
			},
		},
		Healing: &HealingConfig{
			Retries: 1,
			Mig:     MigContainerSpec{Image: contracts.JobImage{Universal: "test/healer:latest"}},
		},
		Router: router,
	}
	gateResult := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
		LogsText:     "[ERROR] build failed\n",
	}

	rc.runRouterForGateFailure(context.Background(), runner, req, typedOpts, workspace, gateResult)
	return gateResult
}

func TestRunRouterForGateFailure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		routerPayload string
		jobType       types.JobType
		routerAmata   *contracts.AmataRunSpec
		envCheck      func(t *testing.T, env map[string]string)
		assertFn      func(t *testing.T, result *contracts.BuildGateStageMetadata)
	}{
		{
			name:          "sets bug summary and recovery metadata",
			routerPayload: `{"bug_summary":"javac: cannot find symbol FooBar","error_kind":"infra","strategy_id":"infra-default","confidence":0.8,"reason":"docker socket missing","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}`,
			jobType:       types.JobTypePreGate,
			envCheck: func(t *testing.T, env map[string]string) {
				t.Helper()
				if got, want := env["PLOY_GATE_PHASE"], "pre_gate"; got != want {
					t.Fatalf("PLOY_GATE_PHASE = %q, want %q", got, want)
				}
				if got, want := env["PLOY_LOOP_KIND"], "healing"; got != want {
					t.Fatalf("PLOY_LOOP_KIND = %q, want %q", got, want)
				}
			},
			assertFn: func(t *testing.T, result *contracts.BuildGateStageMetadata) {
				t.Helper()
				if result.BugSummary != "javac: cannot find symbol FooBar" {
					t.Fatalf("BugSummary = %q, want %q", result.BugSummary, "javac: cannot find symbol FooBar")
				}
				if result.Recovery == nil {
					t.Fatal("Recovery is nil")
				}
				if got, want := result.Recovery.ErrorKind, "infra"; got != want {
					t.Fatalf("ErrorKind = %q, want %q", got, want)
				}
				if got, want := result.Recovery.StrategyID, "infra-default"; got != want {
					t.Fatalf("StrategyID = %q, want %q", got, want)
				}
				if result.Recovery.Confidence == nil || *result.Recovery.Confidence != 0.8 {
					t.Fatalf("Confidence = %#v, want 0.8", result.Recovery.Confidence)
				}
				if got, want := result.Recovery.Reason, "docker socket missing"; got != want {
					t.Fatalf("Reason = %q, want %q", got, want)
				}
				if len(result.Recovery.Expectations) == 0 {
					t.Fatal("Expectations is empty")
				}
			},
		},
		{
			name:          "amata router cmd persists after parse",
			routerPayload: `{"error_kind":"infra","strategy_id":"infra-default","confidence":0.9,"reason":"docker socket missing"}`,
			jobType:       types.JobTypePreGate,
			routerAmata: &contracts.AmataRunSpec{
				Spec: "task: route",
				Set: []contracts.AmataSetParam{
					{Param: "repo", Value: "svc"},
					{Param: "env", Value: "ci"},
				},
			},
			assertFn: func(t *testing.T, result *contracts.BuildGateStageMetadata) {
				t.Helper()
				if result.Recovery == nil {
					t.Fatal("Recovery is nil")
				}
				if got, want := result.Recovery.ErrorKind, "infra"; got != want {
					t.Fatalf("ErrorKind = %q, want %q", got, want)
				}
				wantCmd := []string{"amata", "run", "/in/amata.yaml", "--set", "repo=svc", "--set", "env=ci"}
				if len(result.Recovery.RouterCmd) != len(wantCmd) {
					t.Fatalf("RouterCmd len = %d, want %d: %v", len(result.Recovery.RouterCmd), len(wantCmd), result.Recovery.RouterCmd)
				}
				for i, want := range wantCmd {
					if got := result.Recovery.RouterCmd[i]; got != want {
						t.Fatalf("RouterCmd[%d] = %q, want %q", i, got, want)
					}
				}
			},
		},
		{
			name:          "defaults to unknown on invalid classifier",
			routerPayload: `{"error_kind":"routing"}`,
			jobType:       types.JobTypeReGate,
			assertFn: func(t *testing.T, result *contracts.BuildGateStageMetadata) {
				t.Helper()
				if result.Recovery == nil {
					t.Fatal("Recovery is nil")
				}
				if got, want := result.Recovery.LoopKind, "healing"; got != want {
					t.Fatalf("LoopKind = %q, want %q", got, want)
				}
				if got, want := result.Recovery.ErrorKind, "unknown"; got != want {
					t.Fatalf("ErrorKind = %q, want %q", got, want)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := runRouterTestCase(t, tc.routerPayload, tc.jobType, tc.routerAmata, tc.envCheck)
			tc.assertFn(t, result)
		})
	}
}

// mockContainerRuntime is defined in testutil_docker_test.go.
