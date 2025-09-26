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

var (
	runnerExecutor runnerInvoker     = runnerInvokerFunc(runner.Run)
	eventsFactory  eventsFactoryFunc = func(tenant string) runner.EventsClient {
		return contracts.NewInMemoryBus(tenant)
	}
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
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto]")
}

func printWorkflowUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow <command>")
	_, _ = fmt.Fprintln(w, "\nCommands:")
	_, _ = fmt.Fprintln(w, "  run    Consume a ticket and execute the workflow (stub)")
}

func printWorkflowRunUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy workflow run --tenant <tenant> [--ticket <ticket-id>|--ticket auto]")
}
