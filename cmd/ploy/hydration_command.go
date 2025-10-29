package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	cliHydration "github.com/iw2rmb/ploy/internal/cli/hydration"
)

type hydrationCommandRunner interface {
	Inspect(ticket string) error
	Tune(ticket string, opts hydrationTuneOptions) error
}

type hydrationTuneOptions struct {
	TTL            string
	ReplicationMin *int
	ReplicationMax *int
	Share          *bool
}

func handleHydration(args []string, stderr io.Writer, runner hydrationCommandRunner) error {
	if len(args) == 0 {
		printHydrationUsage(stderr)
		return errors.New("hydration subcommand required")
	}
	if runner == nil {
		var err error
		runner, err = newControlPlaneHydrationRunner(stderr)
		if err != nil {
			return err
		}
	}

	switch args[0] {
	case "inspect":
		if len(args) < 2 {
			printHydrationUsage(stderr)
			return errors.New("ticket required")
		}
		ticket := strings.TrimSpace(args[1])
		if ticket == "" {
			printHydrationUsage(stderr)
			return errors.New("ticket required")
		}
		return runner.Inspect(ticket)
	case "tune":
		opts, ticket, err := parseHydrationTune(args[1:], stderr)
		if err != nil {
			return err
		}
		return runner.Tune(ticket, opts)
	default:
		printHydrationUsage(stderr)
		return fmt.Errorf("unknown hydration subcommand %q", args[0])
	}
}

func parseHydrationTune(args []string, stderr io.Writer) (hydrationTuneOptions, string, error) {
	fs := flag.NewFlagSet("hydration tune", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	ttl := fs.String("ttl", "", "set hydration retention ttl (duration e.g. 24h)")
	repMin := fs.Int("replication-min", -1, "set minimum replication factor")
	repMax := fs.Int("replication-max", -1, "set maximum replication factor")
	share := fs.String("share", "", "toggle sharing (true|false)")

	if err := fs.Parse(args); err != nil {
		printHydrationUsage(stderr)
		return hydrationTuneOptions{}, "", err
	}
	remainder := fs.Args()
	if len(remainder) == 0 {
		printHydrationUsage(stderr)
		return hydrationTuneOptions{}, "", errors.New("ticket required")
	}
	ticket := strings.TrimSpace(remainder[0])
	if ticket == "" {
		printHydrationUsage(stderr)
		return hydrationTuneOptions{}, "", errors.New("ticket required")
	}

	opts := hydrationTuneOptions{}
	if trimmed := strings.TrimSpace(*ttl); trimmed != "" {
		opts.TTL = trimmed
	}
	if *repMin >= 0 {
		value := *repMin
		opts.ReplicationMin = &value
	}
	if *repMax >= 0 {
		value := *repMax
		opts.ReplicationMax = &value
	}
	if trimmed := strings.TrimSpace(*share); trimmed != "" {
		lower := strings.ToLower(trimmed)
		switch lower {
		case "true", "1", "yes", "on":
			v := true
			opts.Share = &v
		case "false", "0", "no", "off":
			v := false
			opts.Share = &v
		default:
			printHydrationUsage(stderr)
			return hydrationTuneOptions{}, "", fmt.Errorf("invalid share value %q", trimmed)
		}
	}
	return opts, ticket, nil
}

func printHydrationUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: ploy hydration <inspect|tune> [options] <ticket>")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  inspect <ticket>             Show hydration snapshot policy")
	fmt.Fprintln(w, "  tune [flags] <ticket>        Update hydration retention policy")
	fmt.Fprintln(w, "Flags (tune):")
	fmt.Fprintln(w, "  --ttl <duration>             Set TTL (e.g. 24h)")
	fmt.Fprintln(w, "  --replication-min <n>        Set minimum replication factor")
	fmt.Fprintln(w, "  --replication-max <n>        Set maximum replication factor")
	fmt.Fprintln(w, "  --share <true|false>         Enable or disable reuse sharing")
}

type controlPlaneHydrationRunner struct {
	client cliHydration.Client
	output io.Writer
}

func newControlPlaneHydrationRunner(stderr io.Writer) (hydrationCommandRunner, error) {
	ctx := context.Background()
	baseURL, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, err
	}
	client := cliHydration.HTTPClient{
		BaseURL:    baseURL,
		HTTPClient: httpClient,
	}
	return controlPlaneHydrationRunner{client: client, output: stderr}, nil
}

func (r controlPlaneHydrationRunner) Inspect(ticket string) error {
	cmd := cliHydration.InspectCommand{
		Ticket: ticket,
		Client: r.client,
		Output: r.output,
	}
	return cmd.Run(context.Background())
}

func (r controlPlaneHydrationRunner) Tune(ticket string, opts hydrationTuneOptions) error {
	request := cliHydration.TuneRequest{
		TTL:            opts.TTL,
		ReplicationMin: opts.ReplicationMin,
		ReplicationMax: opts.ReplicationMax,
		Share:          opts.Share,
	}
	cmd := cliHydration.TuneCommand{
		Ticket: ticket,
		Client: r.client,
	}
	return cmd.Run(context.Background(), request)
}
