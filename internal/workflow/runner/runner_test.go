package runner_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

func defaultManifestCompilation() manifests.Compilation {
	return manifests.Compilation{
		Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
		Lanes: manifests.LaneSet{
			Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			Allowed:  []manifests.Lane{{Name: "gpu-ml"}},
		},
	}
}

const modsPlanStage = mods.StageNamePlan

func newStubCompiler() *recordingCompiler {
	return &recordingCompiler{compiled: defaultManifestCompilation()}
}

func TestDefaultPlannerBuildsOrderedStages(t *testing.T) {
	planner := runner.NewDefaultPlanner()
	ticket := contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"}
	plan, err := planner.Build(context.Background(), ticket)
	if err != nil {
		t.Fatalf("unexpected error building plan: %v", err)
	}
	if plan.TicketID != ticket.TicketID {
		t.Fatalf("plan ticket mismatch: %s", plan.TicketID)
	}
	if len(plan.Stages) != 8 {
		t.Fatalf("expected 8 stages, got %d", len(plan.Stages))
	}
	expectOrder := []string{
		mods.StageNamePlan,
		mods.StageNameORWApply,
		mods.StageNameORWGenerate,
		mods.StageNameLLMPlan,
		mods.StageNameLLMExec,
		mods.StageNameHuman,
		"build",
		"test",
	}
	expectLanes := []string{"node-wasm", "node-wasm", "node-wasm", "gpu-ml", "gpu-ml", "go-native", "go-native", "go-native"}
	for i, name := range expectOrder {
		stage := plan.Stages[i]
		if stage.Name != name {
			t.Fatalf("unexpected stage at %d: %s", i, stage.Name)
		}
		if stage.Lane != expectLanes[i] {
			t.Fatalf("unexpected lane for %s: %s", stage.Name, stage.Lane)
		}
	}
	depMap := map[string][]string{
		mods.StageNamePlan:        nil,
		mods.StageNameORWApply:    {mods.StageNamePlan},
		mods.StageNameORWGenerate: {mods.StageNamePlan},
		mods.StageNameLLMPlan:     {mods.StageNamePlan},
		mods.StageNameLLMExec:     {mods.StageNameORWApply, mods.StageNameORWGenerate, mods.StageNameLLMPlan},
		mods.StageNameHuman:       {mods.StageNameLLMExec},
		"build":                   {mods.StageNameHuman},
		"test":                    {"build"},
	}
	for _, stage := range plan.Stages {
		expectedDeps, ok := depMap[stage.Name]
		if !ok {
			t.Fatalf("unexpected stage in plan: %s", stage.Name)
		}
		if len(stage.Dependencies) != len(expectedDeps) {
			t.Fatalf("dependency mismatch for %s: got %v want %v", stage.Name, stage.Dependencies, expectedDeps)
		}
		for i, dep := range expectedDeps {
			if stage.Dependencies[i] != dep {
				t.Fatalf("dependency %d for %s: got %s want %s", i, stage.Name, stage.Dependencies[i], dep)
			}
		}
	}
}

func TestRunRequiresEventsClient(t *testing.T) {
	opts := runner.Options{Ticket: "ticket-123", Grid: runner.NewInMemoryGrid(), ManifestCompiler: newStubCompiler()}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrEventsClientRequired) {
		t.Fatalf("expected ErrEventsClientRequired, got %v", err)
	}
}

func TestRunRequiresGridClient(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{Ticket: "ticket-123", Events: events, ManifestCompiler: newStubCompiler()}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrGridClientRequired) {
		t.Fatalf("expected ErrGridClientRequired, got %v", err)
	}
}

func TestRunRequiresManifestCompiler(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:          "ticket-123",
		Events:          events,
		Grid:            runner.NewInMemoryGrid(),
		Planner:         runner.NewDefaultPlanner(),
		WorkspaceRoot:   t.TempDir(),
		MaxStageRetries: 1,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrManifestCompilerRequired) {
		t.Fatalf("expected ErrManifestCompilerRequired, got %v", err)
	}
}

func TestRunPropagatesManifestCompilationError(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compilerErr := errors.New("compile failed")
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             runner.NewInMemoryGrid(),
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: failingCompiler{err: compilerErr},
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, compilerErr) {
		t.Fatalf("expected compiler error, got %v", err)
	}
}

func TestRunPassesManifestConstraintsToGrid(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			},
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
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(grid.calls) == 0 {
		t.Fatal("expected at least one grid call")
	}
	for _, call := range grid.calls {
		if call.stage.Constraints.Manifest.Manifest.Name != "smoke" {
			t.Fatalf("expected manifest on stage, got %+v", call.stage.Constraints.Manifest)
		}
	}
	if compiler.ref.Name != "smoke" || compiler.ref.Version == "" {
		t.Fatalf("expected manifest reference to be captured, got %+v", compiler.ref)
	}
}

func TestRunAcceptsAllowedLaneAssignments(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}},
				Allowed:  []manifests.Lane{{Name: "go-native"}, {Name: "gpu-ml"}},
			},
		},
	}
	grid := runner.NewInMemoryGrid()
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAttachesAsterMetadataToStages(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
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
			"build/plan":                        {Stage: "build", Toggle: "plan", BundleID: "build-plan", Digest: "sha256:buildplan", ArtifactCID: "cid-build-plan", Source: "build/aster/build-plan.tar.zst"},
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

	build := findStageCall(grid.calls, "build")
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
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
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
			Locator: locator,
			StageOverrides: map[string]runner.AsterStageOverride{
				"build": {Disable: true},
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

	build := findStageCall(grid.calls, "build")
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
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			Lanes:    manifests.LaneSet{Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}}},
			Aster:    manifests.AsterSet{Required: []string{"plan"}},
		},
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             runner.NewInMemoryGrid(),
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrAsterLocatorRequired) {
		t.Fatalf("expected ErrAsterLocatorRequired, got %v", err)
	}
}

func TestRunMergesAsterOverridesAndToggles(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
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
			"build/plan":                        {Stage: "build", Toggle: "plan", BundleID: "build-plan"},
			"build/exec":                        {Stage: "build", Toggle: "exec", BundleID: "build-exec"},
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
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			Lanes:    manifests.LaneSet{Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}}},
			Aster:    manifests.AsterSet{Required: []string{"plan"}},
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
			"build/plan":                        {Stage: "build", Toggle: "plan", BundleID: "build-plan"},
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
		Aster:            runner.AsterOptions{Locator: locator},
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
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			Lanes:    manifests.LaneSet{Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}}},
			Aster:    manifests.AsterSet{Required: []string{"plan"}},
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
		Aster:            runner.AsterOptions{Locator: &stubAsterLocator{}},
	}
	err := runner.Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when bundle metadata missing")
	}
	if !errors.Is(err, aster.ErrBundleNotFound) {
		t.Fatalf("expected ErrBundleNotFound, got %v", err)
	}
}

func TestRunReturnsClaimTicketError(t *testing.T) {
	events := &errorEvents{claimErr: errors.New("claim failed")}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             runner.NewInMemoryGrid(),
		Planner:          runner.NewDefaultPlanner(),
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.claimErr) {
		t.Fatalf("expected claim error, got %v", err)
	}
}

func TestRunPropagatesPublishCheckpointError(t *testing.T) {
	events := &errorEvents{
		ticket: contracts.WorkflowTicket{
			SchemaVersion: contracts.SchemaVersion,
			TicketID:      "ticket-123",
			Tenant:        "acme",
		},
		publishErr: errors.New("checkpoint failure"),
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             runner.NewInMemoryGrid(),
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.publishErr) {
		t.Fatalf("expected publish error, got %v", err)
	}
}

func TestRunPropagatesPublishArtifactError(t *testing.T) {
	events := &errorEvents{
		ticket: contracts.WorkflowTicket{
			SchemaVersion: contracts.SchemaVersion,
			TicketID:      "ticket-123",
			Tenant:        "acme",
		},
		artifactErr: errors.New("artifact failure"),
	}
	grid := runner.NewInMemoryGrid()
	grid.StageOutcomes[modsPlanStage] = []runner.StageOutcome{{
		Status: runner.StageStatusCompleted,
		Artifacts: []runner.Artifact{{
			Name:        "mods-plan",
			ArtifactCID: "cid-mods-plan",
		}},
	}}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.artifactErr) {
		t.Fatalf("expected artifact publish error, got %v", err)
	}
}

func TestRunErrorsWhenWorkspaceRootInvalid(t *testing.T) {
	temp := t.TempDir()
	file := filepath.Join(temp, "lock")
	if err := os.WriteFile(file, []byte("lock"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             &fakeGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    filepath.Join(file, "workspace"),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "create workspace root") {
		t.Fatalf("expected workspace root error, got %v", err)
	}
}

func TestRunTreatsNegativeRetriesAsZero(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build":       {{Status: runner.StageStatusFailed, Retryable: true, Message: "no more"}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  -3,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrStageFailed) {
		t.Fatalf("expected stage failure, got %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	expected := []stageStatusEntry{
		{stage: "ticket-claimed", status: runner.StageStatusCompleted},
		{stage: modsPlanStage, status: runner.StageStatusRunning},
		{stage: modsPlanStage, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWApply, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWApply, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusCompleted},
		{stage: mods.StageNameHuman, status: runner.StageStatusRunning},
		{stage: mods.StageNameHuman, status: runner.StageStatusCompleted},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusFailed},
		{stage: "workflow", status: runner.StageStatusFailed},
	}
	if err := compareSequences(sequence, expected); err != nil {
		t.Fatalf("checkpoint sequence mismatch: %v", err)
	}
}

func TestRunDefaultsStageOutcomeStatus(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             statuslessGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) == 0 || sequence[len(sequence)-1].status != runner.StageStatusCompleted {
		t.Fatalf("expected workflow completion, got %v", sequence)
	}
}

func TestRunPublishesCacheKeysInCheckpoints(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := runner.NewInMemoryGrid()
	composer := &recordingCacheComposer{}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
		CacheComposer:    composer,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(composer.calls) == 0 {
		t.Fatal("expected cache composer to be invoked")
	}
	stageChecks := map[string]int{modsPlanStage: 0, "build": 0, "test": 0}
	for _, checkpoint := range events.checkpoints {
		switch checkpoint.Stage {
		case modsPlanStage, "build", "test":
			if checkpoint.CacheKey == "" {
				t.Fatalf("expected cache key for stage %s", checkpoint.Stage)
			}
			expected := fmt.Sprintf("cache-%s", checkpoint.Stage)
			if checkpoint.CacheKey != expected {
				t.Fatalf("unexpected cache key for %s: %s", checkpoint.Stage, checkpoint.CacheKey)
			}
			stageChecks[checkpoint.Stage]++
		case "ticket-claimed", "workflow":
			if checkpoint.CacheKey != "" {
				t.Fatalf("expected no cache key for %s checkpoint", checkpoint.Stage)
			}
		}
	}
	for stage, count := range stageChecks {
		if count == 0 {
			t.Fatalf("expected cache key checkpoints for stage %s", stage)
		}
	}
}

func TestRunPublishesStageMetadataAndArtifacts(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	compiler := &recordingCompiler{
		compiled: manifests.Compilation{
			Manifest: manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
			Lanes: manifests.LaneSet{
				Required: []manifests.Lane{{Name: "node-wasm"}, {Name: "go-native"}},
			},
		},
	}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{
				Status:    runner.StageStatusCompleted,
				Artifacts: []runner.Artifact{{Name: "mods-plan", ArtifactCID: "cid-mods-plan", Digest: "sha256:modsplan", MediaType: "application/tar+zst"}},
			}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: compiler,
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var modsRunning, modsCompleted *contracts.WorkflowCheckpoint
	for i := range events.checkpoints {
		cp := events.checkpoints[i]
		if cp.Stage != modsPlanStage {
			continue
		}
		switch cp.Status {
		case contracts.CheckpointStatusRunning:
			modsRunning = &cp
		case contracts.CheckpointStatusCompleted:
			modsCompleted = &cp
		}
	}
	if modsRunning == nil || modsRunning.StageMetadata == nil {
		t.Fatalf("expected running checkpoint with stage metadata: %#v", modsRunning)
	}
	if modsRunning.StageMetadata.Lane != "node-wasm" {
		t.Fatalf("unexpected lane on running checkpoint: %#v", modsRunning.StageMetadata)
	}
	if len(modsRunning.Artifacts) > 0 {
		t.Fatalf("expected no artifacts on running checkpoint: %#v", modsRunning.Artifacts)
	}
	if modsCompleted == nil {
		t.Fatal("expected completed mods checkpoint")
	}
	if modsCompleted.StageMetadata == nil {
		t.Fatalf("expected stage metadata on completed checkpoint: %#v", modsCompleted)
	}
	if modsCompleted.StageMetadata.Manifest.Version != "2025-09-26" {
		t.Fatalf("unexpected manifest on completed checkpoint: %#v", modsCompleted.StageMetadata.Manifest)
	}
	if len(modsCompleted.Artifacts) != 1 {
		t.Fatalf("expected single artifact on completed checkpoint: %#v", modsCompleted.Artifacts)
	}
	artifact := modsCompleted.Artifacts[0]
	if artifact.ArtifactCID != "cid-mods-plan" || artifact.Digest != "sha256:modsplan" {
		t.Fatalf("unexpected artifact manifest: %#v", artifact)
	}

	if len(events.artifacts) != 1 {
		t.Fatalf("expected single artifact envelope, got %d", len(events.artifacts))
	}
	envelope := events.artifacts[0]
	if envelope.TicketID != "ticket-123" {
		t.Fatalf("unexpected artifact ticket id: %#v", envelope)
	}
	if envelope.Stage != modsPlanStage {
		t.Fatalf("unexpected artifact stage: %#v", envelope)
	}
	if envelope.Artifact.ArtifactCID != "cid-mods-plan" {
		t.Fatalf("expected artifact CID to mirror checkpoint, got %#v", envelope.Artifact)
	}
	if envelope.StageMetadata == nil || envelope.StageMetadata.Lane != "node-wasm" {
		t.Fatalf("expected artifact envelope to include stage metadata: %#v", envelope.StageMetadata)
	}

	var workflowCheckpoint *contracts.WorkflowCheckpoint
	for i := range events.checkpoints {
		cp := events.checkpoints[i]
		if cp.Stage == "workflow" && cp.Status == contracts.CheckpointStatusCompleted {
			workflowCheckpoint = &cp
			break
		}
	}
	if workflowCheckpoint == nil {
		t.Fatal("expected workflow completion checkpoint")
	}
	if workflowCheckpoint.StageMetadata != nil {
		t.Fatalf("expected workflow checkpoint to omit stage metadata: %#v", workflowCheckpoint.StageMetadata)
	}
	if len(workflowCheckpoint.Artifacts) != 0 {
		t.Fatalf("expected workflow checkpoint to omit artifacts: %#v", workflowCheckpoint.Artifacts)
	}
}

func TestRunUsesDefaultPlannerWhenNil(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             noStageGrid{},
		Planner:          nil,
		WorkspaceRoot:    "",
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) != 18 {
		t.Fatalf("expected 18 checkpoints, got %d", len(sequence))
	}
	if sequence[1].stage != modsPlanStage || sequence[1].status != runner.StageStatusRunning {
		t.Fatalf("expected first stage checkpoint to be %s running, got %+v", modsPlanStage, sequence[1])
	}
	last := sequence[len(sequence)-1]
	if last.stage != "workflow" || last.status != runner.StageStatusCompleted {
		t.Fatalf("expected workflow completion, got %+v", last)
	}
}

func TestRunFailsWhenStageCompletionPublishFails(t *testing.T) {
	events := &countingEvents{
		ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"},
		failAt: 3,
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             noStageGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.err) {
		t.Fatalf("expected publish error, got %v", err)
	}
}

func TestRunFailsWhenFinalPublishFails(t *testing.T) {
	events := &countingEvents{
		ticket: contracts.WorkflowTicket{SchemaVersion: contracts.SchemaVersion, TicketID: "ticket-123", Tenant: "acme"},
		failAt: 8,
	}
	opts := runner.Options{
		Ticket:           "ticket-123",
		Events:           events,
		Grid:             noStageGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, events.err) {
		t.Fatalf("expected final publish error, got %v", err)
	}
}

func TestRunAutoClaimsTicketAndCleansWorkspace(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build":       {{Status: runner.StageStatusCompleted}},
			"test":        {{Status: runner.StageStatusCompleted}},
		},
	}
	planner := runner.NewDefaultPlanner()
	workspaceRoot := t.TempDir()
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          planner,
		WorkspaceRoot:    workspaceRoot,
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("run error: %v", err)
	}
	if len(events.claimedTickets) != 1 || events.claimedTickets[0] != "ticket-123" {
		t.Fatalf("expected auto-claimed ticket, got %v", events.claimedTickets)
	}
	sequence := extractStageStatuses(events.checkpoints)
	expected := []stageStatusEntry{
		{stage: "ticket-claimed", status: runner.StageStatusCompleted},
		{stage: modsPlanStage, status: runner.StageStatusRunning},
		{stage: modsPlanStage, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWApply, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWApply, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusCompleted},
		{stage: mods.StageNameHuman, status: runner.StageStatusRunning},
		{stage: mods.StageNameHuman, status: runner.StageStatusCompleted},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusCompleted},
		{stage: "test", status: runner.StageStatusRunning},
		{stage: "test", status: runner.StageStatusCompleted},
		{stage: "workflow", status: runner.StageStatusCompleted},
	}
	if err := compareSequences(sequence, expected); err != nil {
		t.Fatalf("checkpoint sequence mismatch: %v", err)
	}
	if grid.lastWorkspace == "" {
		t.Fatal("expected workspace to be recorded")
	}
	if !strings.HasPrefix(grid.lastWorkspace, workspaceRoot) {
		t.Fatalf("workspace %q not under root %q", grid.lastWorkspace, workspaceRoot)
	}
	if _, err := os.Stat(grid.lastWorkspace); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected workspace to be deleted, stat err=%v", err)
	}
}

func TestRunRetriesStageOnce(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build": {
				{Status: runner.StageStatusFailed, Retryable: true, Message: "grid transient"},
				{Status: runner.StageStatusCompleted},
			},
			"test": {{Status: runner.StageStatusCompleted}},
		},
	}
	planner := runner.NewDefaultPlanner()
	workspaceRoot := t.TempDir()
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          planner,
		WorkspaceRoot:    workspaceRoot,
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	if err := runner.Run(context.Background(), opts); err != nil {
		t.Fatalf("run error: %v", err)
	}
	b := gatherStageAttempts(grid.calls, "build")
	if b != 2 {
		t.Fatalf("expected build stage to retry once, got %d attempts", b)
	}
	sequence := extractStageStatuses(events.checkpoints)
	expected := []stageStatusEntry{
		{stage: "ticket-claimed", status: runner.StageStatusCompleted},
		{stage: modsPlanStage, status: runner.StageStatusRunning},
		{stage: modsPlanStage, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWApply, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWApply, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusCompleted},
		{stage: mods.StageNameHuman, status: runner.StageStatusRunning},
		{stage: mods.StageNameHuman, status: runner.StageStatusCompleted},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusRetrying},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusCompleted},
		{stage: "test", status: runner.StageStatusRunning},
		{stage: "test", status: runner.StageStatusCompleted},
		{stage: "workflow", status: runner.StageStatusCompleted},
	}
	if err := compareSequences(sequence, expected); err != nil {
		t.Fatalf("checkpoint sequence mismatch: %v", err)
	}
}

func TestRunStopsAfterRetryLimit(t *testing.T) {
	withCleanupDeadline(t)
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build":       {{Status: runner.StageStatusFailed, Retryable: true, Message: "still broken"}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  0,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrStageFailed) {
		t.Fatalf("expected stage failure, got %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	expected := []stageStatusEntry{
		{stage: "ticket-claimed", status: runner.StageStatusCompleted},
		{stage: modsPlanStage, status: runner.StageStatusRunning},
		{stage: modsPlanStage, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWApply, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWApply, status: runner.StageStatusCompleted},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusRunning},
		{stage: mods.StageNameORWGenerate, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMPlan, status: runner.StageStatusCompleted},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusRunning},
		{stage: mods.StageNameLLMExec, status: runner.StageStatusCompleted},
		{stage: mods.StageNameHuman, status: runner.StageStatusRunning},
		{stage: mods.StageNameHuman, status: runner.StageStatusCompleted},
		{stage: "build", status: runner.StageStatusRunning},
		{stage: "build", status: runner.StageStatusFailed},
		{stage: "workflow", status: runner.StageStatusFailed},
	}
	if err := compareSequences(sequence, expected); err != nil {
		t.Fatalf("checkpoint sequence mismatch: %v", err)
	}
}

func TestRunFailsWhenPlannerErrors(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{}
	planner := failingPlanner{err: errors.New("planner boom")}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          planner,
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, planner.err) {
		t.Fatalf("expected planner error, got %v", err)
	}
}

func TestRunFailsWhenPlannerProducesInvalidStage(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             &fakeGrid{},
		Planner:          invalidStagePlanner{},
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrCheckpointValidationFailed) {
		t.Fatalf("expected checkpoint validation error, got %v", err)
	}
}

func TestRunFailsWhenPlannerOmitsLane(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             &fakeGrid{},
		Planner:          missingLanePlanner{},
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrLaneRequired) {
		t.Fatalf("expected ErrLaneRequired, got %v", err)
	}
}

func TestRunErrorsWhenTicketValidationFails(t *testing.T) {
	events := &recordingEvents{tenant: "acme", invalidTicket: true}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             &fakeGrid{},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, runner.ErrTicketValidationFailed) {
		t.Fatalf("expected ticket validation error, got %v", err)
	}
}

func TestRunSurfacesNonRetryableStageFailure(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build":       {{Status: runner.StageStatusFailed, Retryable: false, Message: "bad cache"}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error from non-retryable failure")
	}
	if !errors.Is(err, runner.ErrStageFailed) {
		t.Fatalf("expected ErrStageFailed, got %v", err)
	}
	sequence := extractStageStatuses(events.checkpoints)
	if len(sequence) == 0 || sequence[len(sequence)-1].status != runner.StageStatusFailed {
		t.Fatalf("expected last checkpoint to be failed, sequence=%v", sequence)
	}
}

func TestRunUsesFallbackFailureMessage(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	grid := &fakeGrid{
		outcomes: map[string][]runner.StageOutcome{
			modsPlanStage: {{Status: runner.StageStatusCompleted}},
			"build":       {{Status: runner.StageStatusFailed, Retryable: false}},
		},
	}
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for failure")
	}
	if !strings.Contains(err.Error(), "stage failed") {
		t.Fatalf("expected fallback message, got %v", err)
	}
}

func TestRunPropagatesGridError(t *testing.T) {
	events := &recordingEvents{nextTicket: "ticket-123", tenant: "acme"}
	gridErr := errors.New("grid down")
	opts := runner.Options{
		Ticket:           "",
		Tenant:           "acme",
		Events:           events,
		Grid:             errorGrid{err: gridErr},
		Planner:          runner.NewDefaultPlanner(),
		WorkspaceRoot:    t.TempDir(),
		MaxStageRetries:  1,
		ManifestCompiler: newStubCompiler(),
	}
	err := runner.Run(context.Background(), opts)
	if !errors.Is(err, gridErr) {
		t.Fatalf("expected grid error, got %v", err)
	}
}

func TestInMemoryGridRecordsInvocations(t *testing.T) {
	grid := runner.NewInMemoryGrid()
	grid.StageOutcomes["build"] = []runner.StageOutcome{{Status: runner.StageStatusFailed, Retryable: true, Message: "retry-me"}}
	if outcome, err := grid.ExecuteStage(context.Background(), contracts.WorkflowTicket{TicketID: "ticket-1", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, runner.Stage{Name: modsPlanStage, Lane: "node-wasm"}, "/tmp/work"); err != nil {
		t.Fatalf("unexpected error for default outcome: %v", err)
	} else if outcome.Status != runner.StageStatusCompleted {
		t.Fatalf("expected completed outcome, got %+v", outcome)
	}
	outcome, err := grid.ExecuteStage(context.Background(), contracts.WorkflowTicket{TicketID: "ticket-1", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, runner.Stage{Name: "build", Lane: "go-native"}, "/tmp/work")
	if err != nil {
		t.Fatalf("unexpected error for configured outcome: %v", err)
	}
	if outcome.Status != runner.StageStatusFailed || !outcome.Retryable {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	invocations := grid.Invocations()
	if len(invocations) != 2 {
		t.Fatalf("expected two invocations, got %d", len(invocations))
	}
	if invocations[0].Stage.Name != modsPlanStage || invocations[1].Stage.Name != "build" {
		t.Fatalf("unexpected invocation order: %+v", invocations)
	}
	if invocations[0].Stage.Lane != "node-wasm" || invocations[1].Stage.Lane != "go-native" {
		t.Fatalf("unexpected lanes recorded: %+v", invocations)
	}
}

func TestInMemoryGridRejectsMissingLane(t *testing.T) {
	grid := runner.NewInMemoryGrid()
	_, err := grid.ExecuteStage(context.Background(), contracts.WorkflowTicket{TicketID: "ticket-1", Manifest: contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}}, runner.Stage{Name: modsPlanStage}, "/tmp/work")
	if err == nil || !strings.Contains(err.Error(), "lane missing") {
		t.Fatalf("expected lane missing error, got %v", err)
	}
}

type errorGrid struct {
	err error
}

func (g errorGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = stage
	_ = workspace
	return runner.StageOutcome{}, g.err
}

type recordingCacheComposer struct {
	calls []runner.CacheComposeRequest
}

func (r *recordingCacheComposer) Compose(ctx context.Context, req runner.CacheComposeRequest) (string, error) {
	_ = ctx
	r.calls = append(r.calls, req)
	return fmt.Sprintf("cache-%s", strings.ToLower(req.Stage.Name)), nil
}

type errorEvents struct {
	ticket      contracts.WorkflowTicket
	claimErr    error
	publishErr  error
	artifactErr error
}

func (e *errorEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if e.claimErr != nil {
		return contracts.WorkflowTicket{}, e.claimErr
	}
	if e.ticket.TicketID == "" {
		e.ticket.TicketID = ticketID
	}
	if e.ticket.SchemaVersion == "" {
		e.ticket.SchemaVersion = contracts.SchemaVersion
	}
	if e.ticket.Tenant == "" {
		e.ticket.Tenant = "acme"
	}
	if e.ticket.Manifest.Name == "" || e.ticket.Manifest.Version == "" {
		e.ticket.Manifest = contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	if strings.TrimSpace(e.ticket.TicketID) == "" {
		e.ticket.TicketID = "ticket-auto"
	}
	return e.ticket, nil
}

func (e *errorEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	if e.publishErr != nil {
		return e.publishErr
	}
	return nil
}

func (e *errorEvents) PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error {
	_ = ctx
	if e.artifactErr != nil {
		return e.artifactErr
	}
	return nil
}

type statuslessGrid struct{}

func (statuslessGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = workspace
	return runner.StageOutcome{Stage: stage}, nil
}

type noStageGrid struct{}

func (noStageGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	_ = workspace
	return runner.StageOutcome{Status: runner.StageStatusCompleted}, nil
}

type countingEvents struct {
	ticket       contracts.WorkflowTicket
	failAt       int
	publishCount int
	err          error
}

func (c *countingEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if c.ticket.SchemaVersion == "" {
		c.ticket.SchemaVersion = contracts.SchemaVersion
	}
	if c.ticket.TicketID == "" {
		c.ticket.TicketID = ticketID
	}
	if c.ticket.Tenant == "" {
		c.ticket.Tenant = "acme"
	}
	if c.ticket.Manifest.Name == "" || c.ticket.Manifest.Version == "" {
		c.ticket.Manifest = contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	return c.ticket, nil
}

func (c *countingEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	c.publishCount++
	if c.failAt > 0 && c.publishCount == c.failAt {
		if c.err == nil {
			c.err = errors.New("publish checkpoint failure")
		}
		return c.err
	}
	return nil
}

func (c *countingEvents) PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error {
	_ = ctx
	return nil
}

type stageStatusEntry struct {
	stage  string
	status runner.StageStatus
}

func extractStageStatuses(checkpoints []contracts.WorkflowCheckpoint) []stageStatusEntry {
	result := make([]stageStatusEntry, 0, len(checkpoints))
	for _, cp := range checkpoints {
		result = append(result, stageStatusEntry{stage: cp.Stage, status: runner.StageStatus(cp.Status)})
	}
	return result
}

func compareSequences(actual, expected []stageStatusEntry) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("length mismatch: got %d want %d", len(actual), len(expected))
	}
	for i := range actual {
		a := actual[i]
		e := expected[i]
		if a.stage != e.stage || a.status != e.status {
			return fmt.Errorf("entry %d mismatch: got %s/%s want %s/%s", i, a.stage, a.status, e.stage, e.status)
		}
	}
	return nil
}

type recordingEvents struct {
	tenant         string
	nextTicket     string
	invalidTicket  bool
	manifest       contracts.ManifestReference
	claimedTickets []string
	checkpoints    []contracts.WorkflowCheckpoint
	artifacts      []contracts.WorkflowArtifact
}

func (r *recordingEvents) ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error) {
	_ = ctx
	if ticketID == "" {
		ticketID = r.nextTicket
	}
	r.claimedTickets = append(r.claimedTickets, ticketID)
	if r.invalidTicket {
		return contracts.WorkflowTicket{}, nil
	}
	ref := r.manifest
	if ref.Name == "" && ref.Version == "" {
		ref = contracts.ManifestReference{Name: "smoke", Version: "2025-09-26"}
	}
	return contracts.WorkflowTicket{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      ticketID,
		Tenant:        r.tenant,
		Manifest:      ref,
	}, nil
}

func (r *recordingEvents) PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error {
	_ = ctx
	r.checkpoints = append(r.checkpoints, checkpoint)
	return nil
}

func (r *recordingEvents) PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error {
	_ = ctx
	r.artifacts = append(r.artifacts, artifact)
	return nil
}

type gridCall struct {
	stage     runner.Stage
	workspace string
}

type fakeGrid struct {
	outcomes      map[string][]runner.StageOutcome
	calls         []gridCall
	lastWorkspace string
}

func (g *fakeGrid) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	_ = ctx
	_ = ticket
	g.calls = append(g.calls, gridCall{stage: stage, workspace: workspace})
	g.lastWorkspace = workspace
	queue := g.outcomes[stage.Name]
	if len(queue) == 0 {
		return runner.StageOutcome{Stage: stage, Status: runner.StageStatusCompleted}, nil
	}
	outcome := queue[0]
	g.outcomes[stage.Name] = queue[1:]
	if outcome.Stage.Name == "" {
		outcome.Stage = stage
	}
	return outcome, nil
}

func gatherStageAttempts(calls []gridCall, stage string) int {
	count := 0
	for _, call := range calls {
		if call.stage.Name == stage {
			count++
		}
	}
	return count
}

func findStageCall(calls []gridCall, stageName string) gridCall {
	for _, call := range calls {
		if call.stage.Name == stageName {
			return call
		}
	}
	return gridCall{}
}

type failingPlanner struct {
	err error
}

func (f failingPlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	_ = ticket
	return runner.ExecutionPlan{}, f.err
}

type invalidStagePlanner struct{}

func (invalidStagePlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	return runner.ExecutionPlan{
		TicketID: ticket.TicketID,
		Stages: []runner.Stage{
			{Name: "", Kind: runner.StageKindModsPlan, Lane: "node-wasm"},
		},
	}, nil
}

type missingLanePlanner struct{}

func (missingLanePlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (runner.ExecutionPlan, error) {
	_ = ctx
	return runner.ExecutionPlan{
		TicketID: ticket.TicketID,
		Stages: []runner.Stage{
			{Name: modsPlanStage, Kind: runner.StageKindModsPlan, Lane: ""},
		},
	}, nil
}

func withCleanupDeadline(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		<-ctx.Done()
	})
}

type failingCompiler struct {
	err error
}

func (f failingCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	_ = ref
	return manifests.Compilation{}, f.err
}

type recordingCompiler struct {
	compiled manifests.Compilation
	ref      contracts.ManifestReference
}

func (r *recordingCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	r.ref = ref
	return r.compiled, nil
}

type stubAsterLocator struct {
	bundles map[string]aster.Metadata
}

func (s *stubAsterLocator) Locate(ctx context.Context, req aster.Request) (aster.Metadata, error) {
	_ = ctx
	key := fmt.Sprintf("%s/%s", strings.ToLower(strings.TrimSpace(req.Stage)), strings.ToLower(strings.TrimSpace(req.Toggle)))
	if meta, ok := s.bundles[key]; ok {
		return meta, nil
	}
	return aster.Metadata{}, aster.ErrBundleNotFound
}
