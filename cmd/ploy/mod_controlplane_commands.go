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
