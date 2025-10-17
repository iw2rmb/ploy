package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// handleModRun executes the Mods-specific run command.
func handleModRun(args []string, stderr io.Writer) error {
	return executeModRun(args, stderr)
}

func executeModRun(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ticket := fs.String("ticket", "auto", "ticket identifier to consume or 'auto'")
	tenant := fs.String("tenant", "", "tenant slug for subject mapping")
	repoURL := fs.String("repo-url", "", "Git repository URL to materialise for Mods execution")
	repoBaseRef := fs.String("repo-base-ref", "", "Git base ref used for materialisation")
	repoTargetRef := fs.String("repo-target-ref", "", "Git target ref created for the run")
	repoWorkspace := fs.String("repo-workspace-hint", "", "Optional subdirectory hint when preparing the workspace")
	asterGlobal := fs.String("aster", "", "comma-separated optional Aster toggles to include")
	var stageOverrides stageOverrideFlag
	fs.Var(&stageOverrides, "aster-step", "per-stage Aster toggles in the form stage=toggle1,toggle2 or stage=off")
	modsPlanTimeout := fs.Duration("mods-plan-timeout", 0, "planner timeout for Mods plan evaluation (e.g. 2m30s)")
	modsMaxParallel := fs.Int("mods-max-parallel", 0, "maximum Mods stages to run in parallel")
	if err := fs.Parse(args); err != nil {
		printModRunUsage(stderr)
		return err
	}

	if *modsPlanTimeout < 0 {
		printModRunUsage(stderr)
		return fmt.Errorf("mods plan timeout must be non-negative")
	}
	if *modsMaxParallel < 0 {
		printModRunUsage(stderr)
		return fmt.Errorf("mods max parallel must be non-negative")
	}

	trimmedTenant := strings.TrimSpace(*tenant)
	if trimmedTenant == "" {
		printModRunUsage(stderr)
		return errors.New("tenant required")
	}

	overrides, err := parseStageOverrides(stageOverrides.values)
	if err != nil {
		printModRunUsage(stderr)
		return err
	}

	compiler, err := manifestRegistryLoader(manifestConfigDir)
	if err != nil {
		return fmt.Errorf("load manifests: %w", err)
	}

	ticketValue := strings.TrimSpace(*ticket)
	if ticketValue == "" || strings.EqualFold(ticketValue, "auto") {
		ticketValue = ""
	}

	repoSpec := contracts.RepoMaterialization{
		URL:           strings.TrimSpace(*repoURL),
		BaseRef:       strings.TrimSpace(*repoBaseRef),
		TargetRef:     strings.TrimSpace(*repoTargetRef),
		WorkspaceHint: strings.TrimSpace(*repoWorkspace),
	}
	if repoSpec.URL != "" && repoSpec.TargetRef == "" {
		printModRunUsage(stderr)
		return fmt.Errorf("repo target ref required when repo url is set")
	}

	events, err := eventsFactory(trimmedTenant)
	if err != nil {
		return fmt.Errorf("configure events client: %w", err)
	}
	if closer, ok := events.(interface{ Close() }); ok {
		defer closer.Close()
	}
	if repoSpec.URL != "" {
		events = newRepoAugmentedEventsClient(events, repoSpec)
	}
	gridClient, err := gridFactory()
	if err != nil {
		return fmt.Errorf("configure grid client: %w", err)
	}
	var asterOpts runner.AsterOptions
	if asterEnabled() {
		locator, err := asterLocatorLoader(asterConfigDir)
		if err != nil {
			return fmt.Errorf("load Aster bundles: %w", err)
		}
		asterOpts = runner.AsterOptions{
			Enabled:           true,
			Locator:           locator,
			AdditionalToggles: splitToggles(*asterGlobal),
			StageOverrides:    overrides,
		}
	}
	modsOptions := runner.ModsOptions{PlanTimeout: *modsPlanTimeout, MaxParallel: *modsMaxParallel}
	advisor, err := knowledgeBaseAdvisorLoader(knowledgeBaseCatalogPath)
	if err != nil {
		return fmt.Errorf("load knowledge base: %w", err)
	}
	modsOptions.Advisor = advisor
	modsOptions.PlanLane = "mods-plan"
	modsOptions.OpenRewriteLane = "mods-java"
	modsOptions.LLMPlanLane = "mods-llm"
	modsOptions.LLMExecLane = "mods-llm"
	modsOptions.HumanLane = "mods-human"
	workspacePrep, err := workspacePreparerFactory()
	if err != nil {
		return fmt.Errorf("configure workspace preparer: %w", err)
	}
	cacheComposer := cacheComposerFactory()
	jobComposer := jobComposerFactory()
	opts := runner.Options{
		Ticket:            ticketValue,
		Tenant:            trimmedTenant,
		Events:            events,
		Grid:              gridClient,
		Planner:           runner.NewDefaultPlannerWithMods(modsOptions),
		MaxStageRetries:   1,
		ManifestCompiler:  compiler,
		CacheComposer:     cacheComposer,
		JobComposer:       jobComposer,
		Mods:              modsOptions,
		Aster:             asterOpts,
		WorkspacePreparer: workspacePrep,
	}
	err = runnerExecutor.Run(context.Background(), opts)
	if errors.Is(err, runner.ErrEventsClientRequired) || errors.Is(err, runner.ErrGridClientRequired) || errors.Is(err, runner.ErrTicketValidationFailed) || errors.Is(err, runner.ErrTicketRequired) {
		printModRunUsage(stderr)
	}
	if err != nil {
		return err
	}
	if recorder, ok := events.(interface {
		RecordedCheckpoints() []contracts.WorkflowCheckpoint
	}); ok {
		printBuildGateSummary(stderr, recorder.RecordedCheckpoints())
	}
	if reporter, ok := interface{}(gridClient).(interface {
		Invocations() []runner.StageInvocation
	}); ok {
		invocations := reporter.Invocations()
		if asterOpts.Enabled {
			printAsterSummary(stderr, invocations)
		}
		printArchiveSummary(stderr, invocations)
	}
	return nil
}

func printModRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run --tenant <tenant> [--ticket <ticket-id>|--ticket auto] [--repo-url <url> --repo-base-ref <branch> --repo-target-ref <branch> --repo-workspace-hint <dir>] [--mods-plan-timeout <duration>] [--mods-max-parallel <n>]")
}
