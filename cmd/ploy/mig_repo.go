// mig_repo.go implements the 'ploy mig repo' command handler.
//
// This command manages a mig's repo set:
// - ploy mig repo add <mig-id|name> --repo <repo-url> --base-ref <ref> --target-ref <ref>
// - ploy mig repo list <mig-id|name>
// - ploy mig repo remove <mig-id|name> --repo-id <repo_id>
// - ploy mig repo import <mig-id|name> --file <path>
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// handleMigRepo routes mig repo subcommands.
func handleMigRepo(args []string, stderr io.Writer) error {
	// Handle help flag or empty args.
	if wantsHelp(args) || len(args) == 0 {
		printMigRepoUsage(stderr)
		if len(args) == 0 {
			return fmt.Errorf("mig repo subcommand required")
		}
		return nil
	}

	switch args[0] {
	case "add":
		return handleMigRepoAdd(args[1:], stderr)
	case "list":
		return handleMigRepoList(args[1:], stderr)
	case "remove":
		return handleMigRepoRemove(args[1:], stderr)
	case "import":
		return handleMigRepoImport(args[1:], stderr)
	default:
		printMigRepoUsage(stderr)
		return fmt.Errorf("unknown mig repo subcommand %q", args[0])
	}
}

// handleMigRepoAdd implements 'ploy mig repo add <mig-id|name> --repo <url> --base-ref <ref> --target-ref <ref>'.
func handleMigRepoAdd(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigRepoAddUsage(stderr)
		return nil
	}

	// Parse flags.
	fs := flag.NewFlagSet("mig repo add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	repoURL := fs.String("repo", "", "Git repository URL (required)")
	baseRef := fs.String("base-ref", "", "Base git ref (required)")
	targetRef := fs.String("target-ref", "", "Target git ref (required)")

	// First positional arg is mig ID/name.
	if len(args) == 0 {
		printMigRepoAddUsage(stderr)
		return fmt.Errorf("mig id/name required")
	}
	migRef := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		printMigRepoAddUsage(stderr)
		return err
	}

	// Validate required flags.
	if *repoURL == "" {
		printMigRepoAddUsage(stderr)
		return fmt.Errorf("--repo is required")
	}
	if *baseRef == "" {
		printMigRepoAddUsage(stderr)
		return fmt.Errorf("--base-ref is required")
	}
	if *targetRef == "" {
		printMigRepoAddUsage(stderr)
		return fmt.Errorf("--target-ref is required")
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mig reference to ID.
	resolveCmd := migs.ResolveMigByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migRef),
	}
	migID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mig repo add command.
	cmd := migs.AddMigRepoCommand{
		Client:    httpClient,
		BaseURL:   base,
		MigRef:    domaintypes.MigRef(migID),
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

// handleMigRepoList implements 'ploy mig repo list <mig-id|name>'.
func handleMigRepoList(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigRepoListUsage(stderr)
		return nil
	}

	// Require mig ID/name as positional arg.
	if len(args) == 0 {
		printMigRepoListUsage(stderr)
		return fmt.Errorf("mig id/name required")
	}
	migRef := args[0]

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mig reference to ID.
	resolveCmd := migs.ResolveMigByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migRef),
	}
	migID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mig repo list command.
	cmd := migs.ListMigReposCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migID),
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
			repo.BaseRef,
			repo.TargetRef,
			repo.CreatedAt.Format(time.RFC3339),
		)
	}
	_ = w.Flush()

	return nil
}

// handleMigRepoRemove implements 'ploy mig repo remove <mig-id|name> --repo-id <repo_id>'.
func handleMigRepoRemove(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigRepoRemoveUsage(stderr)
		return nil
	}

	// Parse flags.
	fs := flag.NewFlagSet("mig repo remove", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	repoID := fs.String("repo-id", "", "Repo ID to remove (required)")

	// First positional arg is mig ID/name.
	if len(args) == 0 {
		printMigRepoRemoveUsage(stderr)
		return fmt.Errorf("mig id/name required")
	}
	migRef := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		printMigRepoRemoveUsage(stderr)
		return err
	}

	// Validate required flags.
	if *repoID == "" {
		printMigRepoRemoveUsage(stderr)
		return fmt.Errorf("--repo-id is required")
	}

	// Resolve control plane connection.
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	// Resolve mig reference to ID.
	resolveCmd := migs.ResolveMigByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migRef),
	}
	migID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mig repo remove command.
	cmd := migs.RemoveMigRepoCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migID),
		RepoID:  domaintypes.MigRepoID(*repoID),
	}

	if err := cmd.Run(ctx); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stderr, "Repo deleted: %s\n", *repoID)
	return nil
}

// handleMigRepoImport implements 'ploy mig repo import <mig-id|name> --file <path>'.
func handleMigRepoImport(args []string, stderr io.Writer) error {
	// Handle help flag.
	if wantsHelp(args) {
		printMigRepoImportUsage(stderr)
		return nil
	}

	// Parse flags.
	fs := flag.NewFlagSet("mig repo import", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	filePath := fs.String("file", "", "Path to CSV file (required)")

	// First positional arg is mig ID/name.
	if len(args) == 0 {
		printMigRepoImportUsage(stderr)
		return fmt.Errorf("mig id/name required")
	}
	migRef := args[0]

	if err := fs.Parse(args[1:]); err != nil {
		printMigRepoImportUsage(stderr)
		return err
	}

	// Validate required flags.
	if *filePath == "" {
		printMigRepoImportUsage(stderr)
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

	// Resolve mig reference to ID.
	resolveCmd := migs.ResolveMigByNameCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migRef),
	}
	migID, err := resolveCmd.Run(ctx)
	if err != nil {
		return err
	}

	// Execute mig repo import command.
	cmd := migs.ImportMigReposCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migID),
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

// printMigRepoUsage prints usage for the mig repo command.
func printMigRepoUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig repo <command>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands:")
	_, _ = fmt.Fprintln(w, "  add <mig> --repo <url> --base-ref <ref> --target-ref <ref>  Add a repo to the mig")
	_, _ = fmt.Fprintln(w, "  list <mig>                                                   List repos in the mig")
	_, _ = fmt.Fprintln(w, "  remove <mig> --repo-id <id>                                  Remove a repo from the mig")
	_, _ = fmt.Fprintln(w, "  import <mig> --file <path>                                   Import repos from CSV")
}

// printMigRepoAddUsage prints usage for the mig repo add command.
func printMigRepoAddUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig repo add <mig-id|name> --repo <url> --base-ref <ref> --target-ref <ref>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Adds a repo to the mig's repo set.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --repo <url>        Git repository URL (required)")
	_, _ = fmt.Fprintln(w, "  --base-ref <ref>    Base git ref (required)")
	_, _ = fmt.Fprintln(w, "  --target-ref <ref>  Target git ref (required)")
}

// printMigRepoListUsage prints usage for the mig repo list command.
func printMigRepoListUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig repo list <mig-id|name>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Lists repos in the mig's repo set.")
}

// printMigRepoRemoveUsage prints usage for the mig repo remove command.
func printMigRepoRemoveUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig repo remove <mig-id|name> --repo-id <id>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Removes a repo from the mig's repo set.")
	_, _ = fmt.Fprintln(w, "Refuses if the repo has historical executions.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --repo-id <id>  Repo ID to remove (required)")
}

// printMigRepoImportUsage prints usage for the mig repo import command.
func printMigRepoImportUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig repo import <mig-id|name> --file <path>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Imports repos from CSV file. CSV format: repo_url,base_ref,target_ref")
	_, _ = fmt.Fprintln(w, "Upserts by repo_url; updates refs for existing repos.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --file <path>  Path to CSV file (required)")
}
