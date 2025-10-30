package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "os"
    "strings"

    "github.com/iw2rmb/ploy/internal/cli/controlplane"
)

type integrationConfig struct {
    APIEndpoint   string
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

const (
    ipfsGatewayEnv = "PLOY_IPFS_GATEWAY"
)

func resolveIntegrationConfig(ctx context.Context) (integrationConfig, error) {
    // Start with environment overrides to keep local/dev and tests simple.
    cfg := integrationConfig{Features: map[string]string{}}
    if v := strings.TrimSpace(os.Getenv(ipfsGatewayEnv)); v != "" {
        cfg.IPFSGateway = v
    }

    // Try control plane discovery for cluster-scoped settings when reachable.
    target, err := controlplane.ResolveTarget(ctx, controlplane.Options{})
    if err != nil || target.BaseURL == nil || strings.TrimSpace(target.ClusterID) == "" {
        // No control plane configured; return env-driven config (may be empty).
        return cfg, nil
    }
    base := *target.BaseURL
    q := base.Query()
    q.Set("cluster_id", strings.TrimSpace(target.ClusterID))
    base.RawQuery = q.Encode()
    base.Path = strings.TrimSuffix(base.Path, "/") + "/v1/config"

    httpBase, httpClient, err := controlplane.ResolveHTTP(ctx, controlplane.Options{})
    if err != nil || httpBase == nil || httpClient == nil {
        return cfg, nil
    }

    // Build absolute URL reusing the resolved client (with tunnels/TLS).
    endpoint, _ := url.Parse(httpBase.String())
    endpoint.RawQuery = base.RawQuery
    endpoint.Path = base.Path

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
    if err != nil {
        return cfg, nil
    }
    resp, err := httpClient.Do(req)
    if err != nil {
        return cfg, nil
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return cfg, nil
    }

    var payload struct {
        Config map[string]any `json:"config"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
        return cfg, nil
    }
    // Best-effort extraction of optional fields from the config document.
    if v := strings.TrimSpace(asString(payload.Config["api_endpoint"])); v != "" {
        cfg.APIEndpoint = v
    }
    // JetStream routes removed; SSE is the streaming surface.
    // IPFS gateway (optional in Next; worker uses Cluster for publishing).
    if v := strings.TrimSpace(asString(payload.Config["ipfs_gateway"])); v != "" && cfg.IPFSGateway == "" {
        cfg.IPFSGateway = v
    }
    // Version and Features if present.
    if v := strings.TrimSpace(asString(payload.Config["version"])); v != "" {
        cfg.Version = v
    }
    if features, ok := payload.Config["features"].(map[string]any); ok {
        cfg.Features = map[string]string{}
        for k, v := range features {
            cfg.Features[strings.TrimSpace(k)] = strings.TrimSpace(asString(v))
        }
    }
    return cfg, nil
}

// asString renders arbitrary interface values as strings.
func asString(value any) string {
    switch v := value.(type) {
    case string:
        return v
    case fmt.Stringer:
        return v.String()
    default:
        return ""
    }
}

// JetStream helpers removed.

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
