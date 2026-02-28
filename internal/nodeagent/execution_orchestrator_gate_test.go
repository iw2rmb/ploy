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

func TestPersistGateProfileSnapshot_UsesGeneratedProfile(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-profile-generated")
	const generated = `{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"java","tool":"maven"},"targets":{"build":{"status":"passed","command":"mvn -q -DskipTests compile","env":{}},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}`

	rc.persistGateProfileSnapshot(
		runID,
		types.JobTypePreGate,
		&contracts.StepGateSpec{RepoID: types.MigRepoID("repo_1")},
		&contracts.BuildGateStageMetadata{GeneratedGateProfile: json.RawMessage(generated)},
	)

	path := filepath.Join(cacheHome, "ploy", "run", runID.String(), "build-gate-profile.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read profile snapshot: %v", err)
	}
	if string(data) != generated {
		t.Fatalf("snapshot profile = %q, want %q", string(data), generated)
	}
}

func TestPersistGateProfileSnapshot_DerivesFromOverride(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-profile-derived")

	rc.persistGateProfileSnapshot(
		runID,
		types.JobTypePreGate,
		&contracts.StepGateSpec{
			RepoID: types.MigRepoID("repo_2"),
			GateProfile: &contracts.BuildGateProfileOverride{
				Command: contracts.CommandSpec{Shell: "mvn -q -DskipTests compile"},
				Env:     map[string]string{"MAVEN_OPTS": "-Xmx2g"},
				Stack:   &contracts.GateProfileStack{Language: "java", Tool: "maven", Release: "21"},
			},
		},
		nil,
	)

	path := filepath.Join(cacheHome, "ploy", "run", runID.String(), "build-gate-profile.json")
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
	if profile.Targets.Build == nil || profile.Targets.Build.Command != "mvn -q -DskipTests compile" {
		t.Fatalf("targets.build.command = %#v, want mvn command", profile.Targets.Build)
	}
	if profile.Targets.Build.Env["MAVEN_OPTS"] != "-Xmx2g" {
		t.Fatalf("targets.build.env[MAVEN_OPTS] = %q, want %q", profile.Targets.Build.Env["MAVEN_OPTS"], "-Xmx2g")
	}
}

func TestPersistGateProfileSnapshot_RemovesStaleSnapshot(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-profile-stale")
	path := filepath.Join(cacheHome, "ploy", "run", runID.String(), "build-gate-profile.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatalf("write stale snapshot: %v", err)
	}

	rc.persistGateProfileSnapshot(runID, types.JobTypePreGate, nil, nil)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected stale snapshot removed, stat err=%v", err)
	}
}

// TestPersistGateStack_WritesStack verifies that persistGateStack writes the
// detected stack to a file under the run directory for later retrieval.
func TestPersistGateStack_WritesStack(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-persist")

	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{Language: "java", Tool: "maven", Passed: true}},
	}

	rc.persistGateStack(runID, meta)

	stackPath := filepath.Join(cacheHome, "ploy", "run", runID.String(), "build-gate-stack.txt")
	data, err := os.ReadFile(stackPath)
	if err != nil {
		t.Fatalf("failed to read persisted stack file: %v", err)
	}

	got := string(data)
	if got != "java-maven" {
		t.Errorf("persisted stack = %q, want %q", got, "java-maven")
	}
}

// TestPersistGateStack_Idempotent verifies that persistGateStack only writes
// the first detection and ignores subsequent calls.
func TestPersistGateStack_Idempotent(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-idempotent")

	rc.persistGateStack(runID, &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Language: "java", Tool: "maven", Passed: true}}})
	rc.persistGateStack(runID, &contracts.BuildGateStageMetadata{StaticChecks: []contracts.BuildGateStaticCheckReport{{Language: "java", Tool: "gradle", Passed: true}}})

	stackPath := filepath.Join(cacheHome, "ploy", "run", runID.String(), "build-gate-stack.txt")
	data, err := os.ReadFile(stackPath)
	if err != nil {
		t.Fatalf("failed to read persisted stack file: %v", err)
	}

	got := string(data)
	if got != "java-maven" {
		t.Errorf("persisted stack = %q, want first stack %q", got, "java-maven")
	}
}

// TestLoadPersistedStack_ReturnsStack verifies that loadPersistedStack reads
// the persisted stack from the run directory.
func TestLoadPersistedStack_ReturnsStack(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-load")

	runDir := filepath.Join(cacheHome, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}
	stackPath := filepath.Join(runDir, "build-gate-stack.txt")
	if err := os.WriteFile(stackPath, []byte("java-gradle"), 0o644); err != nil {
		t.Fatalf("write stack file: %v", err)
	}

	got := rc.loadPersistedStack(runID)
	if got != contracts.ModStackJavaGradle {
		t.Errorf("loadPersistedStack() = %q, want %q", got, contracts.ModStackJavaGradle)
	}
}

// TestLoadPersistedStack_DefaultsToUnknown verifies that loadPersistedStack
// returns ModStackUnknown when no stack file exists.
func TestLoadPersistedStack_DefaultsToUnknown(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-missing")

	got := rc.loadPersistedStack(runID)
	if got != contracts.ModStackUnknown {
		t.Errorf("loadPersistedStack() = %q, want %q", got, contracts.ModStackUnknown)
	}
}

// TestPersistAndLoadGateStack_RoundTrip verifies the complete flow of persisting
// a stack during gate execution and loading it for mig/healing execution.
func TestPersistAndLoadGateStack_RoundTrip(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	rc := &runController{cfg: Config{}}
	runID := types.RunID("run-stack-roundtrip")

	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{Language: "java", Tool: "gradle", Passed: false}},
	}

	rc.persistGateStack(runID, meta)

	got := rc.loadPersistedStack(runID)
	if got != contracts.ModStackJavaGradle {
		t.Errorf("round-trip stack = %q, want %q", got, contracts.ModStackJavaGradle)
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
	if decoded.JobMeta.Gate == nil || decoded.JobMeta.Gate.LogDigest != testLogDigest(1) {
		t.Fatalf("job_meta.Gate.LogDigest = %#v, want %q", decoded.JobMeta.Gate, testLogDigest(1))
	}
}

func TestRunRouterForGateFailure_SetsBugSummary(t *testing.T) {
	t.Parallel()

	rc := &runController{cfg: Config{ServerURL: "http://localhost:9999"}}

	workspace := t.TempDir()

	const wantBugSummary = "javac: cannot find symbol FooBar"

	mc := &mockRouterContainerRuntime{}
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		if strings.Contains(spec.Image, "router") {
			if got, want := spec.Env["PLOY_GATE_PHASE"], "pre_gate"; got != want {
				t.Fatalf("router env PLOY_GATE_PHASE = %q, want %q", got, want)
			}
			if got, want := spec.Env["PLOY_LOOP_KIND"], "healing"; got != want {
				t.Fatalf("router env PLOY_LOOP_KIND = %q, want %q", got, want)
			}
			for _, m := range spec.Mounts {
				if m.Target == "/out" {
					_ = os.WriteFile(filepath.Join(m.Source, "codex-last.txt"), []byte(`{"bug_summary":"`+wantBugSummary+`","error_kind":"infra","strategy_id":"infra-default","confidence":0.8,"reason":"docker socket missing","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}`+"\n"), 0o644)
				}
			}
		}
		return step.ContainerHandle("mock-" + spec.Image), nil
	}

	runner := step.Runner{Containers: mc}

	req := StartRunRequest{
		RunID:   types.RunID("run-router-gate"),
		JobID:   types.JobID("job-router-gate"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		JobType: types.JobTypePreGate,
	}

	typedOpts := RunOptions{
		HealingSelector: &contracts.HealingSpec{
			ByErrorKind: map[string]contracts.HealingActionSpec{
				"infra": {Image: contracts.JobImage{Universal: "test/healer:latest"}},
			},
		},
		Healing: &HealingConfig{
			Retries: 1,
			Mod: ModContainerSpec{
				Image: contracts.JobImage{Universal: "test/healer:latest"},
			},
		},
		Router: &ModContainerSpec{
			Image: contracts.JobImage{Universal: "test/router:latest"},
		},
	}

	gateResult := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
		LogsText:     "[ERROR] build failed\n",
	}

	rc.runRouterForGateFailure(context.Background(), runner, req, typedOpts, workspace, gateResult)

	if gateResult.BugSummary != wantBugSummary {
		t.Fatalf("gateResult.BugSummary = %q, want %q", gateResult.BugSummary, wantBugSummary)
	}
	if gateResult.Recovery == nil {
		t.Fatal("gateResult.Recovery is nil, want classifier metadata")
	}
	if got, want := gateResult.Recovery.ErrorKind, "infra"; got != want {
		t.Fatalf("gateResult.Recovery.ErrorKind = %q, want %q", got, want)
	}
	if got, want := gateResult.Recovery.StrategyID, "infra-default"; got != want {
		t.Fatalf("gateResult.Recovery.StrategyID = %q, want %q", got, want)
	}
	if gateResult.Recovery.Confidence == nil || *gateResult.Recovery.Confidence != 0.8 {
		t.Fatalf("gateResult.Recovery.Confidence = %#v, want %v", gateResult.Recovery.Confidence, 0.8)
	}
	if got, want := gateResult.Recovery.Reason, "docker socket missing"; got != want {
		t.Fatalf("gateResult.Recovery.Reason = %q, want %q", got, want)
	}
	if len(gateResult.Recovery.Expectations) == 0 {
		t.Fatal("gateResult.Recovery.Expectations is empty")
	}
}

func TestRunRouterForGateFailure_DefaultsToUnknownOnInvalidClassifier(t *testing.T) {
	t.Parallel()

	rc := &runController{cfg: Config{ServerURL: "http://localhost:9999"}}
	workspace := t.TempDir()
	mc := &mockRouterContainerRuntime{}
	mc.createFn = func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
		if strings.Contains(spec.Image, "router") {
			for _, m := range spec.Mounts {
				if m.Target == "/out" {
					_ = os.WriteFile(filepath.Join(m.Source, "codex-last.txt"), []byte(`{"error_kind":"routing"}`+"\n"), 0o644)
				}
			}
		}
		return step.ContainerHandle("mock-" + spec.Image), nil
	}
	runner := step.Runner{Containers: mc}
	req := StartRunRequest{
		RunID:   types.RunID("run-router-default"),
		JobID:   types.JobID("job-router-default"),
		RepoURL: types.RepoURL("https://gitlab.com/test/repo.git"),
		JobType: types.JobTypeReGate,
	}
	typedOpts := RunOptions{
		HealingSelector: &contracts.HealingSpec{
			ByErrorKind: map[string]contracts.HealingActionSpec{
				"infra": {Image: contracts.JobImage{Universal: "test/healer:latest"}},
			},
		},
		Healing: &HealingConfig{
			Retries: 1,
			Mod: ModContainerSpec{
				Image: contracts.JobImage{Universal: "test/healer:latest"},
			},
		},
		Router: &ModContainerSpec{
			Image: contracts.JobImage{Universal: "test/router:latest"},
		},
	}
	gateResult := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{Tool: "maven", Passed: false}},
		LogsText:     "[ERROR] build failed\n",
	}

	rc.runRouterForGateFailure(context.Background(), runner, req, typedOpts, workspace, gateResult)

	if gateResult.Recovery == nil {
		t.Fatal("gateResult.Recovery is nil")
	}
	if got, want := gateResult.Recovery.LoopKind, "healing"; got != want {
		t.Fatalf("LoopKind = %q, want %q", got, want)
	}
	if got, want := gateResult.Recovery.ErrorKind, "unknown"; got != want {
		t.Fatalf("ErrorKind = %q, want %q", got, want)
	}
}

type mockRouterContainerRuntime struct {
	createFn func(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error)
	startFn  func(ctx context.Context, handle step.ContainerHandle) error
	waitFn   func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error)
	logsFn   func(ctx context.Context, handle step.ContainerHandle) ([]byte, error)
	removeFn func(ctx context.Context, handle step.ContainerHandle) error
}

func (m *mockRouterContainerRuntime) Create(ctx context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
	if m.createFn != nil {
		return m.createFn(ctx, spec)
	}
	return step.ContainerHandle("mock"), nil
}

func (m *mockRouterContainerRuntime) Start(ctx context.Context, handle step.ContainerHandle) error {
	if m.startFn != nil {
		return m.startFn(ctx, handle)
	}
	return nil
}

func (m *mockRouterContainerRuntime) Wait(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
	if m.waitFn != nil {
		return m.waitFn(ctx, handle)
	}
	return step.ContainerResult{ExitCode: 0}, nil
}

func (m *mockRouterContainerRuntime) Logs(ctx context.Context, handle step.ContainerHandle) ([]byte, error) {
	if m.logsFn != nil {
		return m.logsFn(ctx, handle)
	}
	return []byte{}, nil
}

func (m *mockRouterContainerRuntime) Remove(ctx context.Context, handle step.ContainerHandle) error {
	if m.removeFn != nil {
		return m.removeFn(ctx, handle)
	}
	return nil
}
