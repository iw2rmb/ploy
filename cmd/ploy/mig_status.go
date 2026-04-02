package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func handleMigStatus(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printMigStatusUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("mig status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := parseFlagSet(fs, args, func() { printMigStatusUsage(stderr) }); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printMigStatusUsage(stderr)
		return errors.New("mig id required")
	}
	migID := strings.TrimSpace(rest[0])

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}

	migSummary, err := findMigByID(ctx, httpClient, base, migID)
	if err != nil {
		return err
	}

	repos, err := migs.ListMigReposCommand{
		Client:  httpClient,
		BaseURL: base,
		MigRef:  domaintypes.MigRef(migID),
	}.Run(ctx)
	if err != nil {
		return err
	}

	runs, err := listRunsByMigID(ctx, httpClient, base, migID)
	if err != nil {
		return err
	}

	specID := "-"
	if migSummary.SpecID != nil && !migSummary.SpecID.IsZero() {
		specID = migSummary.SpecID.String()
	}

	_, _ = fmt.Fprintf(stderr, "Mig:   %s  | %s\n", migSummary.ID.String(), migValueOrDash(strings.TrimSpace(migSummary.Name)))
	_, _ = fmt.Fprintf(stderr, "Spec:  %s | Download\n", specID)
	_, _ = fmt.Fprintf(stderr, "Repos: %d\n", len(repos))
	_, _ = fmt.Fprintln(stderr, "")

	if len(runs) == 0 {
		_, _ = fmt.Fprintln(stderr, "No runs found for this migration.")
		return nil
	}

	tw := tabwriter.NewWriter(stderr, 0, 8, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "Run\tSuccess\tFail")
	for _, run := range runs {
		success, fail := runSuccessFail(run.Counts)
		_, _ = fmt.Fprintf(tw, "%s  %s\t%d\t%d\n", migStatusGlyph(run.Status.String()), run.ID.String(), success, fail)
	}
	_ = tw.Flush()

	return nil
}

func findMigByID(ctx context.Context, httpClient *http.Client, baseURL *url.URL, migID string) (domainapi.MigSummary, error) {
	const pageSize int32 = 100
	var offset int32
	for {
		page, err := migs.ListMigsCommand{
			Client:  httpClient,
			BaseURL: baseURL,
			Limit:   pageSize,
			Offset:  offset,
		}.Run(ctx)
		if err != nil {
			return domainapi.MigSummary{}, err
		}
		for _, item := range page {
			if item.ID.String() == migID {
				return item, nil
			}
		}
		if len(page) < int(pageSize) {
			return domainapi.MigSummary{}, fmt.Errorf("mig %q not found", migID)
		}
		offset += pageSize
	}
}

func listRunsByMigID(ctx context.Context, httpClient *http.Client, baseURL *url.URL, migID string) ([]domaintypes.RunSummary, error) {
	const pageSize int32 = 100
	var offset int32
	result := make([]domaintypes.RunSummary, 0)

	for {
		page, err := migs.ListBatchesCommand{
			Client:  httpClient,
			BaseURL: baseURL,
			Limit:   pageSize,
			Offset:  offset,
		}.Run(ctx)
		if err != nil {
			return nil, err
		}
		for _, run := range page {
			if run.MigID.String() == migID {
				result = append(result, run)
			}
		}
		if len(page) < int(pageSize) {
			break
		}
		offset += pageSize
	}

	return result, nil
}

func runSuccessFail(counts *domaintypes.RunRepoCounts) (int32, int32) {
	if counts == nil {
		return 0, 0
	}
	return counts.Success, counts.Fail
}

func migStatusGlyph(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "started":
		return "⣽"
	case "success", "succeeded":
		return "✓"
	case "fail", "failed":
		return "✗"
	case "cancelled", "canceled":
		return "○"
	case "queued", "created":
		return "·"
	default:
		return " "
	}
}

func printMigStatusUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ploy mig status <mig-id>")
}

func migValueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}
