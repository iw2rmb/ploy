package step

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestDockerGateExecutor_PrepOverrideCommandPrecedence(t *testing.T) {
	t.Parallel()

	executor, rt, workspace := newDockerGateTestHarness(t)
	spec := &contracts.StepGateSpec{
		Enabled: true,
		GateProfile: &contracts.BuildGateProfileOverride{
			Command: contracts.CommandSpec{Shell: "echo prep-gate"},
		},
	}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	got := rt.captured.Command
	want := []string{"/bin/sh", "-c", "echo prep-gate"}
	if len(got) != len(want) {
		t.Fatalf("captured command length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("captured command[%d] = %q, want %q", i, got[i], v)
		}
	}
}

func TestDockerGateExecutor_PrepOverrideEnvPrecedence(t *testing.T) {
	t.Parallel()

	executor, rt, workspace := newDockerGateTestHarness(t)
	spec := &contracts.StepGateSpec{
		Enabled: true,
		Env: map[string]string{
			"A": "base",
			"B": "base",
		},
		GateProfile: &contracts.BuildGateProfileOverride{
			Command: contracts.CommandSpec{Shell: "echo prep-gate"},
			Env: map[string]string{
				"B": "prep",
				"C": "prep",
			},
		},
	}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if got := rt.captured.Env["A"]; got != "base" {
		t.Fatalf("env[A] = %q, want %q", got, "base")
	}
	if got := rt.captured.Env["B"]; got != "prep" {
		t.Fatalf("env[B] = %q, want %q", got, "prep")
	}
	if got := rt.captured.Env["C"]; got != "prep" {
		t.Fatalf("env[C] = %q, want %q", got, "prep")
	}
}

func TestDockerGateExecutor_SkipShortCircuitsExecution(t *testing.T) {
	t.Parallel()

	executor, rt, _ := newDockerGateTestHarness(t)
	spec := &contracts.StepGateSpec{
		Enabled: true,
		Skip: &contracts.BuildGateSkipMetadata{
			Enabled:         true,
			SourceProfileID: 55,
			MatchedTarget:   contracts.GateProfileTargetBuild,
		},
		GateProfile: &contracts.BuildGateProfileOverride{
			Stack: &contracts.GateProfileStack{Language: "java", Tool: "maven", Release: "17"},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, "")
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if rt.createCalled || rt.startCalled || rt.waitCalled {
		t.Fatalf("expected no container execution for skip, got runtime=%+v", rt)
	}
	if meta == nil || meta.Skip == nil {
		t.Fatal("expected skip metadata in gate result")
	}
	if got, want := meta.Skip.SourceProfileID, int64(55); got != want {
		t.Fatalf("skip.source_profile_id=%d, want %d", got, want)
	}
	if got, want := meta.Skip.MatchedTarget, contracts.GateProfileTargetBuild; got != want {
		t.Fatalf("skip.matched_target=%q, want %q", got, want)
	}
	if meta.Detected == nil || meta.Detected.Tool != "maven" {
		t.Fatalf("expected detected stack from gate profile stack, got %+v", meta.Detected)
	}
	if len(meta.StaticChecks) == 0 || !meta.StaticChecks[0].Passed {
		t.Fatalf("expected passing static check metadata, got %+v", meta.StaticChecks)
	}
}

func TestDockerGateExecutor_TargetLockUnsupportedCancels(t *testing.T) {
	t.Parallel()

	executor, rt, workspace := newDockerGateTestHarness(t)
	spec := &contracts.StepGateSpec{
		Enabled:           true,
		Target:            contracts.GateProfileTargetUnit,
		EnforceTargetLock: true,
		GateProfile: &contracts.BuildGateProfileOverride{
			Command: contracts.CommandSpec{Shell: "echo candidate"},
			Target:  contracts.GateProfileTargetAllTests,
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrRepoCancelled) {
		t.Fatalf("error=%v, want ErrRepoCancelled", err)
	}
	if rt.createCalled {
		t.Fatal("expected container Create NOT to be called")
	}
	if meta == nil || len(meta.LogFindings) == 0 {
		t.Fatal("expected log findings in metadata")
	}
	if got, want := meta.LogFindings[0].Code, "BUILD_GATE_TARGET_UNSUPPORTED"; got != want {
		t.Fatalf("log_findings[0].code=%q, want %q", got, want)
	}
}

func TestDockerGateExecutor_TargetPinIgnoresPrepOverrideFromAnotherTarget(t *testing.T) {
	t.Parallel()

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)
	workspace := createGradleWorkspace(t, "11")

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Target:  contracts.GateProfileTargetBuild,
		GateProfile: &contracts.BuildGateProfileOverride{
			Command: contracts.CommandSpec{Shell: "echo prep-all-tests"},
			Target:  contracts.GateProfileTargetAllTests,
			Stack:   &contracts.GateProfileStack{Language: "java", Tool: "maven", Release: "11"},
		},
	}

	meta, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected gate metadata, got nil")
	}
	if !rt.createCalled {
		t.Fatal("expected container Create to be called")
	}
	if len(rt.captured.Command) != 3 {
		t.Fatalf("expected 3-element command, got %v", rt.captured.Command)
	}
	cmd := rt.captured.Command[2]
	if !strings.Contains(cmd, "build -x test -p /workspace") {
		t.Fatalf("expected pinned build target command, got %q", cmd)
	}
}

func TestDockerGateExecutor_ReportsRuntimeImageOnPrepStackMismatch(t *testing.T) {
	t.Parallel()

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)
	workspace := createGradleWorkspace(t, "11")

	var observedImage string
	ctx := WithGateRuntimeImageObserver(context.Background(), func(_ context.Context, image string) {
		observedImage = image
	})

	spec := &contracts.StepGateSpec{
		Enabled: true,
		GateProfile: &contracts.BuildGateProfileOverride{
			Command: contracts.CommandSpec{Shell: "echo prep-gradle"},
			Target:  contracts.GateProfileTargetAllTests,
			Stack:   &contracts.GateProfileStack{Language: "java", Tool: "maven", Release: "11"},
		},
	}

	meta, err := executor.Execute(ctx, spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected gate metadata, got nil")
	}
	if rt.createCalled {
		t.Fatal("expected container Create NOT to be called on prep stack mismatch")
	}
	if strings.TrimSpace(meta.RuntimeImage) == "" {
		t.Fatal("expected RuntimeImage to be set on prep stack mismatch")
	}
	if got, want := observedImage, meta.RuntimeImage; got != want {
		t.Fatalf("observed runtime image = %q, want %q", got, want)
	}
}
