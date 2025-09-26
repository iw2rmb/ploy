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

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
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
)

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
	if err := fs.Parse(args); err != nil {
		printWorkflowRunUsage(stderr)
		return err
	}

	trimmedTenant := strings.TrimSpace(*tenant)
	if trimmedTenant == "" {
		printWorkflowRunUsage(stderr)
		return errors.New("tenant required")
	}

	ticketValue := strings.TrimSpace(*ticket)
	if ticketValue == "" || strings.EqualFold(ticketValue, "auto") {
		ticketValue = ""
	}

	events := eventsFactory(trimmedTenant)
	grid := runner.NewInMemoryGrid()
	opts := runner.Options{
		Ticket:          ticketValue,
		Tenant:          trimmedTenant,
		Events:          events,
		Grid:            grid,
		Planner:         runner.NewDefaultPlanner(),
		MaxStageRetries: 1,
	}
	err := runnerExecutor.Run(context.Background(), opts)
	if errors.Is(err, runner.ErrEventsClientRequired) || errors.Is(err, runner.ErrGridClientRequired) || errors.Is(err, runner.ErrTicketValidationFailed) || errors.Is(err, runner.ErrTicketRequired) {
		printWorkflowRunUsage(stderr)
	}
	return err
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
}

func printWorkflowUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  run    Consume a ticket and execute the workflow (stub)")
}

func printWorkflowRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto]")
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
