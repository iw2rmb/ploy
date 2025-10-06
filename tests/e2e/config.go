//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"strings"
)

// Config captures runtime knobs required to execute the Grid-based Mods E2E scenarios.
type Config struct {
	GridEndpoint string
	GridAPIKey   string
	GridID       string
	Tenant       string
	TicketPrefix string
	RepoOverride string
	GitLabToken  string
	SkipReason   string
}

// LoadConfig inspects the environment and prepares the E2E configuration.
func LoadConfig() Config {
	cfg := Config{
		GridEndpoint: strings.TrimSpace(os.Getenv("GRID_ENDPOINT")),
		GridAPIKey:   strings.TrimSpace(os.Getenv("GRID_API_KEY")),
		GridID:       strings.TrimSpace(os.Getenv("GRID_ID")),
		Tenant:       strings.TrimSpace(os.Getenv("PLOY_E2E_TENANT")),
		TicketPrefix: strings.TrimSpace(os.Getenv("PLOY_E2E_TICKET_PREFIX")),
		RepoOverride: strings.TrimSpace(os.Getenv("PLOY_E2E_REPO_OVERRIDE")),
		GitLabToken:  strings.TrimSpace(os.Getenv("PLOY_E2E_GITLAB_TOKEN")),
	}
	if cfg.TicketPrefix == "" {
		cfg.TicketPrefix = "e2e"
	}
	if cfg.GridEndpoint == "" {
		cfg.SkipReason = "GRID_ENDPOINT is not set; Grid discovery required for Mods E2E"
		return cfg
	}
	if cfg.Tenant == "" {
		cfg.SkipReason = "PLOY_E2E_TENANT is not set; workflow run requires a tenant"
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
