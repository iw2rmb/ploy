//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

type scenarioOptions struct {
	Advice          mods.Advice
	GridOutcomes    map[string][]runner.StageOutcome
	PlanTimeout     time.Duration
	ModsMaxParallel int
	WorkspaceHint   string
}

type scenarioHarness struct {
	t         *testing.T
	scenario  Scenario
	options   scenarioOptions
	tenant    string
	ticket    string
	grid      *runner.InMemoryGrid
	bus       *contracts.InMemoryBus
	workspace *recordingWorkspacePreparer
	advisor   *recordingAdvisor
	compiler  staticManifestCompiler
}

type staticManifestCompiler struct {
	compilation manifests.Compilation
}

func (s staticManifestCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	return s.compilation, nil
}

type recordingWorkspacePreparer struct {
	calls []runner.WorkspacePrepareRequest
}

func (p *recordingWorkspacePreparer) Prepare(ctx context.Context, req runner.WorkspacePrepareRequest) error {
	_ = ctx
	p.calls = append(p.calls, req)
	return nil
}

type recordingAdvisor struct {
	advice mods.Advice
	calls  []mods.AdviceRequest
}

func (a *recordingAdvisor) Advise(ctx context.Context, req mods.AdviceRequest) (mods.Advice, error) {
	_ = ctx
	a.calls = append(a.calls, req)
	return a.advice, nil
}

func newScenarioHarness(t *testing.T, scenario Scenario, opts scenarioOptions) *scenarioHarness {
	t.Helper()

	harness := &scenarioHarness{
		t:        t,
		scenario: scenario,
		options:  opts,
		tenant:   "acme",
		ticket:   fmt.Sprintf("ticket-%s", scenario.ID),
		compiler: staticManifestCompiler{compilation: newModsCompilation()},
	}

	if harness.options.WorkspaceHint == "" {
		harness.options.WorkspaceHint = "mods/java"
	}

	harness.grid = runner.NewInMemoryGrid()
	if len(harness.options.GridOutcomes) > 0 {
		for name, outcomes := range harness.options.GridOutcomes {
			harness.grid.StageOutcomes[name] = append([]runner.StageOutcome(nil), outcomes...)
		}
	}

	repo := contracts.RepoMaterialization{
		URL:           strings.TrimSpace(scenario.RepoURL),
		BaseRef:       strings.TrimSpace(scenario.BaseRef),
		TargetRef:     strings.TrimSpace(scenario.FailureRef),
		WorkspaceHint: strings.TrimSpace(harness.options.WorkspaceHint),
	}
	if repo.URL == "" {
		repo.URL = fmt.Sprintf("https://example.com/%s.git", scenario.ID)
	}
	if repo.TargetRef == "" {
		repo.TargetRef = fmt.Sprintf("mods/%s", scenario.ID)
	}
	if repo.BaseRef == "" {
		repo.BaseRef = "main"
	}

	harness.bus = contracts.NewInMemoryBus(harness.tenant)
	harness.bus.Manifest = contracts.ManifestReference{Name: harness.compiler.compilation.Manifest.Name, Version: harness.compiler.compilation.Manifest.Version}
	harness.bus.Repo = repo

	harness.workspace = &recordingWorkspacePreparer{}

	harness.advisor = &recordingAdvisor{advice: normalizeAdvice(opts.Advice)}

	return harness
}

func (h *scenarioHarness) run() error {
	modsOpts := runner.ModsOptions{
		PlanTimeout:     h.options.PlanTimeout,
		MaxParallel:     h.options.ModsMaxParallel,
		Advisor:         h.advisor,
		PlanLane:        "mods-plan",
		OpenRewriteLane: "mods-java",
		LLMPlanLane:     "mods-llm",
		LLMExecLane:     "mods-llm",
		HumanLane:       "mods-human",
	}

	opts := runner.Options{
		Ticket:            h.ticket,
		Tenant:            h.tenant,
		Events:            h.bus,
		Grid:              h.grid,
		Planner:           runner.NewDefaultPlannerWithMods(modsOpts),
		MaxStageRetries:   1,
		ManifestCompiler:  h.compiler,
		WorkspacePreparer: h.workspace,
		Mods:              modsOpts,
	}

	return runner.Run(context.Background(), opts)
}

func (h *scenarioHarness) stageNames() []string {
	invocations := h.grid.Invocations()
	names := make([]string, len(invocations))
	for i, inv := range invocations {
		names[i] = inv.Stage.Name
	}
	return names
}

func (h *scenarioHarness) stageByName(name string) (runner.Stage, bool) {
	invocations := h.grid.Invocations()
	for _, inv := range invocations {
		if inv.Stage.Name == name {
			return inv.Stage, true
		}
	}
	return runner.Stage{}, false
}

func (h *scenarioHarness) workspaceRequests() []runner.WorkspacePrepareRequest {
	return append([]runner.WorkspacePrepareRequest(nil), h.workspace.calls...)
}

func (h *scenarioHarness) advisorRequests() []mods.AdviceRequest {
	return append([]mods.AdviceRequest(nil), h.advisor.calls...)
}

func newModsCompilation() manifests.Compilation {
	required := []manifests.Lane{
		{Name: "mods-plan"},
		{Name: "mods-java"},
		{Name: "mods-llm"},
		{Name: "mods-human"},
		{Name: "go-native"},
	}
	return manifests.Compilation{
		Manifest:        manifests.Metadata{Name: "smoke", Version: "2025-09-26"},
		ManifestVersion: "v2",
		Lanes:           manifests.LaneSet{Required: required},
	}
}

func normalizeAdvice(advice mods.Advice) mods.Advice {
	normalized := advice
	if normalized.Plan.SelectedRecipes == nil {
		normalized.Plan.SelectedRecipes = []string{"org.openrewrite.java.UpgradeJavaVersion"}
	}
	if normalized.Plan.ParallelStages == nil {
		normalized.Plan.ParallelStages = []string{mods.StageNameORWApply, mods.StageNameORWGenerate}
	}
	if strings.TrimSpace(normalized.Plan.Summary) == "" {
		normalized.Plan.Summary = "Default Mods plan"
	}
	return normalized
}

func scenarioIndex(names []string, target string) int {
	for i, name := range names {
		if name == target {
			return i
		}
	}
	return -1
}

func mustScenario(t *testing.T, id string) Scenario {
	t.Helper()
	scenario, ok := ScenarioByID(id)
	if !ok {
		t.Fatalf("scenario %q not found", id)
	}
	return scenario
}
