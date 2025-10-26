package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/tunnel"
)

var gitlabConfigStoreFactory = func(ctx context.Context) (gitlabStore, error) {
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, err
	}
	return newHTTPGitlabConfigStore(base, httpClient), nil
}

var gitlabSignerClientFactory = func(ctx context.Context) (gitlabSignerClient, error) {
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, err
	}
	return newHTTPGitlabSignerClient(base, httpClient), nil
}

func resolveControlPlaneHTTP(ctx context.Context) (*url.URL, *http.Client, error) {
	var lastErr error
	endpoint := strings.TrimSpace(os.Getenv(controlPlaneURLEnv))

	if endpoint == "" {
		cfg, err := resolveIntegrationConfig(ctx)
		if err == nil {
			if trimmed := strings.TrimSpace(cfg.APIEndpoint); trimmed != "" {
				endpoint = trimmed
			}
		} else if !errors.Is(err, errGridClientDisabled) {
			lastErr = err
		}
	}

	if endpoint == "" {
		if lastErr != nil {
			return nil, nil, lastErr
		}
		return nil, nil, errors.New("control plane endpoint not configured; set PLOY_CONTROL_PLANE_URL or provide an API endpoint via 'ploy config gitlab set'")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("parse control plane url: %w", err)
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}

	if err := tunnel.EnsureFallbackNode(parsed); err != nil {
		return nil, nil, err
	}

	httpClient, err := newControlPlaneHTTPClient(parsed)
	if err != nil {
		return nil, nil, err
	}
	if err := tunnel.AttachHTTP(httpClient); err != nil {
		return nil, nil, err
	}
	return parsed, httpClient, nil
}

func newControlPlaneHTTPClient(base *url.URL) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if strings.EqualFold(base.Scheme, "https") {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		transport.TLSClientConfig = tlsCfg
	}
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}
	return client, nil
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
