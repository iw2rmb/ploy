package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/mods"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

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
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	cmd := mods.ArtifactsCommand{
		BaseURL: base,
		Client:  httpClient,
		RunID:   domaintypes.RunID(runID),
		Output:  stderr,
	}
	return cmd.Run(ctx)
}
