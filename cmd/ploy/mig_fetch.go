package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"
)

// handleMigFetch downloads artifacts for an existing Migs run into a directory.
func handleMigFetch(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printMigFetchUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("mig fetch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	runFlag := fs.String("run", "", "migs run id to fetch artifacts for")
	dir := fs.String("artifact-dir", "", "directory to download artifacts into")
	if err := parseFlagSet(fs, args, func() { printMigFetchUsage(stderr) }); err != nil {
		return err
	}

	runID := strings.TrimSpace(*runFlag)
	if runID == "" {
		printMigFetchUsage(stderr)
		return errors.New("run id required")
	}
	outDir := strings.TrimSpace(*dir)
	if outDir == "" {
		printMigFetchUsage(stderr)
		return errors.New("artifact-dir required")
	}

	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	return downloadRunArtifacts(ctx, base, httpClient, runID, outDir, stderr)
}

func printMigFetchUsage(w io.Writer) {
	_, _ = io.WriteString(w, "Usage: ploy mig fetch --run <run-id> --artifact-dir <path>\n")
}
