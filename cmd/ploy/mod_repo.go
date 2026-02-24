// mod_repo.go implements the 'ploy mod repo' command handler.
//
// This command manages a mod's repo set:
// - ploy mod repo add <mod-id|name> --repo <repo-url> --base-ref <ref> --target-ref <ref>
// - ploy mod repo list <mod-id|name>
// - ploy mod repo remove <mod-id|name> --repo-id <repo_id>
// - ploy mod repo import <mod-id|name> --file <path>
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleModRepo routes mod repo subcommands.
func handleModRepo(args []string, stderr io.Writer) error {
	// Handle help flag or empty args.
	if wantsHelp(args) || len(args) == 0 {
		printModRepoUsage(stderr)
		if len(args) == 0 {
			return fmt.Errorf("mod repo subcommand required")
		}
		return nil
	}

	switch args[0] {
	case "add":
		return handleModRepoAdd(args[1:], stderr)
	case "list":
		return handleModRepoList(args[1:], stderr)
	case "remove":
		return handleModRepoRemove(args[1:], stderr)
	case "import":
		return handleModRepoImport(args[1:], stderr)
	default:
		printModRepoUsage(stderr)
		return fmt.Errorf("unknown mod repo subcommand %q", args[0])
	}
}

// handleModRepoAdd implements 'ploy mod repo add <mod-id|name> --repo <url> --base-ref <ref> --target-ref <ref>'.
func handleModRepoAdd(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModRepoAddUsage(stderr)
		return nil
	}

	// Parse flags.
	fs := flag.NewFlagSet("mod repo add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	repoURL := fs.String("repo", "", "Git repository URL (required)")
	baseRef := fs.String("base-ref", "", "Base git ref (required)")
	targetRef := fs.String("target-ref", "", "Target git ref (required)")

	// First positional arg is mod ID/name.
	if len(args) == 0 {
		printModRepoAddUsage(stderr)
		return fmt.Errorf("mod id/name required")
	}
	modRef := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		printModRepoAddUsage(stderr)
		return err
	}

	// Validate required flags.
	if *repoURL == "" {
		printModRepoAddUsage(stderr)
		return fmt.Errorf("--repo is required")
	}
	if *baseRef == "" {
		printModRepoAddUsage(stderr)
		return fmt.Errorf("--base-ref is required")
	}
	if *targetRef == "" {
		printModRepoAddUsage(stderr)
		return fmt.Errorf("--target-ref is required")
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mod reference to ID.
	resolveCmd := mods.ResolveModByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modRef),
	}
	modID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mod repo add command.
	cmd := mods.AddModRepoCommand{
		Client:    httpClient,
		BaseURL:   base,
		MigRef:    domaintypes.MigRef(modID),
		RepoURL:   *repoURL,
		BaseRef:   *baseRef,
		TargetRef: *targetRef,
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Repo added: %s (url: %s)\n", result.ID.String(), domaintypes.NormalizeRepoURLSchemless(result.RepoURL))
	return nil
}

// handleModRepoList implements 'ploy mod repo list <mod-id|name>'.
func handleModRepoList(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModRepoListUsage(stderr)
		return nil
	}

	// Require mod ID/name as positional arg.
	if len(args) == 0 {
		printModRepoListUsage(stderr)
		return fmt.Errorf("mod id/name required")
	}
	modRef := args[0]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mod reference to ID.
	resolveCmd := mods.ResolveModByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modRef),
	}
	modID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mod repo list command.
	cmd := mods.ListModReposCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modID),
	}

	results, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		_, _ = fmt.Fprintln(stderr, "No repos found.")
		return nil
	}

	// Print results in tabular format.
	w := tabwriter.NewWriter(stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tREPO_URL\tBASE_REF\tTARGET_REF\tADDED_AT")
	for _, repo := range results {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			repo.ID.String(),
			domaintypes.NormalizeRepoURLSchemless(repo.RepoURL),
			repo.BaseRef.String(),
			repo.TargetRef.String(),
			repo.CreatedAt.Format(time.RFC3339),
		)
	}
	_ = w.Flush()

	return nil
}

// handleModRepoRemove implements 'ploy mod repo remove <mod-id|name> --repo-id <repo_id>'.
func handleModRepoRemove(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModRepoRemoveUsage(stderr)
		return nil
	}

	// Parse flags.
	fs := flag.NewFlagSet("mod repo remove", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	repoID := fs.String("repo-id", "", "Repo ID to remove (required)")

	// First positional arg is mod ID/name.
	if len(args) == 0 {
		printModRepoRemoveUsage(stderr)
		return fmt.Errorf("mod id/name required")
	}
	modRef := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		printModRepoRemoveUsage(stderr)
		return err
	}

	// Validate required flags.
	if *repoID == "" {
		printModRepoRemoveUsage(stderr)
		return fmt.Errorf("--repo-id is required")
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mod reference to ID.
	resolveCmd := mods.ResolveModByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modRef),
	}
	modID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mod repo remove command.
	cmd := mods.RemoveModRepoCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modID),
		RepoID:  domaintypes.MigRepoID(*repoID),
	}

	if err := cmd.Run(ctx); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Repo deleted: %s\n", *repoID)
	return nil
}

// handleModRepoImport implements 'ploy mod repo import <mod-id|name> --file <path>'.
func handleModRepoImport(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printModRepoImportUsage(stderr)
		return nil
	}

	// Parse flags.
	fs := flag.NewFlagSet("mod repo import", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	filePath := fs.String("file", "", "Path to CSV file (required)")

	// First positional arg is mod ID/name.
	if len(args) == 0 {
		printModRepoImportUsage(stderr)
		return fmt.Errorf("mod id/name required")
	}
	modRef := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		printModRepoImportUsage(stderr)
		return err
	}

	// Validate required flags.
	if *filePath == "" {
		printModRepoImportUsage(stderr)
		return fmt.Errorf("--file is required")
	}

	// Read CSV file.
	csvData, err := os.ReadFile(*filePath)
	if err != nil {
		return fmt.Errorf("read csv file: %w", err)
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mod reference to ID.
	resolveCmd := mods.ResolveModByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modRef),
	}
	modID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mod repo import command.
	cmd := mods.ImportModReposCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(modID),
		CSVData: csvData,
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	// Print import results.
	_, _ = fmt.Fprintf(stderr, "Import complete: %d created, %d updated, %d failed\n",
		result.Created, result.Updated, result.Failed)

	// Print errors if any.
	for _, e := range result.Errors {
		_, _ = fmt.Fprintf(stderr, "  Line %d: %s\n", e.Line, e.Message)
	}

	return nil
}

// printModRepoUsage prints usage for the mod repo command.
func printModRepoUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod repo <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  add <mod> --repo <url> --base-ref <ref> --target-ref <ref>  Add a repo to the mod")
	_, _ = fmt.Fprintln(w, "  list <mod>                                                   List repos in the mod")
	_, _ = fmt.Fprintln(w, "  remove <mod> --repo-id <id>                                  Remove a repo from the mod")
	_, _ = fmt.Fprintln(w, "  import <mod> --file <path>                                   Import repos from CSV")
}

// printModRepoAddUsage prints usage for the mod repo add command.
func printModRepoAddUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod repo add <mod-id|name> --repo <url> --base-ref <ref> --target-ref <ref>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Adds a repo to the mod's repo set.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --repo <url>        Git repository URL (required)")
	_, _ = fmt.Fprintln(w, "  --base-ref <ref>    Base git ref (required)")
	_, _ = fmt.Fprintln(w, "  --target-ref <ref>  Target git ref (required)")
}

// printModRepoListUsage prints usage for the mod repo list command.
func printModRepoListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod repo list <mod-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Lists repos in the mod's repo set.")
}

// printModRepoRemoveUsage prints usage for the mod repo remove command.
func printModRepoRemoveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod repo remove <mod-id|name> --repo-id <id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Removes a repo from the mod's repo set.")
	_, _ = fmt.Fprintln(w, "Refuses if the repo has historical executions.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --repo-id <id>  Repo ID to remove (required)")
}

// printModRepoImportUsage prints usage for the mod repo import command.
func printModRepoImportUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mod repo import <mod-id|name> --file <path>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Imports repos from CSV file. CSV format: repo_url,base_ref,target_ref")
	_, _ = fmt.Fprintln(w, "Upserts by repo_url; updates refs for existing repos.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --file <path>  Path to CSV file (required)")
}
