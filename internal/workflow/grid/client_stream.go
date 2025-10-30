package grid

import (
	"context"
	"errors"
	"fmt"
	"strings"

	workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"
)

type terminalRun struct {
	status   workflowsdk.RunStatus
	message  string
	metadata map[string]string
	result   map[string]any
}

const (
	archiveResultIDKey       = "archive_export_id"
	archiveResultClassKey    = "archive_export_class"
	archiveResultQueuedAtKey = "archive_export_queued_at"
)

func (c *Client) awaitTerminalStatus(ctx context.Context, runID, tenant, workflowID string) (terminalRun, error) {
    _ = tenant
    streamReq := workflowsdk.StreamRequest{
        WorkflowID: strings.TrimSpace(workflowID),
        RunID:      strings.TrimSpace(runID),
    }
	if streamReq.RunID == "" {
		return terminalRun{}, fmt.Errorf("workflow run id is required")
	}

	var final terminalRun

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	opts := c.streamOpts
    if opts.CursorStore == nil && c.cursorFactory != nil {
        store, err := c.cursorFactory("", streamReq.WorkflowID, streamReq.RunID)
        if err != nil {
            return terminalRun{}, err
        }
        opts.CursorStore = store
    }

	streamErr := c.stream(streamCtx, c.rpc.Client(), streamReq, func(evt workflowsdk.StatusEvent) error {
		if evt.RunID == "" {
			return nil
		}
		if message := strings.TrimSpace(evt.Message); message != "" {
			final.message = message
		}
		if isTerminalStatus(evt.Status) {
			final.status = evt.Status
			cancel()
		}
		return nil
	}, opts)

	if streamErr != nil {
		if ctx.Err() != nil {
			return terminalRun{}, ctx.Err()
		}
		if errors.Is(streamErr, context.Canceled) && final.status != "" {
			streamErr = nil
		}
	}

    meta, err := c.rpc.Metadata(ctx, workflowsdk.MetadataRequest{WorkflowID: workflowID, RunID: runID})
	if err != nil {
		if final.status == "" {
			if streamErr != nil {
				return terminalRun{}, streamErr
			}
			return terminalRun{}, err
		}
	} else {
		final.metadata = copyStringMap(meta.Run.Metadata)
		final.result = cloneAnyMap(meta.Run.Result)
		if final.status == "" {
			final.status = meta.Run.Status
		}
		if final.message == "" && meta.Run.Result != nil {
			if msg, ok := meta.Run.Result["reason"].(string); ok {
				final.message = strings.TrimSpace(msg)
			}
		}
	}

	if final.status == "" {
		if streamErr != nil {
			return terminalRun{}, streamErr
		}
		final.status = workflowsdk.RunStatusSucceeded
	}

	return final, nil
}
