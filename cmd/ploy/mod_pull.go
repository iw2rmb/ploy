// mod_pull.go implements the `ploy mod pull` subcommand for pulling Mods diffs
// into the current git worktree based on mod project context.
//
// Command structure:
//
//	ploy mod pull [--origin <remote>] [--dry-run] [--last-failed | --last-succeeded] [<mod-name|id>]
//
// Behavior:
//   - If <mod-name|id> is provided, use it to select the mod.
//   - If <mod-name|id> is omitted, infer the mod from the current repo:
//     Call GET /v1/mods?repo_url=<current_repo_url>&archived=false
//     If exactly one mod matches: use it.
//     If multiple mods match: error with list of matching mods.
//   - Uses POST /v1/mods/{mod_id}/pull to resolve the run and repo.
//   - Pulls diffs via GET /v1/runs/{run_id}/repos/{repo_id}/diffs.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/vcs"
)

// handleModPull implements `ploy mod pull [--origin <remote>] [--dry-run] [--last-failed] [<mod-id|name>]`.
// Parses CLI flags, validates arguments, enforces git worktree preconditions, and resolves the mod + run.
//
// The command:
//   - Must be executed from inside a git repository
//   - Derives repo identity from git remote URL (origin by default)
//   - Optionally accepts a mod ID/name; if omitted, infers from current repo
//   - Uses POST /v1/mods/{mod_id}/pull to resolve run execution identifiers
//   - Pulls diffs via GET /v1/runs/{run_id}/repos/{repo_id}/diffs
//
// Arguments:
//   - args: remaining arguments after "pull" has been stripped
//   - stderr: writer for user-facing output and error messages
//
// Returns an error if argument parsing fails, preconditions are not met,
// mod/run resolution fails, or git/API operations fail.
func handleModPull(args []string, stderr io.Writer) error {
	// Create a flag set for the pull subcommand.
	fs := flag.NewFlagSet("mod pull", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Suppress default flag error output; we print custom usage.

	// Define flags:
	// --origin: git remote to match (default "origin")
	// --dry-run: validate and print actions without mutating the repo
	// --last-failed: select newest run with status=Fail (default: last-succeeded)
	// --last-succeeded: select newest run with status=Success (this is the default)
	origin := fs.String("origin", "origin", "git remote to match (default origin)")
	dryRun := fs.Bool("dry-run", false, "validate and print actions without mutating the repo")
	lastFailed := fs.Bool("last-failed", false, "select the latest failed run (default: last succeeded)")
	lastSucceeded := fs.Bool("last-succeeded", false, "select the latest succeeded run (default)")

	// Parse the flags from the provided arguments.
	if err := fs.Parse(args); err != nil {
		printModPullUsage(stderr)
		return err
	}

	// Validate flag mutual exclusion.
	if *lastFailed && *lastSucceeded {
		printModPullUsage(stderr)
		return errors.New("--last-failed and --last-succeeded are mutually exclusive")
	}

	// Determine pull mode.
	pullMode := mods.PullModeLastSucceeded
	if *lastFailed {
		pullMode = mods.PullModeLastFailed
	}

	// After flag parsing, remaining args may contain the optional mod identifier.
	rest := fs.Args()
	var modIDOrName string
	if len(rest) > 0 && strings.TrimSpace(rest[0]) != "" {
		modIDOrName = strings.TrimSpace(rest[0])
	}

	// Validate that no extra positional arguments were provided.
	if len(rest) > 1 {
		printModPullUsage(stderr)
		return fmt.Errorf("unexpected argument: %s", rest[1])
	}

	// Create a context with a reasonable timeout for git and API operations.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Step 1: Verify we are inside a git worktree.
	if err := ensureInsideGitWorktree(ctx); err != nil {
		return fmt.Errorf("mod pull: %w", err)
	}

	// Step 2: Verify the working tree is clean.
	if err := ensureCleanWorkingTree(ctx); err != nil {
		return fmt.Errorf("mod pull: %w", err)
	}

	// Step 3: Resolve the git remote URL for the specified origin.
	rawOriginURL, err := resolveGitRemoteURL(ctx, *origin)
	if err != nil {
		return fmt.Errorf("mod pull: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "mod pull: resolved origin %q → %s\n", *origin, vcs.NormalizeRepoURLSchemless(rawOriginURL))

	// Step 4: Get control plane connection.
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return fmt.Errorf("mod pull: %w", err)
	}

	// Step 5: Resolve the mod ID.
	// If modIDOrName is provided, use it directly.
	// Otherwise, infer from the current repo by querying mods that include this repo.
	modID := modIDOrName
	if modID == "" {
		inferredModID, err := inferModFromRepo(ctx, httpClient, base, rawOriginURL, stderr)
		if err != nil {
			return fmt.Errorf("mod pull: %w", err)
		}
		modID = inferredModID
	}

	_, _ = fmt.Fprintf(stderr, "mod pull: using mod %q\n", modID)

	// Step 6: Resolve repo execution via POST /v1/mods/{mod_id}/pull.
	pullCmd := mods.ModPullCommand{
		Client:  httpClient,
		BaseURL: base,
		ModRef:  domaintypes.ModRef(modID),
		RepoURL: rawOriginURL,
		Mode:    pullMode,
	}
	resolution, err := pullCmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("mod pull: resolve repo: %w", err)
	}

	_, _ = fmt.Fprintf(stderr, "mod pull: resolved run %s (mode: %s)\n", resolution.RunID.String(), pullMode)
	_, _ = fmt.Fprintf(stderr, "  repo ID: %s\n", resolution.RepoID.String())
	_, _ = fmt.Fprintf(stderr, "  target ref: %s\n", resolution.RepoTargetRef.String())

	// Step 7: Fetch repo details to get base_ref.
	repoDetails, err := fetchRunRepoDetails(ctx, httpClient, base, resolution.RunID, resolution.RepoID)
	if err != nil {
		return fmt.Errorf("mod pull: fetch repo details: %w", err)
	}

	baseRef := strings.TrimSpace(repoDetails.BaseRef)
	if baseRef == "" {
		return errors.New("mod pull: base_ref is not available for this run")
	}
	_, _ = fmt.Fprintf(stderr, "  base ref: %s\n", baseRef)

	targetRef := strings.TrimSpace(resolution.RepoTargetRef.String())
	if targetRef == "" {
		return errors.New("mod pull: target_ref is not available for this run")
	}

	// Step 8: Fetch the base ref from the origin remote.
	if err := fetchRef(ctx, *origin, baseRef, stderr, *dryRun); err != nil {
		return fmt.Errorf("mod pull: %w", err)
	}

	baseCommit := ""
	if !*dryRun {
		commit, err := resolveFetchHeadSHA(ctx)
		if err != nil {
			return fmt.Errorf("mod pull: %w", err)
		}
		baseCommit = commit
		_, _ = fmt.Fprintf(stderr, "  base commit: %s\n", baseCommit)
	}

	// Step 9: Check for branch collisions.
	if err := checkBranchCollision(ctx, *origin, targetRef, stderr); err != nil {
		return fmt.Errorf("mod pull: %w", err)
	}

	// Step 10: Fetch diffs for this repo execution.
	diffs, err := listRunRepoDiffs(ctx, httpClient, base, resolution.RunID, resolution.RepoID)
	if err != nil {
		return fmt.Errorf("mod pull: list diffs: %w", err)
	}
	_, _ = fmt.Fprintf(stderr, "  diffs to apply: %d\n", len(diffs))

	// Step 11: Handle --dry-run mode.
	if *dryRun {
		_, _ = fmt.Fprintf(stderr, "\nWould create branch %q at %q (origin %q) and apply %d Mods diff(s)\n",
			targetRef, baseRef, *origin, len(diffs))
		for i, diff := range diffs {
			_, _ = fmt.Fprintf(stderr, "  diff %d: %s (%d bytes gzipped)\n",
				i+1, diff.ID, diff.Size)
		}
		return nil
	}

	// Step 12: Create the target branch at the fetched base commit.
	if err := createAndCheckoutBranch(ctx, targetRef, baseCommit, stderr); err != nil {
		return fmt.Errorf("mod pull: %w", err)
	}

	// Step 13: Download and apply all diffs.
	appliedCount, err := downloadAndApplyDiffs(ctx, resolution.RunID, resolution.RepoID, diffs, stderr)
	if err != nil {
		return fmt.Errorf("mod pull: %w", err)
	}

	// Success message.
	_, _ = fmt.Fprintf(stderr, "\nApplied %d Mods diff(s) from run %s to branch %q (origin %q)\n",
		appliedCount, resolution.RunID, targetRef, *origin)
	_, _ = fmt.Fprintf(stderr, "  mod: %s\n", modID)

	return nil
}

// inferModFromRepo attempts to infer the mod ID from the current repo.
// It queries GET /v1/mods?repo_url=<url>&archived=false to find mods that include this repo.
//
// Returns:
//   - If exactly one non-archived mod matches: return that mod's ID.
//   - If multiple mods match: return error with list of matching mods.
//   - If no mods match: return error.
func inferModFromRepo(ctx context.Context, httpClient *http.Client, baseURL *url.URL, repoURL string, stderr io.Writer) (string, error) {
	if baseURL == nil {
		return "", errors.New("base url required")
	}

	endpoint := baseURL.JoinPath("v1", "mods")
	q := endpoint.Query()
	q.Set("repo_url", repoURL)
	q.Set("archived", "false")
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Parse the response.
	var result struct {
		Mods []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"mods"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	// Handle results based on number of matches.
	switch len(result.Mods) {
	case 0:
		return "", fmt.Errorf("no mods found that include repo %s", repoURL)
	case 1:
		mod := result.Mods[0]
		_, _ = fmt.Fprintf(stderr, "mod pull: inferred mod %q (%s) from repo\n", mod.Name, mod.ID)
		return mod.ID, nil
	default:
		// Multiple mods match — error with list.
		_, _ = fmt.Fprintf(stderr, "mod pull: multiple mods include this repo:\n")
		for _, mod := range result.Mods {
			_, _ = fmt.Fprintf(stderr, "  - %s (%s)\n", mod.Name, mod.ID)
		}
		return "", errors.New("multiple mods match; specify a mod ID or name explicitly")
	}
}

// printModPullUsage renders the usage help for `ploy mod pull`.
func printModPullUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod pull [--origin <remote>] [--dry-run] [--last-failed | --last-succeeded] [<mod-id|name>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Pulls Mods diffs from a mod's latest run into the current git repository.")
	_, _ = fmt.Fprintln(w, "Creates a new branch at the run's base commit and applies stored diffs.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Use this command when you want to pull from a mod's latest run.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Arguments:")
	_, _ = fmt.Fprintln(w, "  [<mod-id|name>]  Optional mod ID or name (inferred from repo if omitted)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --origin <remote>  Git remote to match (default: origin)")
	_, _ = fmt.Fprintln(w, "  --dry-run          Validate and print actions without mutating the repo")
	_, _ = fmt.Fprintln(w, "  --last-failed      Select the latest failed run")
	_, _ = fmt.Fprintln(w, "  --last-succeeded   Select the latest succeeded run (default)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ploy mod pull")
	_, _ = fmt.Fprintln(w, "  ploy mod pull my-mod")
	_, _ = fmt.Fprintln(w, "  ploy mod pull --last-failed my-mod")
	_, _ = fmt.Fprintln(w, "  ploy mod pull --dry-run")
	_, _ = fmt.Fprintln(w, "  ploy mod pull --origin upstream my-mod")
}
