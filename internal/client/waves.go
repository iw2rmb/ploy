package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

type GetWaveCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	WaveID  domaintypes.WaveID
}

func (c GetWaveCommand) Run(ctx context.Context) (domainapi.WaveSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domainapi.WaveSummary{}, fmt.Errorf("wave status: %w", err)
	}
	endpoint := c.BaseURL.JoinPath("v1", "waves", c.WaveID.String())
	return httpx.DoJSON[domainapi.WaveSummary](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "wave status")
}

type CancelWaveCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	WaveID  domaintypes.WaveID
}

func (c CancelWaveCommand) Run(ctx context.Context) (domainapi.WaveSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domainapi.WaveSummary{}, fmt.Errorf("wave cancel: %w", err)
	}
	endpoint := c.BaseURL.JoinPath("v1", "waves", c.WaveID.String(), "cancel")
	return httpx.DoJSON[domainapi.WaveSummary](ctx, c.Client, http.MethodPost, endpoint.String(), nil, http.StatusOK, "wave cancel")
}

type FollowWaveCommand struct {
	Get      GetWaveCommand
	OnUpdate func(domainapi.WaveSummary) error
}

func (c FollowWaveCommand) Run(ctx context.Context) (domainapi.WaveSummary, error) {
	if c.OnUpdate == nil {
		return domainapi.WaveSummary{}, fmt.Errorf("wave follow: update callback required")
	}
	for {
		wave, err := c.Get.Run(ctx)
		if err != nil {
			return domainapi.WaveSummary{}, err
		}
		if err := c.OnUpdate(wave); err != nil {
			return domainapi.WaveSummary{}, err
		}
		if lifecycle.IsTerminalWaveStatus(wave.Status) {
			return wave, nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return domainapi.WaveSummary{}, ctx.Err()
		case <-timer.C:
		}
	}
}
