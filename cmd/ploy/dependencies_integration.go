package main

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/runtime"
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

func resolveIntegrationConfig(ctx context.Context) (integrationConfig, error) {
	selection := strings.TrimSpace(os.Getenv(runtimeAdapterEnv))
	if selection == "" {
		selection = "grid"
	}
	if runtimeRegistry != nil {
		_, meta, err := runtimeRegistry.Resolve(selection)
		if errors.Is(err, runtime.ErrAdapterNotFound) {
			return integrationConfig{Features: map[string]string{}}, nil
		}
		if err != nil {
			return integrationConfig{}, err
		}
		if meta.Name != "grid" {
			return integrationConfig{Features: map[string]string{}}, nil
		}
	} else if !strings.EqualFold(selection, "grid") {
		return integrationConfig{Features: map[string]string{}}, nil
	}

	client, err := acquireGridClient(ctx)
	if errors.Is(err, errGridClientDisabled) {
		return integrationConfig{Features: map[string]string{}}, nil
	}
	if err != nil {
		return integrationConfig{}, err
	}

	status := client.Status()
	discovery := status.Discovery

	cfg := integrationConfig{
		APIEndpoint:   firstNonEmpty(strings.TrimSpace(discovery.APIEndpoint), strings.TrimSpace(status.Beacon.APIEndpoint)),
		JetStreamURLs: normalizeJetStreamRoutes(discovery.JetStreamURLs),
		IPFSGateway:   strings.TrimSpace(discovery.IPFSGateway),
		Features:      copyFeaturesMap(discovery.Features),
		Version:       strings.TrimSpace(discovery.Version),
	}
	cfg.JetStreamURL = firstJetStreamRoute(cfg.JetStreamURLs)

	if cfg.Features == nil {
		cfg.Features = map[string]string{}
	}

	return cfg, nil
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
