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

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return httpx.WrapError("migs artifacts", resp.Status, resp.Body)
	}
	// Decode RunSummary directly — the server returns the canonical type (no wrapper).
	var summary modsapi.RunSummary
	if err := httpx.DecodeResponseJSON(resp.Body, &summary, httpx.MaxJSONBodyBytes); err != nil {
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
