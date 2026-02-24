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
	fs := flag.NewFlagSet("mig artifacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		printMigUsage(stderr)
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printMigUsage(stderr)
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
