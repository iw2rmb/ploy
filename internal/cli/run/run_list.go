// run_list.go implements run listing CLI commands.
//
// The command delegates to the internal/cli/migs list client because that client
// still owns the shared /v1/runs pagination call.
//
// Command structure:
//   - ploy run ls [--limit N] [--offset N]
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type ListOptions struct {
	Limit        int
	Offset       int
	RepoSelector string
	Output       io.Writer
}

// RunList implements `ploy run ls [selector] [--limit N] [--offset N]`.
// It lists runs with pagination, optionally filtered by a resolved repo URL.
func RunList(ctx context.Context, opts ListOptions) error {
	out := opts.Output
	if out == nil {
		out = io.Discard
	}
	// Validate pagination parameters.
	if opts.Limit < 1 || opts.Limit > 100 {
		return errors.New("limit must be between 1 and 100")
	}
	if opts.Offset < 0 {
		return errors.New("offset must be non-negative")
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	repoURL := ""
	if opts.RepoSelector != "" {
		repo, err := resolveSourceRepo(ctx, base, httpClient, opts.RepoSelector)
		if err != nil {
			return err
		}
		repoURL = repo.RepoURL
	}

	// Execute the list command using the shared runs client.
	cmd := migs.ListRunsCommand{
		Client:  httpClient,
		BaseURL: base,
		Limit:   int32(opts.Limit),
		Offset:  int32(opts.Offset),
		RepoURL: repoURL,
	}

	runs, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	if len(runs) == 0 {
		_, _ = fmt.Fprintln(out, "No runs found.")
		return nil
	}

	// Print results in tabular format.
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tMIG\tSPEC\tREPO")
	for _, b := range runs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			b.ID, b.Status, b.MigID, b.SpecID, formatRunListRepo(b.RepoURL, b.SourceCommitSHA))
	}
	_ = tw.Flush()
	return nil
}

func formatRunListRepo(repoURL, sourceCommitSHA string) string {
	repo := formatRepoNamespace(repoURL)
	sha := strings.TrimSpace(sourceCommitSHA)
	if len(sha) > 8 {
		sha = sha[:8]
	}
	if repo == "" {
		return "-"
	}
	if sha == "" {
		return repo
	}
	return repo + ":" + sha
}

func formatRepoNamespace(repoURL string) string {
	display := domaintypes.NormalizeRepoURLSchemless(repoURL)
	display = strings.Trim(display, "/")
	if display == "" {
		return ""
	}
	parts := strings.Split(display, "/")
	if len(parts) <= 2 {
		return display
	}
	return strings.Join(parts[1:], "/")
}
