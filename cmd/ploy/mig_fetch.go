package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"
)

// handleMigFetch downloads artifacts for an existing Mods run into a directory.
func handleMigFetch(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("mod fetch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runFlag := fs.String("run", "", "mods run id to fetch artifacts for")
	dir := fs.String("artifact-dir", "", "directory to download artifacts into")
	if err := fs.Parse(args); err != nil {
		printMigUsage(stderr)
		return err
	}

	runID := strings.TrimSpace(*runFlag)
	if runID == "" {
		printMigUsage(stderr)
		return errors.New("run id required")
	}
	outDir := strings.TrimSpace(*dir)
	if outDir == "" {
		printMigUsage(stderr)
		return errors.New("artifact-dir required")
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	return downloadRunArtifacts(ctx, base, httpClient, runID, outDir, stderr)
}
