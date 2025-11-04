package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"
)

// handleModFetch downloads artifacts for an existing Mods ticket into a directory.
func handleModFetch(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod fetch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ticket := fs.String("ticket", "", "mods ticket id to fetch artifacts for")
	dir := fs.String("artifact-dir", "", "directory to download artifacts into")
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}

	tid := strings.TrimSpace(*ticket)
	if tid == "" {
		printModUsage(stderr)
		return errors.New("ticket required")
	}
	outDir := strings.TrimSpace(*dir)
	if outDir == "" {
		printModUsage(stderr)
		return errors.New("artifact-dir required")
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	return downloadTicketArtifacts(ctx, base, httpClient, tid, outDir, stderr)
}
