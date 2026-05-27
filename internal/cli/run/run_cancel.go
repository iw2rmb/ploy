package run

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type CancelOptions struct {
	RunID  string
	Reason string
	Output io.Writer
}

func RunCancel(ctx context.Context, opts CancelOptions) error {
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
	cmd := runs.CancelCommand{
		BaseURL: base,
		Client:  httpClient,
		RunID:   domaintypes.RunID(runID),
		Reason:  strings.TrimSpace(opts.Reason),
		Output:  out,
	}
	return cmd.Run(ctx)
}
