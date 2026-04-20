package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestResolveGateProfileRepoSHA(t *testing.T) {
	t.Run("prefers repo_sha_out", func(t *testing.T) {
		job := store.Job{
			RepoShaIn:  "0123456789abcdef0123456789abcdef01234567",
			RepoShaOut: "89abcdef0123456789abcdef0123456789abcdef",
		}
		got, err := resolveGateProfileRepoSHA(job)
		if err != nil {
			t.Fatalf("resolveGateProfileRepoSHA() error = %v", err)
		}
		if want := "89abcdef0123456789abcdef0123456789abcdef"; got != want {
			t.Fatalf("resolveGateProfileRepoSHA() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to repo_sha_in", func(t *testing.T) {
		job := store.Job{
			RepoShaIn:  "0123456789abcdef0123456789abcdef01234567",
			RepoShaOut: "",
		}
		got, err := resolveGateProfileRepoSHA(job)
		if err != nil {
			t.Fatalf("resolveGateProfileRepoSHA() error = %v", err)
		}
		if want := "0123456789abcdef0123456789abcdef01234567"; got != want {
			t.Fatalf("resolveGateProfileRepoSHA() = %q, want %q", got, want)
		}
	})
}

func TestResolveGateProfileTarget(t *testing.T) {
	t.Run("pre gate uses configured pre target", func(t *testing.T) {
		spec := []byte(`{
			"apiVersion":"ploy.mig/v1alpha1",
			"kind":"MigRunSpec",
			"steps":[{"image":"example.com/mig:latest"}],
			"build_gate":{"pre":{"target":"build"}}
		}`)
		got, err := resolveGateProfileTarget(spec, domaintypes.JobTypePreGate)
		if err != nil {
			t.Fatalf("resolveGateProfileTarget() error = %v", err)
		}
		if got != contracts.GateProfileTargetBuild {
			t.Fatalf("resolveGateProfileTarget() = %q, want %q", got, contracts.GateProfileTargetBuild)
		}
	})

	t.Run("post gate defaults to all_tests when post target is empty", func(t *testing.T) {
		spec := []byte(`{
			"apiVersion":"ploy.mig/v1alpha1",
			"kind":"MigRunSpec",
			"steps":[{"image":"example.com/mig:latest"}],
			"build_gate":{"post":{}}
		}`)
		got, err := resolveGateProfileTarget(spec, domaintypes.JobTypePostGate)
		if err != nil {
			t.Fatalf("resolveGateProfileTarget() error = %v", err)
		}
		if got != contracts.GateProfileTargetAllTests {
			t.Fatalf("resolveGateProfileTarget() = %q, want %q", got, contracts.GateProfileTargetAllTests)
		}
	})
}

func TestBuildSuccessfulGateProfilePayload(t *testing.T) {
	repoID := domaintypes.RepoID("repo1234")
	stack := gateProfileStackRow{
		ID:      3,
		Lang:    "java",
		Tool:    "gradle",
		Release: "11",
	}
	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Language: "java",
			Tool:     "gradle",
			Passed:   true,
		}},
		ExecutedCommand: "./gradlew -q --stacktrace --build-cache build -x test -p /workspace",
		Detected: &contracts.StackExpectation{
			Language: "java",
			Tool:     "gradle",
			Release:  "11",
		},
	}

	raw, err := buildSuccessfulGateProfilePayload(repoID, contracts.GateProfileTargetBuild, stack, meta)
	if err != nil {
		t.Fatalf("buildSuccessfulGateProfilePayload() error = %v", err)
	}

	profile, err := contracts.ParseGateProfileJSON(raw)
	if err != nil {
		t.Fatalf("ParseGateProfileJSON() error = %v", err)
	}
	if got, want := profile.RepoID, repoID.String(); got != want {
		t.Fatalf("repo_id = %q, want %q", got, want)
	}
	if got, want := profile.Targets.Active, contracts.GateProfileTargetBuild; got != want {
		t.Fatalf("targets.active = %q, want %q", got, want)
	}
	if profile.Targets.Build == nil || profile.Targets.Build.Status != contracts.PrepTargetStatusPassed {
		t.Fatalf("targets.build = %#v, want passed target", profile.Targets.Build)
	}
	if got, want := profile.Targets.Build.Command, "./gradlew -q --stacktrace --build-cache build -x test -p /workspace"; got != want {
		t.Fatalf("targets.build.command = %q, want %q", got, want)
	}
	if got, want := profile.Stack.Tool, "gradle"; got != want {
		t.Fatalf("stack.tool = %q, want %q", got, want)
	}
}

func TestBuildSuccessfulGateProfilePayload_RequiresExecutedCommand(t *testing.T) {
	repoID := domaintypes.RepoID("repo1234")
	stack := gateProfileStackRow{
		ID:      3,
		Lang:    "java",
		Tool:    "gradle",
		Release: "11",
	}
	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Language: "java",
			Tool:     "gradle",
			Passed:   true,
		}},
		Detected: &contracts.StackExpectation{
			Language: "java",
			Tool:     "gradle",
			Release:  "11",
		},
	}

	_, err := buildSuccessfulGateProfilePayload(repoID, contracts.GateProfileTargetBuild, stack, meta)
	if err == nil {
		t.Fatal("expected error when executed command is missing")
	}
	if got, want := err.Error(), "gate metadata executed_command is required for successful gate profile persistence"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}
