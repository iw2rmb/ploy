package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	runcmd "github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type StartOptions struct {
	RunID  string
	Output io.Writer
}

// RunStart implements `ploy run start <run-id>`.
// Starts execution for pending repos in a batch run.
func RunStart(ctx context.Context, opts StartOptions) error {
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

	cmd := runcmd.StartCommand{
		Client:  httpClient,
		BaseURL: base,
		RunID:   domaintypes.RunID(runID),
	}

	result, err := cmd.Run(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "Run %s: started %d repo(s), %d already done, %d pending\n",
		result.RunID, result.Started, result.AlreadyDone, result.Pending)

	return nil
}
