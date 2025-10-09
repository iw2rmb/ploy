//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"strings"
)

// Config captures runtime knobs required to execute the Grid-based Mods E2E scenarios.
type Config struct {
	GridID       string
	GridAPIKey   string
	BeaconURL    string
	Tenant       string
	TicketPrefix string
	RepoOverride string
	GitLabToken  string
	SkipReason   string
}

// LoadConfig inspects the environment and prepares the E2E configuration.
func LoadConfig() Config {
	cfg := Config{
		GridID:       strings.TrimSpace(os.Getenv("PLOY_GRID_ID")),
		GridAPIKey:   strings.TrimSpace(os.Getenv("PLOY_GRID_API_KEY")),
		BeaconURL:    strings.TrimSpace(os.Getenv("GRID_CLIENT_BEACON_URL")),
		Tenant:       strings.TrimSpace(os.Getenv("PLOY_E2E_TENANT")),
		TicketPrefix: strings.TrimSpace(os.Getenv("PLOY_E2E_TICKET_PREFIX")),
		RepoOverride: strings.TrimSpace(os.Getenv("PLOY_E2E_REPO_OVERRIDE")),
		GitLabToken:  strings.TrimSpace(os.Getenv("PLOY_E2E_GITLAB_TOKEN")),
	}
	if cfg.TicketPrefix == "" {
		cfg.TicketPrefix = "e2e"
	}
	if cfg.GridID == "" {
		cfg.SkipReason = "PLOY_GRID_ID is not set; grid client requires a grid identifier"
		return cfg
	}
	if cfg.GridAPIKey == "" {
		cfg.SkipReason = "PLOY_GRID_API_KEY is not set; grid client requires a grid API key"
		return cfg
	}
	if cfg.Tenant == "" {
		cfg.SkipReason = "PLOY_E2E_TENANT is not set; mod run requires a tenant"
		return cfg
	}
	return cfg
}

// TicketID returns a deterministic ticket identifier for a scenario.
func (c Config) TicketID(scenarioID string) string {
	trimmed := strings.TrimSpace(scenarioID)
	if trimmed == "" {
		trimmed = "scenario"
	}
	prefix := strings.TrimSpace(c.TicketPrefix)
	if prefix == "" {
		prefix = "e2e"
	}
	return fmt.Sprintf("%s-%s", prefix, trimmed)
}

// TargetRef returns a unique git branch name for the provided scenario.
func (c Config) TargetRef(scenarioID string) string {
	trimmed := strings.TrimSpace(scenarioID)
	if trimmed == "" {
		trimmed = "scenario"
	}
	prefix := strings.TrimSpace(c.TicketPrefix)
	if prefix == "" {
		prefix = "e2e"
	}
	return fmt.Sprintf("%s-%s", prefix, trimmed)
}
