// run_list.go implements run listing CLI commands.
//
// The command delegates to the shared control-plane list client.
//
// Command structure:
//   - ploy run ls [--limit N] [--offset N]
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/iw2rmb/ploy/internal/cli/common"
	sharedclient "github.com/iw2rmb/ploy/internal/client"
)

type ListOptions struct {
	Limit        int
	Offset       int
	RepoSelector string
	All          bool
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
	cmd := sharedclient.ListRunsCommand{
		Client:  httpClient,
		BaseURL: base,
		Limit:   int32(opts.Limit),
		Offset:  int32(opts.Offset),
		RepoURL: repoURL,
		All:     opts.All,
	}
	if !opts.All {
		cmd.CreatedBy = strings.TrimSpace(os.Getenv("USER"))
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
	_, _ = fmt.Fprintln(tw, "ID\tSTATUS\tSPEC\tREPO")
	for _, b := range runs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			b.ID, b.Status, common.FormatRunListSpec(b), common.FormatRunListRepo(b.RepoURL, b.SourceCommitSHA))
	}
	_ = tw.Flush()
	return nil
}
