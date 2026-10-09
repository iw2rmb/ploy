package step

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	workspaceutil "github.com/iw2rmb/ploy/internal/testutil/workspace"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// findMount returns the first mount whose Target equals target.
func findMount(mounts []ContainerMount, target string) (ContainerMount, bool) {
	for _, m := range mounts {
		if m.Target == target {
			return m, true
		}
	}
	return ContainerMount{}, false
}

// requireMount fails the test unless a mount with the given target exists and
// matches the expected source and readOnly flag.
func requireMount(t *testing.T, mounts []ContainerMount, target, source string, readOnly bool) {
	t.Helper()
	m, ok := findMount(mounts, target)
	if !ok {
		t.Fatalf("mount %q not found in %+v", target, mounts)
	}
	if m.Source != source {
		t.Fatalf("mount %q: source=%q, want %q", target, m.Source, source)
	}
	if m.ReadOnly != readOnly {
		t.Fatalf("mount %q: ReadOnly=%v, want %v", target, m.ReadOnly, readOnly)
	}
}

// requireNoMount fails the test if any mount has the given target.
func requireNoMount(t *testing.T, mounts []ContainerMount, target string) {
	t.Helper()
	if _, ok := findMount(mounts, target); ok {
		t.Fatalf("unexpected mount with target %q in %+v", target, mounts)
	}
}

// requireErrContains fails the test unless err is non-nil and its message
// contains the want substring. When want is empty, only non-nil-ness is checked.
func requireErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if want != "" && !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q should contain %q", err.Error(), want)
	}
}

// testContainerRuntime is a configurable mock for ContainerRuntime.
// Fields can be set to customize behavior; nil fields use sensible defaults.
// Boolean tracking fields record whether each method was called.
type testContainerRuntime struct {
	createFn func(ctx context.Context, spec ContainerSpec) (ContainerHandle, error)
	startFn  func(ctx context.Context, handle ContainerHandle) error
	waitFn   func(ctx context.Context, handle ContainerHandle) (ContainerResult, error)
	logsFn   func(ctx context.Context, handle ContainerHandle) ([]byte, error)

	// captured holds the last ContainerSpec passed to Create.
	captured     ContainerSpec
	createCalled bool
	startCalled  bool
	waitCalled   bool
	logsCalled   bool
}

func (m *testContainerRuntime) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	m.createCalled = true
	m.captured = spec
	if m.createFn != nil {
		return m.createFn(ctx, spec)
	}
	return ContainerHandle("mock"), nil
}

func (m *testContainerRuntime) Start(ctx context.Context, handle ContainerHandle) error {
	m.startCalled = true
	if m.startFn != nil {
		return m.startFn(ctx, handle)
	}
	return nil
}

func (m *testContainerRuntime) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	m.waitCalled = true
	if m.waitFn != nil {
		return m.waitFn(ctx, handle)
	}
	return ContainerResult{ExitCode: 0}, nil
}

func (m *testContainerRuntime) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	m.logsCalled = true
	if m.logsFn != nil {
		return m.logsFn(ctx, handle)
	}
	return nil, nil
}

func newTestJobMounts(t *testing.T, jobType types.JobType) JobMounts {
	t.Helper()
	root := t.TempDir()
	return JobMounts{
		Cache:        filepath.Join(root, "cache"),
		Home:         filepath.Join(root, "home"),
		In:           filepath.Join(root, "in"),
		Out:          filepath.Join(root, "out"),
		Staging:      filepath.Join(root, "staging"),
		Tmp:          filepath.Join(root, "tmp"),
		Share:        filepath.Join(root, "share"),
		RuntimeShare: filepath.Join(root, "runtime-share"),
		NodeCache:    filepath.Join(root, "node-cache"),
		CommonConfig: filepath.Join(root, "job-config", "common"),
		JobConfig:    filepath.Join(root, "job-config", jobType.String()),
		JobType:      jobType,
	}
}

func newTestGateJobMounts(t *testing.T) JobMounts {
	t.Helper()
	return newTestJobMounts(t, types.JobTypePreGate)
}

func newTestMigJobMounts(t *testing.T) JobMounts {
	t.Helper()
	return newTestJobMounts(t, types.JobTypeMig)
}

func buildContainerSpecForTest(
	runID types.RunID,
	jobID types.JobID,
	manifest contracts.StepManifest,
	workspace string,
	outDir string,
	inDir string,
	shareDir string,
	tmpDir string,
	stagingDir string,
) (ContainerSpec, error) {
	root := filepath.Join("/tmp", "ploy-step-tests", jobID.String())
	mounts := JobMounts{
		Cache:        filepath.Join(root, "cache"),
		Home:         filepath.Join(root, "home"),
		In:           filepath.Join(root, "in"),
		Out:          filepath.Join(root, "out"),
		Staging:      filepath.Join(root, "staging"),
		Tmp:          filepath.Join(root, "tmp"),
		Share:        filepath.Join(root, "share"),
		RuntimeShare: filepath.Join(root, "runtime-share"),
		NodeCache:    filepath.Join(root, "node-cache"),
		CommonConfig: filepath.Join(root, "job-config", "common"),
		JobConfig:    filepath.Join(root, "job-config", "mig"),
		JobType:      types.JobTypeMig,
	}
	if outDir != "" {
		mounts.Out = outDir
	}
	if inDir != "" {
		mounts.In = inDir
	}
	if shareDir != "" {
		mounts.Share = shareDir
	}
	if tmpDir != "" {
		mounts.Tmp = tmpDir
	}
	if stagingDir != "" {
		mounts.Staging = stagingDir
	}
	return buildContainerSpec(runID, jobID, manifest, workspace, mounts)
}

// newGateTestHarness creates a GateExecutor backed by a
// testContainerRuntime and a temporary Maven workspace. Returns the executor,
// the runtime (for assertions), and the workspace path.
func newGateTestHarness(t *testing.T) (GateExecutor, *testContainerRuntime, string) {
	t.Helper()
	rt := &testContainerRuntime{}
	executor := NewGateExecutor(rt)
	workspace := createMavenWorkspace(t, "17")
	return executor, rt, workspace
}

func createMavenWorkspace(t *testing.T, javaVersion string) string {
	t.Helper()
	return workspaceutil.Maven(t, javaVersion)
}

func createMavenWorkspaceNoJavaVersion(t *testing.T) string {
	t.Helper()
	return workspaceutil.MavenNoJavaVersion(t)
}

func createGradleWorkspace(t *testing.T, javaVersion string) string {
	t.Helper()
	return workspaceutil.Gradle(t, javaVersion)
}

func createGoWorkspace(t *testing.T, goVersion string) string {
	t.Helper()
	return workspaceutil.Go(t, goVersion)
}

func createPythonWorkspace(t *testing.T, pythonVersion string) string {
	t.Helper()
	return workspaceutil.Python(t, pythonVersion)
}
