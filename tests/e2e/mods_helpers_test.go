//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	PlanTimeout     time.Duration
	ModsMaxParallel int
	WorkspaceHint   string
}

type scenarioHarness struct {
	t           *testing.T
	scenario    Scenario
	options     scenarioOptions
	tenant      string
	ticket      string
	grid        runner.GridClient
	recorder    *capturingGrid
	bus         *contracts.InMemoryBus
	workspace   *recordingWorkspacePreparer
	advisor     *recordingAdvisor
	jobComposer runner.JobComposer
	compiler    staticManifestCompiler
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

	cfg := LoadConfig()
	if cfg.SkipReason != "" {
		t.Fatalf("live grid config missing: %s", cfg.SkipReason)
	}
	client, stateDir, err := liveGridClient(cfg)
	if err != nil {
		t.Fatalf("create live grid client: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(stateDir)
	})

	recorder := newCapturingGrid(client)
	harness.grid = recorder
	harness.recorder = recorder

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

	harness.jobComposer = runner.NewStaticJobComposer()

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
		JobComposer:       h.jobComposer,
		WorkspacePreparer: h.workspace,
		Mods:              modsOpts,
	}

	return runner.Run(context.Background(), opts)
}

func (h *scenarioHarness) stageNames() []string {
	if h.recorder == nil {
		return nil
	}
	invocations := h.recorder.Invocations()
	names := make([]string, len(invocations))
	for i, inv := range invocations {
		names[i] = inv.Stage.Name
	}
	return names
}

func (h *scenarioHarness) stageByName(name string) (runner.Stage, bool) {
	if h.recorder == nil {
		return runner.Stage{}, false
	}
	invocations := h.recorder.Invocations()
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
		{Name: "build-gate"},
		{Name: "static-checks"},
		{Name: "test"},
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
