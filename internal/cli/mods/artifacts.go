package mods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// ArtifactsCommand lists artifacts attached to a Mods run by stage.
type ArtifactsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	Output  io.Writer
}

// Run performs GET /v1/runs/{id}/status and prints per-stage artifacts.
func (c ArtifactsCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("mods artifacts: http client required")
	}
	if c.BaseURL == nil {
		return errors.New("mods artifacts: base url required")
	}
	if c.RunID.IsZero() {
		return errors.New("mods artifacts: run id required")
	}
	runID := c.RunID.String()
	endpoint, err := url.JoinPath(c.BaseURL.String(), "v1", "runs", url.PathEscape(runID), "status")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("mods artifacts: %s", msg)
	}
	// Decode RunSummary directly — the server returns the canonical type (no wrapper).
	var summary modsapi.RunSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return err
	}
	if c.Output == nil {
		return nil
	}
	// Stable iteration order by stage id (map key = job ID, KSUID string).
	var stageIDs []string
	for id := range summary.Stages {
		stageIDs = append(stageIDs, id)
	}
	sort.Strings(stageIDs)
	for _, id := range stageIDs {
		st := summary.Stages[id]
		_, _ = fmt.Fprintf(c.Output, "%s:\n", strings.TrimSpace(id))
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
