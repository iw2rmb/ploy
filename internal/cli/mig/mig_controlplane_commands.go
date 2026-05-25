package mig

import (
	"context"
	"errors"
	"flag"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/migs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func handleMigArtifacts(args []string, stderr io.Writer) error {
	if common.WantsHelp(args) {
		printMigArtifactsUsage(stderr)
		return nil
	}

	fs := flag.NewFlagSet("mig artifacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := common.ParseFlagSet(fs, args, func() { printMigArtifactsUsage(stderr) }); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || strings.TrimSpace(rest[0]) == "" {
		printMigArtifactsUsage(stderr)
		return errors.New("run id required")
	}
	runID := strings.TrimSpace(rest[0])
	ctx := context.Background()
	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
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
