// mod_run_project.go implements the 'ploy mod run <mod-id|name>' command handler for mod projects.
//
// This command creates a run from a mod project:
// - ploy mod run <mod-id|name> [--repo <repo-url> ...] [--failed]
// - Resolves <mod-id|name> to a mod (supports both ID and name).
// - Refuses when the mod is archived.
// - Uses mods.spec_id as the run's spec_id.
// - Selects repos:
//   - --repo ... → explicit repos (by repo_url identity within the mod)
//   - --failed → repos with last terminal state Fail
//   - omitted → all repos in the mod repo set
//
// - Creates a mod-scoped run via POST /v1/mods/{mod_id}/runs and immediately starts execution.
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

	"github.com/iw2rmb/ploy/internal/cli/follow"
	climods "github.com/iw2rmb/ploy/internal/cli/mods"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// handleModRunProject implements 'ploy mod run <mod-id|name> [--repo <url>...] [--failed]'.
// This is the v1 mod project run command with repo selection.
func handleModRunProject(args []string, stderr io.Writer) error {
	// Handle help flag (support `ploy mod run <mod> --help`).
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printModRunProjectUsage(stderr)
			return nil
		}
	}

	// First positional arg is mod ID/name.
	if len(args) == 0 {
		printModRunProjectUsage(stderr)
		return fmt.Errorf("mod id/name required")
	}
	modRef := args[0]

	// Parse remaining flags.
	fs := flag.NewFlagSet("mod run project", flag.ContinueOnError)
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
		printModRunProjectUsage(stderr)
		return err
	}

	// Validate mutual exclusion: --failed and --repo cannot both be specified.
	if *failed && len(repoURLs) > 0 {
		printModRunProjectUsage(stderr)
		return fmt.Errorf("--failed and --repo are mutually exclusive")
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mod reference to ID (supports both name and ID).
	resolveCmd := climods.ResolveModByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modRef),
	}
	modID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mod run command with repo selection.
	cmd := climods.CreateMigRunCommand{
		Client:   httpClient,
		BaseURL:  base,
		MigRef:   domaintypes.MigRef(modID),
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
		return followModRunProject(ctx, base, httpClient, result.RunID, *capDuration, *cancelOnCap, *maxRetries, stderr)
	}

	return nil
}

// followModRunProject displays the job graph until run completion.
func followModRunProject(ctx context.Context, baseURL *url.URL, client *http.Client, runID domaintypes.RunID, capDuration time.Duration, cancelOnCap bool, maxRetries int, stderr io.Writer) error {

	followCtx := ctx
	var cancel context.CancelFunc
	if capDuration > 0 {
		followCtx, cancel = context.WithTimeout(ctx, capDuration)
		defer cancel()
	}

	engine := follow.NewEngine(cloneForStream(client), baseURL, runID, follow.Config{
		MaxRetries: maxRetries,
		Output:     stderr,
	})

	final, err := engine.Run(followCtx)
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
	if final != modsapi.RunStateSucceeded {
		return fmt.Errorf("mod run ended in %s", strings.ToLower(string(final)))
	}

	return nil
}

// printModRunProjectUsage prints usage for the mod run <mod> command.
func printModRunProjectUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run <mod-id|name> [--repo <url>...] [--failed] [--follow]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Creates a run from a mod project and immediately starts execution.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Repo selection (mutually exclusive):")
	_, _ = fmt.Fprintln(w, "  --repo <url>    Explicit repo URL(s) to run (repeatable)")
	_, _ = fmt.Fprintln(w, "  --failed        Run repos with last terminal state Fail")
	_, _ = fmt.Fprintln(w, "  (omitted)       Run all repos in the mod repo set")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Follow mode:")
	_, _ = fmt.Fprintln(w, "  --follow            Follow run until completion (shows job graph)")
	_, _ = fmt.Fprintln(w, "  --cap <duration>    Optional time cap for --follow (e.g., 30m, 1h)")
	_, _ = fmt.Fprintln(w, "  --cancel-on-cap     Cancel run if cap exceeded")
	_, _ = fmt.Fprintln(w, "  --max-retries <n>   Max SSE reconnect attempts (default: 5)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy mod run my-mod                                    # Run all repos")
	_, _ = fmt.Fprintln(w, "  ploy mod run my-mod --failed                           # Retry failed repos")
	_, _ = fmt.Fprintln(w, "  ploy mod run my-mod --repo https://a.git --repo https://b.git  # Specific repos")
	_, _ = fmt.Fprintln(w, "  ploy mod run my-mod --follow                           # Follow until completion")
}
