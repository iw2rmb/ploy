// mod_run_project.go implements the 'ploy mod run <mod-id|name>' command handler for mod projects.
//
// Per roadmap/v1/cli.md:102-119, this command creates a run from a mod project:
// - ploy mod run <mod-id|name> [--repo <repo-url> ...] [--failed]
// - Resolves <mod-id|name> to a mod.
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

	climods "github.com/iw2rmb/ploy/internal/cli/mods"
)

// handleModRunProject implements 'ploy mod run <mod-id|name> [--repo <url>...] [--failed]'.
// This is the v1 mod project run command with repo selection.
func handleModRunProject(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModRunProjectUsage(stderr)
		return nil
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

	// Repo selection flags per roadmap/v1/cli.md:113-115.
	// --repo can be repeated for explicit repo selection.
	// --failed selects repos with last terminal state Fail.
	var repoURLs stringSlice
	fs.Var(&repoURLs, "repo", "Explicit repo URL(s) to run (repeatable)")
	failed := fs.Bool("failed", false, "Run repos with last terminal state Fail")

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

	// Resolve mod reference to ID (supports name/ID resolution per roadmap/v1/cli.md:169-170).
	resolveCmd := climods.ResolveModByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		ModRef:  modRef,
	}
	modID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mod run command with repo selection.
	cmd := climods.CreateModRunCommand{
		Client:   httpClient,
		BaseURL:  base,
		ModID:    modID,
		RepoURLs: repoURLs,
		Failed:   *failed,
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	// Print run_id per roadmap/v1/cli.md:117-119.
	_, _ = fmt.Fprintf(stderr, "Run created: %s\n", result.RunID)
	return nil
}

// printModRunProjectUsage prints usage for the mod run <mod> command.
func printModRunProjectUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod run <mod-id|name> [--repo <url>...] [--failed]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Creates a run from a mod project and immediately starts execution.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Repo selection (mutually exclusive):")
	_, _ = fmt.Fprintln(w, "  --repo <url>    Explicit repo URL(s) to run (repeatable)")
	_, _ = fmt.Fprintln(w, "  --failed        Run repos with last terminal state Fail")
	_, _ = fmt.Fprintln(w, "  (omitted)       Run all repos in the mod repo set")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy mod run my-mod                                    # Run all repos")
	_, _ = fmt.Fprintln(w, "  ploy mod run my-mod --failed                           # Retry failed repos")
	_, _ = fmt.Fprintln(w, "  ploy mod run my-mod --repo https://a.git --repo https://b.git  # Specific repos")
}
