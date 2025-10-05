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

// handleWorkflowRun executes the workflow run command flow.
func handleWorkflowRun(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("workflow run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ticket := fs.String("ticket", "auto", "ticket identifier to consume or 'auto'")
	tenant := fs.String("tenant", "", "tenant slug for subject mapping")
	asterGlobal := fs.String("aster", "", "comma-separated optional Aster toggles to include")
	var stageOverrides stageOverrideFlag
	fs.Var(&stageOverrides, "aster-step", "per-stage Aster toggles in the form stage=toggle1,toggle2 or stage=off")
	modsPlanTimeout := fs.Duration("mods-plan-timeout", 0, "planner timeout for Mods plan evaluation (e.g. 2m30s)")
	modsMaxParallel := fs.Int("mods-max-parallel", 0, "maximum Mods stages to run in parallel")
	if err := fs.Parse(args); err != nil {
		printWorkflowRunUsage(stderr)
		return err
	}

	if *modsPlanTimeout < 0 {
		printWorkflowRunUsage(stderr)
		return fmt.Errorf("mods plan timeout must be non-negative")
	}
	if *modsMaxParallel < 0 {
		printWorkflowRunUsage(stderr)
		return fmt.Errorf("mods max parallel must be non-negative")
	}

	trimmedTenant := strings.TrimSpace(*tenant)
	if trimmedTenant == "" {
		printWorkflowRunUsage(stderr)
		return errors.New("tenant required")
	}

	overrides, err := parseStageOverrides(stageOverrides.values)
	if err != nil {
		printWorkflowRunUsage(stderr)
		return err
	}

	compiler, err := manifestRegistryLoader(manifestConfigDir)
	if err != nil {
		return fmt.Errorf("load manifests: %w", err)
	}

	laneReg, err := laneRegistryLoader(laneConfigDir)
	if err != nil {
		return fmt.Errorf("load lanes: %w", err)
	}

	ticketValue := strings.TrimSpace(*ticket)
	if ticketValue == "" || strings.EqualFold(ticketValue, "auto") {
		ticketValue = ""
	}

	events, err := eventsFactory(trimmedTenant)
	if err != nil {
		return fmt.Errorf("configure events client: %w", err)
	}
	if closer, ok := events.(interface{ Close() }); ok {
		defer closer.Close()
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
	opts := runner.Options{
		Ticket:           ticketValue,
		Tenant:           trimmedTenant,
		Events:           events,
		Grid:             gridClient,
		Planner:          runner.NewDefaultPlannerWithMods(modsOptions),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
		CacheComposer:    laneCacheComposer{lanes: laneReg},
		JobComposer:      runner.LaneJobComposer{Lanes: laneReg},
		Mods:             modsOptions,
		Aster:            asterOpts,
	}
	err = runnerExecutor.Run(context.Background(), opts)
	if errors.Is(err, runner.ErrEventsClientRequired) || errors.Is(err, runner.ErrGridClientRequired) || errors.Is(err, runner.ErrTicketValidationFailed) || errors.Is(err, runner.ErrTicketRequired) {
		printWorkflowRunUsage(stderr)
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
