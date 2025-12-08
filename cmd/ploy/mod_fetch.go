package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"
)

// handleModFetch downloads artifacts for an existing Mods run into a directory.
func handleModFetch(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod fetch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	// Keep --ticket flag for backward compatibility with existing scripts.
	ticket := fs.String("ticket", "", "mods run id to fetch artifacts for")
	dir := fs.String("artifact-dir", "", "directory to download artifacts into")
	if err := fs.Parse(args); err != nil {
		printModUsage(stderr)
		return err
	}

	runID := strings.TrimSpace(*ticket)
	if runID == "" {
		printModUsage(stderr)
		return errors.New("run id required")
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
	return downloadRunArtifacts(ctx, base, httpClient, runID, outDir, stderr)
}
