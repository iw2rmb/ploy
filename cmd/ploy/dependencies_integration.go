package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type integrationConfig struct {
	APIEndpoint   string
	JetStreamURL  string
	JetStreamURLs []string
	IPFSGateway   string
	Features      map[string]string
	Version       string
}

// FeatureEnabled reports whether the named discovery feature is marked as enabled.
func (cfg integrationConfig) FeatureEnabled(name string) bool {
	if len(cfg.Features) == 0 {
		return false
	}
	value, ok := cfg.Features[name]
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "enabled")
}

var (
	discoveryCache     = newClusterInfoCache()
	fetchClusterInfoFn = fetchClusterInfo
)

func resolveIntegrationConfig(ctx context.Context) (integrationConfig, error) {
	endpoint := strings.TrimSpace(os.Getenv(gridEndpointEnv))

	if endpoint == "" {
		return integrationConfig{Features: map[string]string{}}, nil
	}

	info, err := loadClusterInfo(ctx, endpoint)
	if err != nil {
		fallback := integrationConfig{
			APIEndpoint: sanitizeAPIEndpoint(endpoint),
			Features:    map[string]string{},
		}
		return fallback, err
	}

	cfg := integrationConfig{
		APIEndpoint:   firstNonEmpty(info.APIEndpoint, sanitizeAPIEndpoint(endpoint)),
		JetStreamURLs: append([]string(nil), info.JetStreamURLs...),
		IPFSGateway:   strings.TrimSpace(info.IPFSGateway),
		Features:      copyFeaturesMap(info.Features),
		Version:       strings.TrimSpace(info.Version),
	}

	cfg.JetStreamURL = firstJetStreamRoute(cfg.JetStreamURLs)

	if cfg.Features == nil {
		cfg.Features = map[string]string{}
	}

	return cfg, nil
}

type clusterInfo struct {
	APIEndpoint   string
	JetStreamURLs []string
	IPFSGateway   string
	Features      map[string]string
	Version       string
}

func (c clusterInfo) clone() clusterInfo {
	clone := clusterInfo{
		APIEndpoint: strings.TrimSpace(c.APIEndpoint),
		IPFSGateway: strings.TrimSpace(c.IPFSGateway),
		Features:    copyFeaturesMap(c.Features),
		Version:     strings.TrimSpace(c.Version),
	}
	clone.JetStreamURLs = normalizeJetStreamRoutes(c.JetStreamURLs)
	return clone
}

type clusterInfoCache struct {
	mu      sync.Mutex
	entries map[string]clusterInfo
}

func newClusterInfoCache() *clusterInfoCache {
	return &clusterInfoCache{entries: make(map[string]clusterInfo)}
}

func (c *clusterInfoCache) get(endpoint string) (clusterInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, ok := c.entries[endpoint]
	if !ok {
		return clusterInfo{}, false
	}
	return info.clone(), true
}

func (c *clusterInfoCache) set(endpoint string, info clusterInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[endpoint] = info.clone()
}

func loadClusterInfo(ctx context.Context, endpoint string) (clusterInfo, error) {
	if info, ok := discoveryCache.get(endpoint); ok {
		return info, nil
	}

	info, err := fetchClusterInfoWithRetry(ctx, endpoint)
	if err != nil {
		return clusterInfo{}, err
	}

	discoveryCache.set(endpoint, info)
	return info, nil
}

func fetchClusterInfoWithRetry(ctx context.Context, endpoint string) (clusterInfo, error) {
	const attempts = 3
	baseBackoff := 100 * time.Millisecond
	var lastErr error
	for i := 0; i < attempts; i++ {
		info, err := fetchClusterInfoFn(ctx, endpoint)
		if err == nil {
			return info, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return clusterInfo{}, ctx.Err()
		default:
		}
		if i < attempts-1 {
			time.Sleep(time.Duration(i+1) * baseBackoff)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("cluster discovery failed")
	}
	return clusterInfo{}, lastErr
}

// fetchClusterInfo retrieves cluster discovery data from the provided endpoint.
func fetchClusterInfo(ctx context.Context, endpoint string) (info clusterInfo, err error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return clusterInfo{}, fmt.Errorf("grid endpoint required for discovery; set %s", gridEndpointEnv)
	}

	discoveryURL := fmt.Sprintf("%s/v1/cluster/info", strings.TrimRight(trimmed, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return clusterInfo{}, fmt.Errorf("create discovery request: %w", err)
	}
	if token := strings.TrimSpace(os.Getenv(gridAPIKeyEnv)); token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}
	if gridID := strings.TrimSpace(os.Getenv(gridIDEnv)); gridID != "" {
		req.Header.Set("X-Ploy-Grid-ID", gridID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return clusterInfo{}, fmt.Errorf("fetch discovery info: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close discovery response body: %w", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return clusterInfo{}, fmt.Errorf("discovery request failed: %s", resp.Status)
	}

	var payload struct {
		APIEndpoint   string            `json:"api_endpoint"`
		JetStreamURLs []string          `json:"jetstream_urls"`
		IPFSGateway   string            `json:"ipfs_gateway"`
		Features      map[string]string `json:"features"`
		Version       string            `json:"version"`
	}
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return clusterInfo{}, fmt.Errorf("decode discovery response: %w", err)
	}

	return clusterInfo{
		APIEndpoint:   strings.TrimSpace(payload.APIEndpoint),
		JetStreamURLs: normalizeJetStreamRoutes(payload.JetStreamURLs),
		IPFSGateway:   strings.TrimSpace(payload.IPFSGateway),
		Features:      copyFeaturesMap(payload.Features),
		Version:       strings.TrimSpace(payload.Version),
	}, nil
}

func normalizeJetStreamRoutes(routes []string) []string {
	if len(routes) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(routes))
	for _, route := range routes {
		trimmed := strings.TrimSpace(route)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func firstJetStreamRoute(routes []string) string {
	for _, route := range routes {
		trimmed := strings.TrimSpace(route)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func copyFeaturesMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		dst[trimmedKey] = strings.TrimSpace(value)
	}
	if len(dst) == 0 {
		return map[string]string{}
	}
	return dst
}

func sanitizeAPIEndpoint(value string) string {
	trimmed := strings.TrimSpace(value)
	return strings.TrimRight(trimmed, "/")
}

func sanitizePathComponent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	builder := strings.Builder{}
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	component := builder.String()
	component = strings.Trim(component, "-_")
	return component
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
