package mods

import (
	"net/url"
	"strings"
)

// HarnessConfig captures environment-driven configuration for Mods integration tests and harnesses.
type HarnessConfig struct {
	Infra            Infra
	seaweedFallbacks []string
	seaweedMaster    string
}

// ResolveHarness resolves harness configuration using the provided getter.
func ResolveHarness(get func(string) string) HarnessConfig {
	infra := ResolveInfra(get)
	if infra.Controller == "" {
		infra.Controller = "http://localhost:8080/v1"
		infra.APIBase = strings.TrimSuffix(infra.Controller, "/v1")
	}
	fallbackCSV := strings.TrimSpace(get("MODS_SEAWEED_FALLBACKS"))
	var fallbacks []string
	if fallbackCSV != "" {
		for _, raw := range strings.Split(fallbackCSV, ",") {
			candidate := strings.TrimSpace(raw)
			if candidate != "" {
				fallbacks = append(fallbacks, candidate)
			}
		}
	}
	master := strings.TrimSpace(get("MODS_SEAWEED_MASTER"))
	return HarnessConfig{Infra: infra, seaweedFallbacks: fallbacks, seaweedMaster: master}
}

// ResolveHarnessFromEnv resolves harness configuration using the process environment.
func ResolveHarnessFromEnv() HarnessConfig { return ResolveHarness(getenv) }

// SeaweedCandidates returns the ordered list of SeaweedFS filer endpoints to try.
func (h HarnessConfig) SeaweedCandidates() []string {
	seen := map[string]struct{}{}
	var ordered []string
	add := func(values ...string) {
		for _, v := range values {
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			ordered = append(ordered, trimmed)
		}
	}
	add(h.Infra.SeaweedURL)
	// If no primary was provided, fall back to defaults.SeaweedURL
	if len(ordered) == 0 {
		defaults := ResolveDefaultsFromEnv()
		add(defaults.SeaweedURL)
	}
	add(h.seaweedFallbacks...)
	// Provide legacy default when nothing else present
	if len(ordered) == 0 {
		add("http://seaweedfs-filer.storage.ploy.local:8888")
	}
	return ordered
}

// SeaweedFilerHost returns the host:port for the SeaweedFS filer.
func (h HarnessConfig) SeaweedFilerHost() string {
	return hostPortFromURL(h.Infra.SeaweedURL)
}

// SeaweedMasterHost returns the host:port for the SeaweedFS master service.
func (h HarnessConfig) SeaweedMasterHost() string {
	if h.seaweedMaster != "" {
		return h.seaweedMaster
	}
	host := hostPortFromURL(h.Infra.SeaweedURL)
	if host == "" {
		return ""
	}
	if strings.Contains(host, ":") {
		parts := strings.Split(host, ":")
		return parts[0] + ":9333"
	}
	return host + ":9333"
}

func hostPortFromURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return strings.TrimPrefix(strings.TrimPrefix(trimmed, "https://"), "http://")
	}
	if u.Host != "" {
		return u.Host
	}
	return strings.TrimPrefix(strings.TrimPrefix(trimmed, "https://"), "http://")
}
