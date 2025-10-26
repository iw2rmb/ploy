package config

import (
	"sort"
	"strings"
)

// normalizeRuntimeConfig deduplicates runtimes, fills defaults, and keeps adapters stable.
func normalizeRuntimeConfig(rt *RuntimeConfig) {
	if rt == nil {
		return
	}
	plugins := make([]RuntimePluginConfig, 0, len(rt.Plugins))
	seen := make(map[string]struct{}, len(rt.Plugins))
	for _, plugin := range rt.Plugins {
		name := strings.TrimSpace(strings.ToLower(plugin.Name))
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		module := strings.TrimSpace(plugin.Module)
		if plugin.Config == nil {
			plugin.Config = make(map[string]any)
		}
		if plugin.Enabled == nil {
			plugin.Enabled = ptrTo(true)
		}
		plugins = append(plugins, RuntimePluginConfig{
			Name:    name,
			Module:  module,
			Enabled: plugin.Enabled,
			Config:  plugin.Config,
		})
	}
	sort.SliceStable(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})
	rt.Plugins = plugins

	rt.DefaultAdapter = strings.TrimSpace(strings.ToLower(rt.DefaultAdapter))
	if rt.DefaultAdapter == "" {
		if len(rt.Plugins) > 0 {
			rt.DefaultAdapter = rt.Plugins[0].Name
		} else {
			rt.DefaultAdapter = "local"
		}
	}
}

// ptrTo returns a pointer to the provided value.
func ptrTo[T any](v T) *T {
	return &v
}
