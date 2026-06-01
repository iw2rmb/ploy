package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type RestartOptions struct {
	RunID  string
	Output io.Writer
}

func RunRestart(ctx context.Context, opts RestartOptions) error {
	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		return errors.New("run id required")
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	summary, err := runs.RestartCommand{
		BaseURL: base,
		Client:  httpClient,
		RunID:   domaintypes.RunID(runID),
	}.Run(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Restarted run %s (attempt %d)\n", summary.ID, summary.Attempt)
	return nil
}
