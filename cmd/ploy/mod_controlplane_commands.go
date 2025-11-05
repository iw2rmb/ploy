package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/mods"
)

func handleModCancel(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod cancel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ticket := fs.String("ticket", "", "mods ticket id to cancel")
	reason := fs.String("reason", "", "optional reason for cancellation")
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}
	if strings.TrimSpace(*ticket) == "" {
		printModUsage(stderr)
		return errors.New("ticket required")
	}
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.CancelCommand{BaseURL: base, Client: httpClient, Ticket: strings.TrimSpace(*ticket), Reason: strings.TrimSpace(*reason), Output: stderr}
	return cmd.Run(ctx)
}

func handleModResume(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod resume", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printModUsage(stderr)
		return errors.New("ticket required")
	}
	ticket := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.ResumeCommand{BaseURL: base, Client: httpClient, Ticket: ticket, Output: stderr}
	return cmd.Run(ctx)
}

func handleModInspect(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod inspect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printModUsage(stderr)
		return errors.New("ticket required")
	}
	ticket := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.InspectCommand{BaseURL: base, Client: httpClient, Ticket: ticket, Output: stderr}
	return cmd.Run(ctx)
}

func handleModArtifacts(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod artifacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printModUsage(stderr)
		return errors.New("ticket required")
	}
	ticket := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.ArtifactsCommand{BaseURL: base, Client: httpClient, Ticket: ticket, Output: stderr}
	return cmd.Run(ctx)
}

func handleModDiffs(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod diffs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	download := fs.Bool("download", false, "download newest diff and print to stdout (gunzipped)")
	savePath := fs.String("output", "", "save newest diff to file (gunzipped)")
	// Allow both orders: flags before or after ticket.
	var ticketArg string
	if len(args) > 0 && !strings.HasPrefix(strings.TrimSpace(args[0]), "-") {
		ticketArg = strings.TrimSpace(args[0])
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}
	rest := fs.Args()
	if ticketArg == "" {
		if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
			printModUsage(stderr)
			return errors.New("ticket required")
		}
		ticketArg = strings.TrimSpace(rest[0])
	}
	ticket := ticketArg
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.DiffsCommand{BaseURL: base, Client: httpClient, Ticket: ticket, Output: stderr, Download: *download, SavePath: strings.TrimSpace(*savePath)}
	return cmd.Run(ctx)
}
