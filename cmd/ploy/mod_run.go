package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// handleModRun executes the Mods-specific run command.
// Routes to batch lifecycle subcommands (list/stop/start), repo
// subcommands, and pull when args[0] matches a known action. Otherwise executes
// the standard mod run workflow for single-repo run submission.
//
// Batch lifecycle commands:
//   - list: Lists runs with status and repo counts.
//   - stop <id>: Stops a run and cancels pending repos.
//   - start <id>: Starts execution for pending repos in a run.
//   - repo <action>: Routes to repo-level operations (add/remove/restart/status).
//   - pull [--origin <remote>] [--dry-run] <run-name|run-id>: Pulls Mods diffs into a local git repository.
func handleModRun(args []string, stderr io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		// Batch lifecycle commands: list/stop/start.
		case "list":
			return handleModRunList(args[1:], stderr)
		case "stop":
			return handleModRunStop(args[1:], stderr)
		case "start":
			return handleModRunStart(args[1:], stderr)
		// Repo-level operations for managing repos within a batch.
		case "repo":
			return handleModRunRepo(args[1:], stderr)
		// Pull Mods diffs into local git repository.
		case "pull":
			return handleModRunPull(args[1:], stderr)
		}
	}
	return executeModRun(args, stderr)
}

// executeModRun orchestrates the full mod run workflow:
// 1. Parse CLI flags
// 2. Build and submit run request
// 3. Follow run events (if requested)
// 4. Download artifacts (if requested)
// 5. Output JSON summary (if requested)
func executeModRun(args []string, stderr io.Writer) error {
	// Parse CLI flags using extracted flag handling.
	flags, err := parseModRunFlags(args)
	if err != nil {
		printModRunUsage(stderr)
		return err
	}

	ctx := context.Background()

	// Resolve control plane connection.
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Build run request from parsed flags.
	request, err := buildRunRequest(flags)
	if err != nil {
		printModRunUsage(stderr)
		return err
	}

	// Submit run to control plane using canonical 201 Created contract.
	// The server creates the run directly from the RunSubmitRequest.
	summary, err := submitRun(ctx, base, httpClient, request)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stderr, "Mods run %s submitted (state: %s)\n", summary.RunID, summary.State)

	// Track states for JSON output.
	initialState := strings.ToLower(string(summary.State))
	finalState := ""

	// Follow run events if requested.
	if *flags.Follow {
		final, err := followRunEvents(ctx, base, httpClient, string(summary.RunID), flags, stderr)
		if err != nil {
			return err
		}
		finalState = strings.ToLower(string(final))

		// Download artifacts after successful completion.
		if artifactDir := strings.TrimSpace(*flags.ArtifactDir); artifactDir != "" {
			if err := downloadRunArtifacts(ctx, base, httpClient, string(summary.RunID), artifactDir, stderr); err != nil {
				return err
			}
		}
	}

	// Output JSON summary if requested.
	if *flags.JSONOut {
		if err := outputJSONSummary(ctx, base, httpClient, summary.RunID, initialState, finalState, flags); err != nil {
			return err
		}
	}

	return nil
}
