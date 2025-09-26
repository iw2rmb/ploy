package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
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
