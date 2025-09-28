package contracts

import (
	"fmt"
	"strings"
	"time"
)

// ModsStageMetadata captures Mods-specific checkpoint metadata.
type ModsStageMetadata struct {
	Plan            *ModsPlanMetadata    `json:"plan,omitempty"`
	Human           *ModsHumanMetadata   `json:"human,omitempty"`
	Recommendations []ModsRecommendation `json:"recommendations,omitempty"`
}

// Validate ensures Mods metadata entries are well formed.
func (m ModsStageMetadata) Validate() error {
	if m.Plan != nil {
		if err := m.Plan.Validate(); err != nil {
			return fmt.Errorf("plan metadata invalid: %w", err)
		}
	}
	if m.Human != nil {
		if err := m.Human.Validate(); err != nil {
			return fmt.Errorf("human metadata invalid: %w", err)
		}
	}
	for i, rec := range m.Recommendations {
		if err := rec.Validate(); err != nil {
			return fmt.Errorf("recommendation %d invalid: %w", i, err)
		}
	}
	return nil
}

// ModsPlanMetadata documents planner decisions included in checkpoints.
type ModsPlanMetadata struct {
	SelectedRecipes []string `json:"selected_recipes,omitempty"`
	ParallelStages  []string `json:"parallel_stages,omitempty"`
	HumanGate       bool     `json:"human_gate"`
	Summary         string   `json:"summary,omitempty"`
	PlanTimeout     string   `json:"plan_timeout,omitempty"`
	MaxParallel     int      `json:"max_parallel,omitempty"`
}

// Validate ensures Mods plan metadata entries contain non-empty values.
func (m ModsPlanMetadata) Validate() error {
	for i, recipe := range m.SelectedRecipes {
		if strings.TrimSpace(recipe) == "" {
			return fmt.Errorf("selected recipe %d is empty", i)
		}
	}
	for i, stage := range m.ParallelStages {
		if strings.TrimSpace(stage) == "" {
			return fmt.Errorf("parallel stage %d is empty", i)
		}
	}
	if trimmed := strings.TrimSpace(m.PlanTimeout); trimmed != "" {
		if _, err := time.ParseDuration(trimmed); err != nil {
			return fmt.Errorf("plan timeout invalid: %w", err)
		}
	}
	if m.MaxParallel < 0 {
		return fmt.Errorf("max parallel cannot be negative")
	}
	return nil
}

// ModsHumanMetadata captures human checkpoint expectations.
type ModsHumanMetadata struct {
	Required  bool     `json:"required"`
	Playbooks []string `json:"playbooks,omitempty"`
}

// Validate ensures Mods human metadata contains valid playbooks.
func (m ModsHumanMetadata) Validate() error {
	for i, playbook := range m.Playbooks {
		if strings.TrimSpace(playbook) == "" {
			return fmt.Errorf("playbook %d is empty", i)
		}
	}
	return nil
}

// ModsRecommendation records a single recommendation entry.
type ModsRecommendation struct {
	Source     string  `json:"source,omitempty"`
	Message    string  `json:"message"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Validate ensures the recommendation message exists and confidence is bounded.
func (m ModsRecommendation) Validate() error {
	if strings.TrimSpace(m.Message) == "" {
		return fmt.Errorf("recommendation message is required")
	}
	if m.Confidence < 0 || m.Confidence > 1 {
		return fmt.Errorf("recommendation confidence must be within [0,1]")
	}
	return nil
}
