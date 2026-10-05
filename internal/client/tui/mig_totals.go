package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	sharedclient "github.com/iw2rmb/ploy/internal/client"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// CountMigRunsCommand counts runs belonging to a specific migration by scanning the runs list.
type CountMigRunsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigID   domaintypes.MigID
}

// Run fetches runs pages and counts those whose MigID matches the configured migration.
func (c CountMigRunsCommand) Run(ctx context.Context) (int, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return 0, fmt.Errorf("count mig runs: %w", err)
	}
	if c.MigID.IsZero() {
		return 0, fmt.Errorf("count mig runs: mig id required")
	}

	const pageSize = int32(100)
	var offset int32
	total := 0

	for {
		page, err := sharedclient.ListRunsCommand{
			Client:  c.Client,
			BaseURL: c.BaseURL,
			Limit:   pageSize,
			Offset:  offset,
		}.Run(ctx)
		if err != nil {
			return 0, fmt.Errorf("count mig runs: %w", err)
		}
		for _, run := range page {
			if run.MigID == c.MigID {
				total++
			}
		}
		if len(page) < int(pageSize) {
			break
		}
		offset += pageSize
	}

	return total, nil
}
