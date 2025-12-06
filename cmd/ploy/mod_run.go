package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// handleModRun executes the Mods-specific run command.
// Routes to repo subcommands when args[0] == "repo", otherwise executes
// the standard mod run workflow for single-repo ticket submission.
func handleModRun(args []string, stderr io.Writer) error {
	// Check for "repo" subcommand to route to batch repo operations.
	// This enables `ploy mod run repo add/remove/restart/status` workflows.
	if len(args) > 0 && args[0] == "repo" {
		return handleModRunRepo(args[1:], stderr)
	}
	return executeModRun(args, stderr)
}

// executeModRun orchestrates the full mod run workflow:
// 1. Parse CLI flags
// 2. Build and submit ticket request
// 3. Follow ticket events (if requested)
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

	// Build ticket request from parsed flags.
	request, err := buildTicketRequest(flags)
	if err != nil {
		printModRunUsage(stderr)
		return err
	}

	// Load spec from file (if provided) and merge with CLI overrides.
	// CLI flags take precedence over spec file values.
	specPayload, err := buildSpecPayload(
		strings.TrimSpace(*flags.SpecFile),
		*flags.ModEnvs,
		strings.TrimSpace(*flags.ModImage),
		*flags.Retain,
		strings.TrimSpace(*flags.ModCommand),
		strings.TrimSpace(*flags.GitLabPAT),
		strings.TrimSpace(*flags.GitLabDomain),
		*flags.MRSuccess,
		*flags.MRFail,
		*flags.HealOnBuild,
	)
	if err != nil {
		return fmt.Errorf("build spec: %w", err)
	}

	// Submit ticket to control plane.
	summary, err := submitTicket(ctx, base, httpClient, request, specPayload)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stderr, "Mods ticket %s submitted (state: %s)\n", summary.TicketID, summary.State)

	// Track states for JSON output.
	initialState := strings.ToLower(string(summary.State))
	finalState := ""

	// Follow ticket events if requested.
	if *flags.Follow {
		final, err := followTicketEvents(ctx, base, httpClient, string(summary.TicketID), flags, stderr)
		if err != nil {
			return err
		}
		finalState = strings.ToLower(string(final))

		// Download artifacts after successful completion.
		if artifactDir := strings.TrimSpace(*flags.ArtifactDir); artifactDir != "" {
			if err := downloadTicketArtifacts(ctx, base, httpClient, string(summary.TicketID), artifactDir, stderr); err != nil {
				return err
			}
		}
	}

	// Output JSON summary if requested.
	if *flags.JSONOut {
		if err := outputJSONSummary(ctx, base, httpClient, string(summary.TicketID), initialState, finalState, flags); err != nil {
			return err
		}
	}

	return nil
}
