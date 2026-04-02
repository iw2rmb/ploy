package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func handleMigArtifacts(args []string, stderr io.Writer) error {
	if wantsHelp(args) {
		printMigArtifactsUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("mig artifacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := parseFlagSet(fs, args, func() { printMigArtifactsUsage(stderr) }); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printMigArtifactsUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := migs.ArtifactsCommand{
		BaseURL: base,
		Client:  httpClient,
		RunID:   domaintypes.RunID(runID),
		Output:  stderr,
	}
	return cmd.Run(ctx)
}

func printMigArtifactsUsage(w io.Writer) {
	_, _ = io.WriteString(w, "Usage: ploy mig artifacts <run-id>\n")
}
