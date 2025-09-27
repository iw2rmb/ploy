package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// handleWorkflow routes workflow subcommands to their implementations.
func handleWorkflow(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printWorkflowUsage(stderr)
		return errors.New("workflow subcommand required")
	}

	switch args[0] {
	case "run":
		return handleWorkflowRun(args[1:], stderr)
	default:
		printWorkflowUsage(stderr)
		return fmt.Errorf("unknown workflow subcommand %q", args[0])
	}
}

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
	locator, err := asterLocatorLoader(asterConfigDir)
	if err != nil {
		return fmt.Errorf("load Aster bundles: %w", err)
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
		Mods:             modsOptions,
		Aster: runner.AsterOptions{
			Locator:           locator,
			AdditionalToggles: splitToggles(*asterGlobal),
			StageOverrides:    overrides,
		},
	}
	err = runnerExecutor.Run(context.Background(), opts)
	if errors.Is(err, runner.ErrEventsClientRequired) || errors.Is(err, runner.ErrGridClientRequired) || errors.Is(err, runner.ErrTicketValidationFailed) || errors.Is(err, runner.ErrTicketRequired) {
		printWorkflowRunUsage(stderr)
	}
	if err != nil {
		return err
	}
	if reporter, ok := interface{}(gridClient).(interface {
		Invocations() []runner.StageInvocation
	}); ok {
		printAsterSummary(stderr, reporter.Invocations())
	}
	return nil
}

// printWorkflowUsage details the workflow command usage information.
func printWorkflowUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  run    Consume a ticket and execute the workflow (stub)")
}

// printWorkflowRunUsage outputs the accepted workflow run flags.
func printWorkflowRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto] [--mods-plan-timeout <duration>] [--mods-max-parallel <n>]")
}

// printAsterSummary summarises the most recent Aster bundle usage per stage.
func printAsterSummary(w io.Writer, invocations []runner.StageInvocation) {
	if len(invocations) == 0 {
		return
	}
	latest := make(map[string]runner.Stage)
	for _, invocation := range invocations {
		stage := invocation.Stage
		if strings.TrimSpace(stage.Name) == "" {
			continue
		}
		latest[stage.Name] = stage
	}
	if len(latest) == 0 {
		return
	}
	names := make([]string, 0, len(latest))
	for name := range latest {
		names = append(names, name)
	}
	sort.Strings(names)
	_, _ = fmt.Fprintln(w, "Aster Bundles:")
	for _, name := range names {
		stage := latest[name]
		if !stage.Aster.Enabled || len(stage.Aster.Bundles) == 0 {
			_, _ = fmt.Fprintf(w, "  %s: disabled\n", name)
			continue
		}
		bundleSummaries := make([]string, len(stage.Aster.Bundles))
		for i, bundle := range stage.Aster.Bundles {
			id := strings.TrimSpace(bundle.BundleID)
			if id == "" {
				id = fmt.Sprintf("%s-%s", bundle.Stage, bundle.Toggle)
			}
			if bundle.ArtifactCID != "" {
				bundleSummaries[i] = fmt.Sprintf("%s (%s)", id, bundle.ArtifactCID)
			} else if bundle.Digest != "" {
				bundleSummaries[i] = fmt.Sprintf("%s [%s]", id, bundle.Digest)
			} else {
				bundleSummaries[i] = id
			}
		}
		_, _ = fmt.Fprintf(w, "  %s: %s (toggles: %s)\n", name, strings.Join(bundleSummaries, ", "), strings.Join(stage.Aster.Toggles, ", "))
	}
}

type stageOverrideFlag struct {
	values []string
}

// String returns the joined representation for the stage overrides flag.
func (f *stageOverrideFlag) String() string {
	return strings.Join(f.values, ",")
}

// Set appends a stage override value while validating empties.
func (f *stageOverrideFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("aster-step value cannot be empty")
	}
	f.values = append(f.values, trimmed)
	return nil
}

// parseStageOverrides interprets --aster-step arguments into runner overrides.
func parseStageOverrides(values []string) (map[string]runner.AsterStageOverride, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]runner.AsterStageOverride)
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --aster-step value: %s", value)
		}
		stage := strings.ToLower(strings.TrimSpace(parts[0]))
		if stage == "" {
			return nil, fmt.Errorf("invalid --aster-step value: stage is required (%s)", value)
		}
		payload := strings.TrimSpace(parts[1])
		override := result[stage]
		if strings.EqualFold(payload, "off") {
			override.Disable = true
			override.ExtraToggles = nil
			result[stage] = override
			continue
		}
		toggles := splitToggles(payload)
		if len(toggles) == 0 {
			return nil, fmt.Errorf("invalid --aster-step toggles for stage %s", stage)
		}
		override.Disable = false
		override.ExtraToggles = append(override.ExtraToggles, toggles...)
		result[stage] = override
	}
	return result, nil
}
