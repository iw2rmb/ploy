// mig_run_project.go implements the 'ploy mig run <mig-id|name>' command handler for mig projects.
//
// This command creates a run from a mig project:
// - ploy mig run <mig-id|name> [--repo <repo-url> ...] [--failed]
// - Resolves <mig-id|name> to a mig (supports both ID and name).
// - Refuses when the mig is archived.
// - Uses migs.spec_id as the run's spec_id.
// - Selects repos:
//   - --repo ... → explicit repos (by repo_url identity within the mig)
//   - --failed → repos with last terminal state Fail
//   - omitted → all repos in the mig repo set
//
// - Creates a mig-scoped run via POST /v1/migs/{mig_id}/runs and immediately starts execution.
// - Prints run_id.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	climigs "github.com/iw2rmb/ploy/internal/cli/migs"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// handleMigRunProject implements 'ploy mig run <mig-id|name> [--repo <url>...] [--failed]'.
// This is the v1 mig project run command with repo selection.
func handleMigRunProject(args []string, stderr io.Writer) error {
	// Handle help flag (support `ploy mig run <mig> --help`).
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printMigRunProjectUsage(stderr)
			return nil
		}
	}

	// First positional arg is mig ID/name.
	if len(args) == 0 {
		printMigRunProjectUsage(stderr)
		return fmt.Errorf("mig id/name required")
	}
	migRef := args[0]

	// Parse remaining flags.
	fs := flag.NewFlagSet("mig run project", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	// Repo selection flags.
	// --repo can be repeated for explicit repo selection.
	// --failed selects repos with last terminal state Fail.
	var repoURLs stringSlice
	fs.Var(&repoURLs, "repo", "Explicit repo URL(s) to run (repeatable)")
	failed := fs.Bool("failed", false, "Run repos with last terminal state Fail")

	// Follow mode flags.
	followFlag := fs.Bool("follow", false, "Follow run until completion (shows job graph)")
	capDuration := fs.Duration("cap", 0, "Optional time cap for --follow")
	cancelOnCap := fs.Bool("cancel-on-cap", false, "Cancel run if cap exceeded")
	maxRetries := fs.Int("max-retries", 5, "Max SSE reconnect attempts")

	if err := fs.Parse(args[1:]); err != nil {
		printMigRunProjectUsage(stderr)
		return err
	}

	// Validate mutual exclusion: --failed and --repo cannot both be specified.
	if *failed && len(repoURLs) > 0 {
		printMigRunProjectUsage(stderr)
		return fmt.Errorf("--failed and --repo are mutually exclusive")
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mig reference to ID (supports both name and ID).
	resolveCmd := climigs.ResolveMigByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migRef),
	}
	migID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mig run command with repo selection.
	cmd := climigs.CreateMigRunCommand{
		Client:   httpClient,
		BaseURL:  base,
		MigRef:   domaintypes.MigRef(migID),
		RepoURLs: repoURLs,
		Failed:   *failed,
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	// Print run_id on success.
	_, _ = fmt.Fprintln(stderr, result.RunID)

	// Follow mode: display job graph until completion.
	if *followFlag {
		return followMigRunProject(ctx, base, httpClient, result.RunID, *capDuration, *cancelOnCap, *maxRetries, stderr)
	}

	return nil
}

// followMigRunProject displays the job graph until run completion.
func followMigRunProject(ctx context.Context, baseURL *url.URL, client *http.Client, runID domaintypes.RunID, capDuration time.Duration, cancelOnCap bool, maxRetries int, stderr io.Writer) error {

	followCtx := ctx
	var cancel context.CancelFunc
	if capDuration > 0 {
		followCtx, cancel = context.WithTimeout(ctx, capDuration)
		defer cancel()
	}

	final, err := runs.FollowRunCommand{
		Client:     cloneForStream(client),
		BaseURL:    baseURL,
		RunID:      runID,
		Output:     stderr,
		MaxRetries: maxRetries,
	}.Run(followCtx)
	if err != nil {
		// Handle timeout with optional cancellation.
		if capDuration > 0 && followCtx.Err() == context.DeadlineExceeded {
			if cancelOnCap {
				_, _ = fmt.Fprintln(stderr, "Follow timed out; requesting run cancellation...")
				_ = runs.CancelCommand{
					BaseURL: baseURL,
					Client:  client,
					RunID:   runID,
					Reason:  "cap exceeded",
					Output:  stderr,
				}.Run(context.Background())
			} else {
				_, _ = fmt.Fprintf(stderr, "Follow capped after %s; run %s continues running in the background.\n", capDuration.String(), runID)
			}
			return nil
		}
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Final state: %s\n", strings.ToLower(string(final)))
	if final != migsapi.RunStateSucceeded {
		return fmt.Errorf("mig run ended in %s", strings.ToLower(string(final)))
	}

	return nil
}

// printMigRunProjectUsage prints usage for the mig run <mig> command.
func printMigRunProjectUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig run <mig-id|name> [--repo <url>...] [--failed] [--follow]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Creates a run from a mig project and immediately starts execution.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Repo selection (mutually exclusive):")
	_, _ = fmt.Fprintln(w, "  --repo <url>    Explicit repo URL(s) to run (repeatable)")
	_, _ = fmt.Fprintln(w, "  --failed        Run repos with last terminal state Fail")
	_, _ = fmt.Fprintln(w, "  (omitted)       Run all repos in the mig repo set")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Follow mode:")
	_, _ = fmt.Fprintln(w, "  --follow            Follow run until completion (shows job graph)")
	_, _ = fmt.Fprintln(w, "  --cap <duration>    Optional time cap for --follow (e.g., 30m, 1h)")
	_, _ = fmt.Fprintln(w, "  --cancel-on-cap     Cancel run if cap exceeded")
	_, _ = fmt.Fprintln(w, "  --max-retries <n>   Max SSE reconnect attempts (default: 5)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy mig run my-mig                                    # Run all repos")
	_, _ = fmt.Fprintln(w, "  ploy mig run my-mig --failed                           # Retry failed repos")
	_, _ = fmt.Fprintln(w, "  ploy mig run my-mig --repo https://a.git --repo https://b.git  # Specific repos")
	_, _ = fmt.Fprintln(w, "  ploy mig run my-mig --follow                           # Follow until completion")
}
