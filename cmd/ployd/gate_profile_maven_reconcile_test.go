package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type fakeMavenGateProfileReconcileStore struct {
	rows []mavenGateProfileBlobRef
}

func (f *fakeMavenGateProfileReconcileStore) ListMavenGateProfiles(context.Context) ([]mavenGateProfileBlobRef, error) {
	return append([]mavenGateProfileBlobRef(nil), f.rows...), nil
}

func TestRewriteMavenGateProfileCommands(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		buildCommand string
		unitCommand  string
		testsCommand string
		wantChanged  bool
		wantBuild    string
		wantUnit     string
		wantAllTests string
	}{
		{
			name:         "rewrites_default_generated_commands",
			buildCommand: mavenLegacyBuildCommand,
			unitCommand:  mavenLegacyUnitCommand,
			testsCommand: mavenLegacyTestsCommand,
			wantChanged:  true,
			wantBuild:    mavenWrapperConditionalCommand(mavenLegacyBuildCommand),
			wantUnit:     mavenWrapperConditionalCommand(mavenLegacyUnitCommand),
			wantAllTests: mavenWrapperConditionalCommand(mavenLegacyTestsCommand),
		},
		{
			name:         "leaves_custom_commands_unchanged",
			buildCommand: "mvn -B -e -f /workspace/pom.xml verify",
			unitCommand:  "mvn -B -e -f /workspace/pom.xml test",
			testsCommand: "mvn -B -e -f /workspace/pom.xml clean verify",
			wantChanged:  false,
			wantBuild:    "mvn -B -e -f /workspace/pom.xml verify",
			wantUnit:     "mvn -B -e -f /workspace/pom.xml test",
			wantAllTests: "mvn -B -e -f /workspace/pom.xml clean verify",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := mustGateProfileJSON(t, tc.buildCommand, tc.unitCommand, tc.testsCommand)
			got, changed, err := rewriteMavenGateProfileCommands(raw)
			if err != nil {
				t.Fatalf("rewriteMavenGateProfileCommands() error: %v", err)
			}
			if changed != tc.wantChanged {
				t.Fatalf("changed=%v, want %v", changed, tc.wantChanged)
			}

			profile, err := contracts.ParseGateProfileJSON(got)
			if err != nil {
				t.Fatalf("parse rewritten profile: %v", err)
			}
			if got := profile.Targets.Build.Command; got != tc.wantBuild {
				t.Fatalf("build.command=%q, want %q", got, tc.wantBuild)
			}
			if got := profile.Targets.Unit.Command; got != tc.wantUnit {
				t.Fatalf("unit.command=%q, want %q", got, tc.wantUnit)
			}
			if got := profile.Targets.AllTests.Command; got != tc.wantAllTests {
				t.Fatalf("all_tests.command=%q, want %q", got, tc.wantAllTests)
			}
		})
	}
}

func TestReconcileMavenGateProfiles_RewritesDefaultAndRepoRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	bs := bsmock.New()
	store := &fakeMavenGateProfileReconcileStore{
		rows: []mavenGateProfileBlobRef{
			{ID: 1, ObjectKey: "gate-profiles/defaults/java/17/maven/profile.json"},
			{ID: 2, ObjectKey: "gate-profiles/repos/repo-1/sha-1/stack-17/profile.json"},
			{ID: 3, ObjectKey: "gate-profiles/repos/repo-2/sha-2/stack-17/profile.json"},
		},
	}

	defaultRaw := mustGateProfileJSON(t, mavenLegacyBuildCommand, mavenLegacyUnitCommand, mavenLegacyTestsCommand)
	repoRaw := mustGateProfileJSON(t, mavenLegacyBuildCommand, mavenLegacyUnitCommand, mavenLegacyTestsCommand)
	customRaw := mustGateProfileJSON(t, "mvn -B verify", "mvn -B test", "mvn -B clean verify")

	if _, err := bs.Put(ctx, store.rows[0].ObjectKey, contentTypeJSON, defaultRaw); err != nil {
		t.Fatalf("seed default blob: %v", err)
	}
	if _, err := bs.Put(ctx, store.rows[1].ObjectKey, contentTypeJSON, repoRaw); err != nil {
		t.Fatalf("seed repo blob: %v", err)
	}
	if _, err := bs.Put(ctx, store.rows[2].ObjectKey, contentTypeJSON, customRaw); err != nil {
		t.Fatalf("seed custom blob: %v", err)
	}

	stats, err := reconcileMavenGateProfiles(ctx, store, bs)
	if err != nil {
		t.Fatalf("reconcileMavenGateProfiles() error: %v", err)
	}
	if got, want := stats.Scanned, 3; got != want {
		t.Fatalf("stats.scanned=%d, want %d", got, want)
	}
	if got, want := stats.Rewritten, 2; got != want {
		t.Fatalf("stats.rewritten=%d, want %d", got, want)
	}
	if got, want := stats.Unchanged, 1; got != want {
		t.Fatalf("stats.unchanged=%d, want %d", got, want)
	}

	assertProfileCommands(t, bs, store.rows[0].ObjectKey,
		mavenWrapperConditionalCommand(mavenLegacyBuildCommand),
		mavenWrapperConditionalCommand(mavenLegacyUnitCommand),
		mavenWrapperConditionalCommand(mavenLegacyTestsCommand),
	)
	assertProfileCommands(t, bs, store.rows[1].ObjectKey,
		mavenWrapperConditionalCommand(mavenLegacyBuildCommand),
		mavenWrapperConditionalCommand(mavenLegacyUnitCommand),
		mavenWrapperConditionalCommand(mavenLegacyTestsCommand),
	)
	assertProfileCommands(t, bs, store.rows[2].ObjectKey,
		"mvn -B verify",
		"mvn -B test",
		"mvn -B clean verify",
	)
}

func mustGateProfileJSON(t *testing.T, buildCommand, unitCommand, testsCommand string) []byte {
	t.Helper()
	targetNotAttempted := func(command string) *contracts.GateProfileTarget {
		target := &contracts.GateProfileTarget{
			Status: contracts.PrepTargetStatusNotAttempted,
			Env:    map[string]string{},
		}
		if command != "" {
			target.Command = command
		}
		return target
	}
	payload := contracts.GateProfile{
		SchemaVersion: 1,
		RepoID:        "default",
		RunnerMode:    contracts.PrepRunnerModeSimple,
		Stack: contracts.GateProfileStack{
			Language: "java",
			Tool:     "maven",
			Release:  "17",
		},
		Targets: contracts.GateProfileTargets{
			Active:   contracts.GateProfileTargetAllTests,
			Build:    targetNotAttempted(buildCommand),
			Unit:     targetNotAttempted(unitCommand),
			AllTests: targetNotAttempted(testsCommand),
		},
		Orchestration: contracts.GateProfileOrchestration{
			Pre:  []json.RawMessage{},
			Post: []json.RawMessage{},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal gate profile: %v", err)
	}
	if _, err := contracts.ParseGateProfileJSON(raw); err != nil {
		t.Fatalf("validate gate profile: %v", err)
	}
	return raw
}

func assertProfileCommands(t *testing.T, bs *bsmock.Store, key, wantBuild, wantUnit, wantAllTests string) {
	t.Helper()

	raw, ok := bs.GetData(key)
	if !ok {
		t.Fatalf("blob %q not found", key)
	}
	profile, err := contracts.ParseGateProfileJSON(raw)
	if err != nil {
		t.Fatalf("parse profile %q: %v", key, err)
	}

	assertField := func(field, got, want string) {
		t.Helper()
		if got != want {
			t.Fatalf("%s for key=%q got=%q want=%q", field, key, got, want)
		}
	}
	assertField("build.command", profile.Targets.Build.Command, wantBuild)
	assertField("unit.command", profile.Targets.Unit.Command, wantUnit)
	assertField("all_tests.command", profile.Targets.AllTests.Command, wantAllTests)
}

func TestMavenWrapperConditionalCommand(t *testing.T) {
	t.Parallel()

	got := mavenWrapperConditionalCommand(mavenLegacyBuildCommand)
	want := fmt.Sprintf(
		"if [ -f /workspace/mvnw ]; then %s; else %s; fi",
		mavenWrapperCompile,
		mavenLegacyBuildCommand,
	)
	if got != want {
		t.Fatalf("mavenWrapperConditionalCommand()=%q, want %q", got, want)
	}
}
