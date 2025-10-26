package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/controlplane"
)

var gitlabConfigStoreFactory = func(ctx context.Context, opts controlplane.Options) (gitlabStore, error) {
	base, httpClient, err := controlplane.ResolveHTTP(ctx, opts)
	if err != nil {
		return nil, err
	}
	return newHTTPGitlabConfigStore(base, httpClient), nil
}

var gitlabSignerClientFactory = func(ctx context.Context, opts controlplane.Options) (gitlabSignerClient, error) {
	base, httpClient, err := controlplane.ResolveHTTP(ctx, opts)
	if err != nil {
		return nil, err
	}
	return newHTTPGitlabSignerClient(base, httpClient), nil
}

func resolveControlPlaneHTTP(ctx context.Context) (*url.URL, *http.Client, error) {
	return controlplane.ResolveHTTP(ctx, controlplane.Options{})
}

func parseTimestamp(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return ts
	}
	return time.Time{}
}

func controlPlaneHTTPError(resp *http.Response) error {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("control plane responded with %s", resp.Status)
	}
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &payload); err == nil {
			if msg := strings.TrimSpace(payload.Message); msg != "" {
				return errors.New(msg)
			}
			if msg := strings.TrimSpace(payload.Error); msg != "" {
				return errors.New(msg)
			}
		}
		if msg := strings.TrimSpace(string(data)); msg != "" {
			return errors.New(msg)
		}
	}
	return fmt.Errorf("control plane responded with %s", resp.Status)
}

func closeGitlabSignerClient(client gitlabSignerClient) {
	if closer, ok := client.(interface{ Close() error }); ok && closer != nil {
		_ = closer.Close()
	}
}
