package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// handleWorkflowCancel wires the workflow cancel subcommand onto the Grid client.
func handleWorkflowCancel(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("workflow cancel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	tenant := fs.String("tenant", "", "tenant slug owning the workflow run")
	runID := fs.String("run-id", "", "workflow run identifier to cancel")
	workflowID := fs.String("workflow", "", "optional workflow identifier to validate")
	reason := fs.String("reason", "", "optional reason recorded alongside the cancellation")
	if err := fs.Parse(args); err != nil {
		printWorkflowCancelUsage(stderr)
		return err
	}

	trimmedTenant := strings.TrimSpace(*tenant)
	if trimmedTenant == "" {
		printWorkflowCancelUsage(stderr)
		return errors.New("tenant required")
	}
	trimmedRunID := strings.TrimSpace(*runID)
	if trimmedRunID == "" {
		printWorkflowCancelUsage(stderr)
		return errors.New("run id required")
	}

	gridClient, err := gridFactory()
	if err != nil {
		return fmt.Errorf("configure grid client: %w", err)
	}
	result, err := gridClient.CancelWorkflow(context.Background(), runner.CancelRequest{
		Tenant:     trimmedTenant,
		WorkflowID: strings.TrimSpace(*workflowID),
		RunID:      trimmedRunID,
		Reason:     strings.TrimSpace(*reason),
	})
	if errors.Is(err, runner.ErrGridCancellationUnsupported) {
		return fmt.Errorf("workflow cancellation requires a Grid endpoint; set GRID_ENDPOINT and retry")
	}
	if err != nil {
		return err
	}

	status := result.Status
	if status == "" {
		status = runner.StageStatusRunning
	}
	_, _ = fmt.Fprintf(stderr, "Cancellation requested for run %s (status=%s).\n", strings.TrimSpace(result.RunID), status)
	if !result.Requested {
		_, _ = fmt.Fprintln(stderr, "Run was already terminal when the request was processed.")
	}
	return nil
}
