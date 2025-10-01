package runner_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func TestRunAttachesAsterMetadataToStages(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			},
			Aster: manifests.AsterSet{Required: []string{"plan"}},
		},
	}
	locator := &stubAsterLocator{
		bundles: map[string]aster.Metadata{
			modsPlanStage + "/plan":             {Stage: modsPlanStage, Toggle: "plan", BundleID: "mods-plan", Digest: "sha256:modsplan", ArtifactCID: "cid-mods-plan", Source: "build/aster/mods-plan.tar.zst"},
			mods.StageNameORWApply + "/plan":    {Stage: mods.StageNameORWApply, Toggle: "plan", BundleID: "orw-apply-plan"},
			mods.StageNameORWGenerate + "/plan": {Stage: mods.StageNameORWGenerate, Toggle: "plan", BundleID: "orw-gen-plan"},
			mods.StageNameLLMPlan + "/plan":     {Stage: mods.StageNameLLMPlan, Toggle: "plan", BundleID: "llm-plan-plan"},
			mods.StageNameLLMExec + "/plan":     {Stage: mods.StageNameLLMExec, Toggle: "plan", BundleID: "llm-exec-plan"},
			mods.StageNameHuman + "/plan":       {Stage: mods.StageNameHuman, Toggle: "plan", BundleID: "mods-human-plan"},
			buildGateStage + "/plan":            {Stage: buildGateStage, Toggle: "plan", BundleID: "build-plan", Digest: "sha256:buildplan", ArtifactCID: "cid-build-plan", Source: "build/aster/build-plan.tar.zst"},
			staticChecksStage + "/plan":         {Stage: staticChecksStage, Toggle: "plan", BundleID: "static-checks-plan", Digest: "sha256:staticplan", ArtifactCID: "cid-static-plan", Source: "build/aster/static-plan.tar.zst"},
			"test/plan":                         {Stage: "test", Toggle: "plan", BundleID: "test-plan", Digest: "sha256:testplan", ArtifactCID: "cid-test-plan", Source: "build/aster/test-plan.tar.zst"},
		},
	}
	grid := &fakeGrid{}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
		Aster: runner.AsterOptions{
			Enabled: true,
			Locator: locator,
		},
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(grid.calls) == 0 {
		t.Fatal("expected grid invocations")
	}

	mods := findStageCall(grid.calls, modsPlanStage)
	if !mods.stage.Aster.Enabled {
		t.Fatal("expected mods stage to enable Aster")
	}
	if len(mods.stage.Aster.Toggles) != 1 || mods.stage.Aster.Toggles[0] != "plan" {
		t.Fatalf("unexpected mods toggles: %+v", mods.stage.Aster.Toggles)
	}
	if len(mods.stage.Aster.Bundles) != 1 || mods.stage.Aster.Bundles[0].BundleID != "mods-plan" {
		t.Fatalf("unexpected mods bundle metadata: %+v", mods.stage.Aster.Bundles)
	}

	build := findStageCall(grid.calls, buildGateStage)
	if !build.stage.Aster.Enabled {
		t.Fatal("expected build stage to enable Aster")
	}
	if len(build.stage.Aster.Bundles) != 1 || build.stage.Aster.Bundles[0].BundleID != "build-plan" {
		t.Fatalf("unexpected build bundle metadata: %+v", build.stage.Aster.Bundles)
	}

	testStage := findStageCall(grid.calls, "test")
	if !testStage.stage.Aster.Enabled {
		t.Fatal("expected test stage to enable Aster")
	}
	if len(testStage.stage.Aster.Bundles) != 1 || testStage.stage.Aster.Bundles[0].BundleID != "test-plan" {
		t.Fatalf("unexpected test bundle metadata: %+v", testStage.stage.Aster.Bundles)
	}
}

func TestRunAllowsDisablingAsterPerStage(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			},
			Aster: manifests.AsterSet{Required: []string{"plan"}},
		},
	}
	locator := &stubAsterLocator{
		bundles: map[string]aster.Metadata{
			modsPlanStage + "/plan":             {Stage: modsPlanStage, Toggle: "plan", BundleID: "mods-plan"},
			mods.StageNameORWApply + "/plan":    {Stage: mods.StageNameORWApply, Toggle: "plan", BundleID: "orw-apply-plan"},
			mods.StageNameORWGenerate + "/plan": {Stage: mods.StageNameORWGenerate, Toggle: "plan", BundleID: "orw-gen-plan"},
			mods.StageNameLLMPlan + "/plan":     {Stage: mods.StageNameLLMPlan, Toggle: "plan", BundleID: "llm-plan-plan"},
			mods.StageNameLLMExec + "/plan":     {Stage: mods.StageNameLLMExec, Toggle: "plan", BundleID: "llm-exec-plan"},
			mods.StageNameHuman + "/plan":       {Stage: mods.StageNameHuman, Toggle: "plan", BundleID: "mods-human-plan"},
			staticChecksStage + "/plan":         {Stage: staticChecksStage, Toggle: "plan", BundleID: "static-checks-plan"},
			"test/plan":                         {Stage: "test", Toggle: "plan", BundleID: "test-plan"},
		},
	}
	grid := &fakeGrid{}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
		Aster: runner.AsterOptions{
			Enabled: true,
			Locator: locator,
			StageOverrides: map[string]runner.AsterStageOverride{
				buildGateStage: {Disable: true},
			},
		},
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mods := findStageCall(grid.calls, modsPlanStage)
	if !mods.stage.Aster.Enabled {
		t.Fatal("expected mods stage to enable Aster")
	}
	if len(mods.stage.Aster.Bundles) != 1 || mods.stage.Aster.Bundles[0].BundleID != "mods-plan" {
		t.Fatalf("unexpected mods bundles: %+v", mods.stage.Aster.Bundles)
	}

	build := findStageCall(grid.calls, buildGateStage)
	if build.stage.Aster.Enabled {
		t.Fatalf("expected build stage to disable Aster, got %+v", build.stage.Aster)
	}
	if len(build.stage.Aster.Bundles) != 0 {
		t.Fatalf("expected no bundles for build stage, got %+v", build.stage.Aster.Bundles)
	}

	testStage := findStageCall(grid.calls, "test")
	if !testStage.stage.Aster.Enabled {
		t.Fatal("expected test stage to enable Aster")
	}
	if len(testStage.stage.Aster.Bundles) != 1 || testStage.stage.Aster.Bundles[0].BundleID != "test-plan" {
		t.Fatalf("unexpected test stage bundles: %+v", testStage.stage.Aster.Bundles)
	}
}

func TestRunRequiresAsterLocatorWhenManifestRequiresToggles(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			},
			Aster: manifests.AsterSet{Required: []string{"plan"}},
		},
	}
	grid := &fakeGrid{}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
		Aster:            runner.AsterOptions{Enabled: true},
	}
	err := runner.Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when locator missing")
	}
	if !errors.Is(err, runner.ErrAsterLocatorRequired) {
		t.Fatalf("expected ErrAsterLocatorRequired, got %v", err)
	}
}

func TestRunMergesAsterOverridesAndToggles(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			},
			Aster: manifests.AsterSet{Required: []string{"plan"}},
		},
	}
	locator := &stubAsterLocator{
		bundles: map[string]aster.Metadata{
			modsPlanStage + "/plan":             {Stage: modsPlanStage, Toggle: "plan", BundleID: "mods-plan"},
			modsPlanStage + "/exec":             {Stage: modsPlanStage, Toggle: "exec", BundleID: "mods-exec"},
			modsPlanStage + "/lint":             {Stage: modsPlanStage, Toggle: "lint", BundleID: "mods-lint"},
			mods.StageNameORWApply + "/plan":    {Stage: mods.StageNameORWApply, Toggle: "plan", BundleID: "orw-apply-plan"},
			mods.StageNameORWApply + "/exec":    {Stage: mods.StageNameORWApply, Toggle: "exec", BundleID: "orw-apply-exec"},
			mods.StageNameORWGenerate + "/plan": {Stage: mods.StageNameORWGenerate, Toggle: "plan", BundleID: "orw-gen-plan"},
			mods.StageNameORWGenerate + "/exec": {Stage: mods.StageNameORWGenerate, Toggle: "exec", BundleID: "orw-gen-exec"},
			mods.StageNameLLMPlan + "/plan":     {Stage: mods.StageNameLLMPlan, Toggle: "plan", BundleID: "llm-plan-plan"},
			mods.StageNameLLMPlan + "/exec":     {Stage: mods.StageNameLLMPlan, Toggle: "exec", BundleID: "llm-plan-exec"},
			mods.StageNameLLMExec + "/plan":     {Stage: mods.StageNameLLMExec, Toggle: "plan", BundleID: "llm-exec-plan"},
			mods.StageNameLLMExec + "/exec":     {Stage: mods.StageNameLLMExec, Toggle: "exec", BundleID: "llm-exec-exec"},
			mods.StageNameHuman + "/plan":       {Stage: mods.StageNameHuman, Toggle: "plan", BundleID: "mods-human-plan"},
			mods.StageNameHuman + "/exec":       {Stage: mods.StageNameHuman, Toggle: "exec", BundleID: "mods-human-exec"},
			buildGateStage + "/plan":            {Stage: buildGateStage, Toggle: "plan", BundleID: "build-plan"},
			buildGateStage + "/exec":            {Stage: buildGateStage, Toggle: "exec", BundleID: "build-exec"},
			staticChecksStage + "/plan":         {Stage: staticChecksStage, Toggle: "plan", BundleID: "static-checks-plan"},
			staticChecksStage + "/exec":         {Stage: staticChecksStage, Toggle: "exec", BundleID: "static-checks-exec"},
			"test/plan":                         {Stage: "test", Toggle: "plan", BundleID: "test-plan"},
			"test/exec":                         {Stage: "test", Toggle: "exec", BundleID: "test-exec"},
		},
	}
	grid := &fakeGrid{}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
		Aster: runner.AsterOptions{
			Enabled:           true,
			Locator:           locator,
			AdditionalToggles: []string{"EXEC", "plan"},
			StageOverrides: map[string]runner.AsterStageOverride{
				modsPlanStage: {ExtraToggles: []string{"Lint", "plan"}},
			},
		},
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mods := findStageCall(grid.calls, modsPlanStage)
	if len(mods.stage.Aster.Toggles) != 3 {
		t.Fatalf("expected merged toggles, got %+v", mods.stage.Aster.Toggles)
	}
	expect := []string{"exec", "lint", "plan"}
	for i, toggle := range expect {
		if mods.stage.Aster.Toggles[i] != toggle {
			t.Fatalf("expected toggle %s at %d, got %s", toggle, i, mods.stage.Aster.Toggles[i])
		}
	}
	if len(mods.stage.Aster.Bundles) != 3 {
		t.Fatalf("expected 3 bundles, got %+v", mods.stage.Aster.Bundles)
	}
}

func TestRunFillsMissingAsterMetadataFields(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Lanes:           manifests.LaneSet{Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}}},
			Aster:           manifests.AsterSet{Required: []string{"plan"}},
		},
	}
	locator := &stubAsterLocator{
		bundles: map[string]aster.Metadata{
			modsPlanStage + "/plan":             {BundleID: "mods-plan"},
			mods.StageNameORWApply + "/plan":    {BundleID: "orw-apply-plan"},
			mods.StageNameORWGenerate + "/plan": {BundleID: "orw-gen-plan"},
			mods.StageNameLLMPlan + "/plan":     {BundleID: "llm-plan-plan"},
			mods.StageNameLLMExec + "/plan":     {BundleID: "llm-exec-plan"},
			mods.StageNameHuman + "/plan":       {BundleID: "mods-human-plan"},
			buildGateStage + "/plan":            {Stage: buildGateStage, Toggle: "plan", BundleID: "build-plan"},
			staticChecksStage + "/plan":         {Stage: staticChecksStage, Toggle: "plan", BundleID: "static-checks-plan"},
			"test/plan":                         {Stage: "test", Toggle: "plan", BundleID: "test-plan"},
		},
	}
	grid := &fakeGrid{}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
		Aster:            runner.AsterOptions{Enabled: true, Locator: locator},
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mods := findStageCall(grid.calls, modsPlanStage)
	if mods.stage.Aster.Bundles[0].Stage != modsPlanStage {
		t.Fatalf("expected stage fallback, got %+v", mods.stage.Aster.Bundles[0])
	}
	if mods.stage.Aster.Bundles[0].Toggle != "plan" {
		t.Fatalf("expected toggle fallback, got %+v", mods.stage.Aster.Bundles[0])
	}
}

func TestRunPropagatesAsterLocatorError(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			ManifestVersion: "v2",
			Lanes:           manifests.LaneSet{Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}}},
			Aster:           manifests.AsterSet{Required: []string{"plan"}},
		},
	}
	grid := &fakeGrid{}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
		Aster:            runner.AsterOptions{Enabled: true, Locator: &stubAsterLocator{}},
	}
	err := runner.Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when bundle metadata missing")
	}
	if !errors.Is(err, aster.ErrBundleNotFound) {
		t.Fatalf("expected ErrBundleNotFound, got %v", err)
	}
}
