package migs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// ArtifactsCommand lists artifacts attached to a Migs run by stage.
type ArtifactsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	Output  io.Writer
}

// Run performs GET /v1/runs/{id}/status and prints per-stage artifacts.
func (c ArtifactsCommand) Run(ctx context.Context) error {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return fmt.Errorf("migs artifacts: %w", err)
	}
	if c.RunID.IsZero() {
		return errors.New("migs artifacts: run id required")
	}
	runID := c.RunID.String()
	endpoint := c.BaseURL.JoinPath("v1", "runs", runID, "status")
	summary, err := httpx.DoJSON[migsapi.RunSummary](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "migs artifacts")
	if err != nil {
		return err
	}
	if c.Output == nil {
		return nil
	}
	// Stable iteration order by stage id (map key = job ID, KSUID string).
	var stageIDs []domaintypes.JobID
	for id := range summary.Stages {
		stageIDs = append(stageIDs, id)
	}
	sort.Slice(stageIDs, func(i, j int) bool { return stageIDs[i].String() < stageIDs[j].String() })
	for _, id := range stageIDs {
		st := summary.Stages[id]
		_, _ = fmt.Fprintf(c.Output, "%s:\n", strings.TrimSpace(id.String()))
		if len(st.Artifacts) == 0 {
			continue
		}
		// Stable artifact key order.
		var keys []string
		for k := range st.Artifacts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := st.Artifacts[k]
			_, _ = fmt.Fprintf(c.Output, "  %s: %s\n", strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
	return nil
}
