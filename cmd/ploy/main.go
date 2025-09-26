package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
)

type runnerInvoker interface {
	Run(ctx context.Context, opts runner.Options) error
}

type runnerInvokerFunc func(context.Context, runner.Options) error

func (f runnerInvokerFunc) Run(ctx context.Context, opts runner.Options) error {
	return f(ctx, opts)
}

type eventsFactoryFunc func(tenant string) runner.EventsClient

type laneRegistry interface {
	Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error)
}

type laneRegistryLoaderFunc func(dir string) (laneRegistry, error)

type snapshotRegistry interface {
	Plan(ctx context.Context, name string) (snapshots.PlanReport, error)
	Capture(ctx context.Context, name string, opts snapshots.CaptureOptions) (snapshots.CaptureResult, error)
}

type snapshotRegistryLoaderFunc func(dir string) (snapshotRegistry, error)

type manifestCompilerLoaderFunc func(dir string) (runner.ManifestCompiler, error)

type environmentService interface {
	Materialize(ctx context.Context, req environments.Request) (environments.Result, error)
}

type environmentFactoryFunc func(l laneRegistry, s snapshotRegistry) (environmentService, error)

type asterLocatorLoaderFunc func(dir string) (aster.Locator, error)

type laneCacheComposer struct {
	lanes laneRegistry
}

func (c laneCacheComposer) Compose(ctx context.Context, req runner.CacheComposeRequest) (string, error) {
	_ = ctx
	if c.lanes == nil {
		return "", fmt.Errorf("lane registry unavailable")
	}
	manifestVersion := req.Stage.Constraints.Manifest.Manifest.Version
	desc, err := c.lanes.Describe(req.Stage.Lane, lanes.DescribeOptions{
		ManifestVersion: manifestVersion,
		AsterToggles:    req.Stage.Aster.Toggles,
	})
	if err != nil {
		return "", err
	}
	return desc.CacheKey, nil
}

type stageOverrideFlag struct {
	values []string
}

func (f *stageOverrideFlag) String() string {
	return strings.Join(f.values, ",")
}

func (f *stageOverrideFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("aster-step value cannot be empty")
	}
	f.values = append(f.values, trimmed)
	return nil
}

var (
	runnerExecutor runnerInvoker     = runnerInvokerFunc(runner.Run)
	eventsFactory  eventsFactoryFunc = func(tenant string) runner.EventsClient {
		return contracts.NewInMemoryBus(tenant)
	}
)

var (
	laneRegistryLoader laneRegistryLoaderFunc = func(dir string) (laneRegistry, error) {
		return lanes.LoadDirectory(dir)
	}
	laneConfigDir = "configs/lanes"

	snapshotRegistryLoader snapshotRegistryLoaderFunc = func(dir string) (snapshotRegistry, error) {
		return snapshots.LoadDirectory(dir, snapshots.LoadOptions{})
	}
	snapshotConfigDir = "configs/snapshots"

	manifestRegistryLoader manifestCompilerLoaderFunc = func(dir string) (runner.ManifestCompiler, error) {
		registry, err := manifests.LoadDirectory(dir)
		if err != nil {
			return nil, err
		}
		return registryCompiler{registry: registry}, nil
	}
	manifestConfigDir = "configs/manifests"

	asterLocatorLoader asterLocatorLoaderFunc = func(dir string) (aster.Locator, error) {
		return aster.NewFilesystemLocator(dir)
	}
	asterConfigDir = "configs/aster"

	environmentServiceFactory environmentFactoryFunc = func(l laneRegistry, s snapshotRegistry) (environmentService, error) {
		if l == nil {
			return nil, fmt.Errorf("environment lane registry missing")
		}
		if s == nil {
			return nil, fmt.Errorf("environment snapshot registry missing")
		}
		hydrator := environments.NewInMemoryHydrator()
		return environments.NewService(environments.ServiceOptions{
			Lanes:     l,
			Snapshots: s,
			Hydrator:  hydrator,
		}), nil
	}
)

type registryCompiler struct {
	registry *manifests.Registry
}

func (r registryCompiler) Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error) {
	_ = ctx
	if r.registry == nil {
		return manifests.Compilation{}, fmt.Errorf("compile manifest: registry missing")
	}
	comp, err := r.registry.Compile(manifests.CompileOptions{Name: ref.Name, Version: ref.Version})
	if err != nil {
		return manifests.Compilation{}, err
	}
	return comp, nil
}

func main() {
	if err := execute(os.Args[1:], os.Stderr); err != nil {
		reportError(err, os.Stderr)
		os.Exit(1)
	}
}

func execute(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("command required")
	}

	switch args[0] {
	case "workflow":
		return handleWorkflow(args[1:], stderr)
	case "lanes":
		return handleLanes(args[1:], stderr)
	case "snapshot":
		return handleSnapshot(args[1:], stderr)
	case "environment":
		return handleEnvironment(args[1:], stderr)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

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

func handleWorkflowRun(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("workflow run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ticket := fs.String("ticket", "auto", "ticket identifier to consume or 'auto'")
	tenant := fs.String("tenant", "", "tenant slug for subject mapping")
	asterGlobal := fs.String("aster", "", "comma-separated optional Aster toggles to include")
	var stageOverrides stageOverrideFlag
	fs.Var(&stageOverrides, "aster-step", "per-stage Aster toggles in the form stage=toggle1,toggle2 or stage=off")
	if err := fs.Parse(args); err != nil {
		printWorkflowRunUsage(stderr)
		return err
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

	events := eventsFactory(trimmedTenant)
	grid := runner.NewInMemoryGrid()
	locator, err := asterLocatorLoader(asterConfigDir)
	if err != nil {
		return fmt.Errorf("load Aster bundles: %w", err)
	}
	opts := runner.Options{
		Ticket:           ticketValue,
		Tenant:           trimmedTenant,
		Events:           events,
		Grid:             grid,
		Planner:          runner.NewDefaultPlanner(),
		MaxStageRetries:  1,
		ManifestCompiler: compiler,
		CacheComposer:    laneCacheComposer{lanes: laneReg},
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
	if reporter, ok := interface{}(grid).(interface {
		Invocations() []runner.StageInvocation
	}); ok {
		printAsterSummary(stderr, reporter.Invocations())
	}
	return nil
}

func reportError(err error, stderr io.Writer) {
	_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  workflow  Manage workflow execution entries")
	_, _ = fmt.Fprintln(w, "  lanes     Inspect lane definitions and cache previews")
	_, _ = fmt.Fprintln(w, "  snapshot  Plan and capture database snapshots")
	_, _ = fmt.Fprintln(w, "  environment  Materialize commit-scoped environments")
}

func printWorkflowUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  run    Consume a ticket and execute the workflow (stub)")
}

func printWorkflowRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto]")
}

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

func handleLanes(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printLanesUsage(stderr)
		return errors.New("lanes subcommand required")
	}

	switch args[0] {
	case "describe":
		return handleLanesDescribe(args[1:], stderr)
	default:
		printLanesUsage(stderr)
		return fmt.Errorf("unknown lanes subcommand %q", args[0])
	}
}

func printLanesUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy lanes <command>")
}

func handleLanesDescribe(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("lanes describe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	laneName := fs.String("lane", "", "lane identifier to describe")
	commit := fs.String("commit", "", "commit SHA for cache preview")
	snapshot := fs.String("snapshot", "", "snapshot fingerprint for cache preview")
	manifest := fs.String("manifest", "", "integration manifest version for cache preview")
	aster := fs.String("aster", "", "comma-separated Aster toggles for cache preview")
	if err := fs.Parse(args); err != nil {
		printLanesDescribeUsage(stderr)
		return err
	}

	trimmedLane := strings.TrimSpace(*laneName)
	if trimmedLane == "" {
		printLanesDescribeUsage(stderr)
		return errors.New("lane is required")
	}

	reg, err := laneRegistryLoader(laneConfigDir)
	if err != nil {
		return err
	}

	desc, err := reg.Describe(trimmedLane, lanes.DescribeOptions{
		CommitSHA:           strings.TrimSpace(*commit),
		SnapshotFingerprint: strings.TrimSpace(*snapshot),
		ManifestVersion:     strings.TrimSpace(*manifest),
		AsterToggles:        splitToggles(*aster),
	})
	if err != nil {
		return err
	}

	printLaneDescription(stderr, desc)
	return nil
}

func printLanesDescribeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy lanes describe --lane <lane-name> [--commit <sha>] [--snapshot <fingerprint>] [--manifest <version>] [--aster <a,b,c>]")
}

func splitToggles(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if candidate := strings.TrimSpace(part); candidate != "" {
			result = append(result, candidate)
		}
	}
	return result
}

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

func printLaneDescription(w io.Writer, desc lanes.Description) {
	_, _ = fmt.Fprintf(w, "Lane: %s\n", desc.Lane.Name)
	if desc.Lane.Description != "" {
		_, _ = fmt.Fprintf(w, "Description: %s\n", desc.Lane.Description)
	}
	_, _ = fmt.Fprintf(w, "Runtime Family: %s\n", desc.Lane.RuntimeFamily)
	_, _ = fmt.Fprintf(w, "Cache Namespace: %s\n", desc.Lane.CacheNamespace)
	if len(desc.Lane.Commands.Setup) > 0 {
		_, _ = fmt.Fprintf(w, "Setup Command: %s\n", strings.Join(desc.Lane.Commands.Setup, " "))
	}
	_, _ = fmt.Fprintf(w, "Build Command: %s\n", strings.Join(desc.Lane.Commands.Build, " "))
	_, _ = fmt.Fprintf(w, "Test Command: %s\n", strings.Join(desc.Lane.Commands.Test, " "))
	_, _ = fmt.Fprintf(w, "Cache Key Preview: %s\n", desc.CacheKey)

	inputs := []string{}
	if desc.Parameters.CommitSHA != "" {
		inputs = append(inputs, fmt.Sprintf("commit=%s", desc.Parameters.CommitSHA))
	}
	if desc.Parameters.SnapshotFingerprint != "" {
		inputs = append(inputs, fmt.Sprintf("snapshot=%s", desc.Parameters.SnapshotFingerprint))
	}
	if desc.Parameters.ManifestVersion != "" {
		inputs = append(inputs, fmt.Sprintf("manifest=%s", desc.Parameters.ManifestVersion))
	}
	if len(desc.Parameters.AsterToggles) > 0 {
		inputs = append(inputs, fmt.Sprintf("aster=%s", strings.Join(desc.Parameters.AsterToggles, ",")))
	}
	if len(inputs) > 0 {
		_, _ = fmt.Fprintf(w, "Inputs: %s\n", strings.Join(inputs, "; "))
	}
}

func handleSnapshot(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printSnapshotUsage(stderr)
		return errors.New("snapshot subcommand required")
	}

	switch args[0] {
	case "plan":
		return handleSnapshotPlan(args[1:], stderr)
	case "capture":
		return handleSnapshotCapture(args[1:], stderr)
	default:
		printSnapshotUsage(stderr)
		return fmt.Errorf("unknown snapshot subcommand %q", args[0])
	}
}

func printSnapshotUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy snapshot <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  plan     Preview strip/mask/synthetic rules for a snapshot")
	_, _ = fmt.Fprintln(w, "  capture  Execute snapshot capture and publish metadata")
}

func handleSnapshotPlan(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("snapshot plan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	snapshotName := fs.String("snapshot", "", "snapshot identifier to plan")
	if err := fs.Parse(args); err != nil {
		printSnapshotPlanUsage(stderr)
		return err
	}

	if strings.TrimSpace(*snapshotName) == "" {
		printSnapshotPlanUsage(stderr)
		return errors.New("snapshot is required")
	}

	reg, err := snapshotRegistryLoader(snapshotConfigDir)
	if err != nil {
		return err
	}

	report, err := reg.Plan(context.Background(), strings.TrimSpace(*snapshotName))
	if err != nil {
		return err
	}

	printSnapshotPlan(stderr, report)
	return nil
}

func printSnapshotPlanUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy snapshot plan --snapshot <snapshot-name>")
}

func printSnapshotPlan(w io.Writer, report snapshots.PlanReport) {
	_, _ = fmt.Fprintf(w, "Snapshot: %s\n", report.SnapshotName)
	if report.Description != "" {
		_, _ = fmt.Fprintf(w, "Description: %s\n", report.Description)
	}
	_, _ = fmt.Fprintf(w, "Engine: %s\n", report.Engine)
	if report.FixturePath != "" {
		_, _ = fmt.Fprintf(w, "Fixture: %s\n", report.FixturePath)
	}
	_, _ = fmt.Fprintf(w, "Strip Rules: %d\n", report.Stripping.Total)
	_, _ = fmt.Fprintf(w, "Mask Rules: %d\n", report.Masking.Total)
	_, _ = fmt.Fprintf(w, "Synthetic Rules: %d\n", report.Synthetic.Total)
	if len(report.Highlights) > 0 {
		_, _ = fmt.Fprintln(w, "Highlights:")
		for _, highlight := range report.Highlights {
			_, _ = fmt.Fprintf(w, "  - %s\n", highlight)
		}
	}
}

func handleSnapshotCapture(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("snapshot capture", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	snapshotName := fs.String("snapshot", "", "snapshot identifier to capture")
	tenant := fs.String("tenant", "", "tenant slug for metadata publishing")
	ticket := fs.String("ticket", "", "ticket identifier associated with capture")
	if err := fs.Parse(args); err != nil {
		printSnapshotCaptureUsage(stderr)
		return err
	}

	trimmedSnapshot := strings.TrimSpace(*snapshotName)
	if trimmedSnapshot == "" {
		printSnapshotCaptureUsage(stderr)
		return errors.New("snapshot is required")
	}
	trimmedTenant := strings.TrimSpace(*tenant)
	if trimmedTenant == "" {
		printSnapshotCaptureUsage(stderr)
		return errors.New("tenant is required")
	}
	trimmedTicket := strings.TrimSpace(*ticket)
	if trimmedTicket == "" {
		printSnapshotCaptureUsage(stderr)
		return errors.New("ticket is required")
	}

	reg, err := snapshotRegistryLoader(snapshotConfigDir)
	if err != nil {
		return err
	}

	result, err := reg.Capture(context.Background(), trimmedSnapshot, snapshots.CaptureOptions{
		Tenant:   trimmedTenant,
		TicketID: trimmedTicket,
	})
	if err != nil {
		return err
	}

	printSnapshotCapture(stderr, result)
	return nil
}

func printSnapshotCaptureUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy snapshot capture --snapshot <snapshot-name> --tenant <tenant> --ticket <ticket-id>")
}

func printSnapshotCapture(w io.Writer, result snapshots.CaptureResult) {
	_, _ = fmt.Fprintf(w, "Snapshot: %s\n", result.Metadata.SnapshotName)
	_, _ = fmt.Fprintf(w, "Artifact CID: %s\n", result.ArtifactCID)
	_, _ = fmt.Fprintf(w, "Fingerprint: %s\n", result.Fingerprint)
	_, _ = fmt.Fprintf(w, "Captured At: %s\n", result.Metadata.CapturedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(w, "Rule Totals: strip=%d mask=%d synthetic=%d\n", result.Metadata.RuleCounts.Strip, result.Metadata.RuleCounts.Mask, result.Metadata.RuleCounts.Synthetic)
	if len(result.Diff.StrippedColumns) > 0 {
		_, _ = fmt.Fprintln(w, "Stripped Columns:")
		printColumnSet(w, result.Diff.StrippedColumns)
	}
	if len(result.Diff.MaskedColumns) > 0 {
		_, _ = fmt.Fprintln(w, "Masked Columns:")
		printColumnSet(w, result.Diff.MaskedColumns)
	}
	if len(result.Diff.SyntheticColumns) > 0 {
		_, _ = fmt.Fprintln(w, "Synthetic Columns:")
		printColumnSet(w, result.Diff.SyntheticColumns)
	}
}

func printColumnSet(w io.Writer, values map[string][]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cols := append([]string(nil), values[key]...)
		sort.Strings(cols)
		_, _ = fmt.Fprintf(w, "  %s: %s\n", key, strings.Join(cols, ", "))
	}
}

func handleEnvironment(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printEnvironmentUsage(stderr)
		return errors.New("environment subcommand required")
	}

	switch args[0] {
	case "materialize":
		return handleEnvironmentMaterialize(args[1:], stderr)
	default:
		printEnvironmentUsage(stderr)
		return fmt.Errorf("unknown environment subcommand %q", args[0])
	}
}

func printEnvironmentUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy environment <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  materialize  Plan or hydrate a commit-scoped environment")
}

func handleEnvironmentMaterialize(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("environment materialize", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	app := fs.String("app", "", "application identifier")
	tenant := fs.String("tenant", "", "tenant slug for metadata publishing")
	dryRun := fs.Bool("dry-run", false, "plan resources without hydrating caches")
	manifestOverride := fs.String("manifest", "", "override manifest in the form name@version")
	aster := fs.String("aster", "", "comma-separated optional Aster toggles to include")

	commitArg := ""
	parseArgs := args
	if len(parseArgs) > 0 && !strings.HasPrefix(strings.TrimSpace(parseArgs[0]), "-") {
		commitArg = parseArgs[0]
		parseArgs = parseArgs[1:]
	}

	if err := fs.Parse(parseArgs); err != nil {
		printEnvironmentMaterializeUsage(stderr)
		return err
	}

	remaining := fs.Args()
	if commitArg == "" {
		if len(remaining) == 0 {
			printEnvironmentMaterializeUsage(stderr)
			return errors.New("commit SHA required")
		}
		commitArg = remaining[0]
		remaining = remaining[1:]
	}
	if len(remaining) > 0 {
		printEnvironmentMaterializeUsage(stderr)
		return fmt.Errorf("unexpected arguments: %s", strings.Join(remaining, " "))
	}

	commit := strings.TrimSpace(commitArg)
	if commit == "" {
		printEnvironmentMaterializeUsage(stderr)
		return errors.New("commit SHA required")
	}

	trimmedApp := strings.TrimSpace(*app)
	if trimmedApp == "" {
		printEnvironmentMaterializeUsage(stderr)
		return errors.New("app is required")
	}

	trimmedTenant := strings.TrimSpace(*tenant)
	if !*dryRun && trimmedTenant == "" {
		printEnvironmentMaterializeUsage(stderr)
		return errors.New("tenant is required")
	}

	manifestName, manifestVersion, err := parseManifestOverride(*manifestOverride, trimmedApp)
	if err != nil {
		printEnvironmentMaterializeUsage(stderr)
		return err
	}

	laneReg, err := laneRegistryLoader(laneConfigDir)
	if err != nil {
		return err
	}
	snapshotReg, err := snapshotRegistryLoader(snapshotConfigDir)
	if err != nil {
		return err
	}

	compiler, err := manifestRegistryLoader(manifestConfigDir)
	if err != nil {
		return fmt.Errorf("load manifests: %w", err)
	}

	compiled, err := compiler.Compile(context.Background(), contracts.ManifestReference{Name: manifestName, Version: manifestVersion})
	if err != nil {
		return err
	}

	service, err := environmentServiceFactory(laneReg, snapshotReg)
	if err != nil {
		return err
	}

	result, err := service.Materialize(context.Background(), environments.Request{
		CommitSHA:    commit,
		App:          trimmedApp,
		Tenant:       trimmedTenant,
		DryRun:       *dryRun,
		Manifest:     compiled,
		ManifestRef:  contracts.ManifestReference{Name: compiled.Manifest.Name, Version: compiled.Manifest.Version},
		AsterToggles: splitToggles(*aster),
	})
	if err != nil {
		return err
	}

	printEnvironmentMaterialize(stderr, result)
	return nil
}

func printEnvironmentMaterializeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy environment materialize <commit-sha> --app <app> --tenant <tenant> [--dry-run] [--manifest <name@version>] [--aster <toggle,...>]")
}

func printEnvironmentMaterialize(w io.Writer, result environments.Result) {
	_, _ = fmt.Fprintf(w, "Environment: %s@%s\n", result.App, result.CommitSHA)
	mode := "execute"
	if result.DryRun {
		mode = "dry-run"
	}
	_, _ = fmt.Fprintf(w, "Mode: %s\n", mode)
	_, _ = fmt.Fprintf(w, "Manifest: %s@%s\n", result.ManifestRef.Name, result.ManifestRef.Version)
	if len(result.AsterToggles) > 0 {
		_, _ = fmt.Fprintf(w, "Aster Toggles: %s\n", strings.Join(result.AsterToggles, ", "))
	}

	if len(result.Snapshots) == 0 {
		_, _ = fmt.Fprintln(w, "Snapshots: none")
	} else {
		_, _ = fmt.Fprintln(w, "Snapshots:")
		for _, snap := range result.Snapshots {
			status := "planned"
			if snap.Attached {
				status = "attached"
			}
			fingerprint := snap.Fingerprint
			if fingerprint == "" {
				fingerprint = "pending"
			}
			_, _ = fmt.Fprintf(w, "  - %s (%s, fingerprint=%s)\n", snap.Name, status, fingerprint)
		}
	}

	if len(result.Caches) == 0 {
		_, _ = fmt.Fprintln(w, "Caches: none")
	} else {
		_, _ = fmt.Fprintln(w, "Caches:")
		for _, cache := range result.Caches {
			status := "pending"
			if cache.Hydrated {
				status = "hydrated"
			}
			_, _ = fmt.Fprintf(w, "  - %s -> %s (%s)\n", cache.Lane, cache.CacheKey, status)
		}
	}
}

func parseManifestOverride(candidate, fallback string) (string, string, error) {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return fallback, "", nil
	}
	parts := strings.Split(trimmed, "@")
	if len(parts) > 2 {
		return "", "", errors.New("manifest override must be <name>@<version>")
	}
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return "", "", errors.New("manifest override requires a name")
	}
	version := ""
	if len(parts) == 2 {
		version = strings.TrimSpace(parts[1])
	}
	return name, version, nil
}
