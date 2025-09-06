package transflow

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type TransflowStep struct {
	Type        string     `yaml:"type"`
	ID          string     `yaml:"id"`
	Engine      string     `yaml:"engine"`
	Recipes     []string   `yaml:"recipes"`
	Model       string     `yaml:"model,omitempty"`
	Prompts     []string   `yaml:"prompts,omitempty"`
	MCPTools    []MCPTool  `yaml:"mcp_tools,omitempty"`
	Context     []string   `yaml:"context,omitempty"`
	Budgets     MCPBudgets `yaml:"budgets,omitempty"`
	Parallel    bool       `yaml:"parallel,omitempty"`
	MaxParallel int        `yaml:"max_parallel_execs,omitempty"`
}

type TransflowConfig struct {
	Version      string          `yaml:"version"`
	ID           string          `yaml:"id"`
	TargetRepo   string          `yaml:"target_repo"`
	TargetBranch string          `yaml:"target_branch"`
	BaseRef      string          `yaml:"base_ref"`
	Lane         string          `yaml:"lane"`
	BuildTimeout string          `yaml:"build_timeout"`
	Steps        []TransflowStep `yaml:"steps"`
	SelfHeal     *SelfHealConfig `yaml:"self_heal"`
}

func LoadConfig(path string) (*TransflowConfig, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg TransflowConfig
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}

	// Set default self-heal config if not provided or apply defaults to existing config
	if cfg.SelfHeal == nil {
		cfg.SelfHeal = GetDefaultSelfHealConfig()
	} else {
		// Apply defaults for missing fields
		if cfg.SelfHeal.MaxRetries == 0 {
			cfg.SelfHeal.MaxRetries = GetDefaultSelfHealConfig().MaxRetries
		}
		// Cooldown defaults to empty string, so no need to set it
		// For enabled field, explicit false should remain false, no default needed
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *TransflowConfig) Validate() error {
	if c.ID == "" {
		return errors.New("id is required")
	}
	if c.TargetRepo == "" {
		return errors.New("target_repo is required")
	}
	if c.BaseRef == "" {
		return errors.New("base_ref is required")
	}
	if len(c.Steps) == 0 {
		return errors.New("at least one step is required")
	}

	// Validate each step
	for i, step := range c.Steps {
		if step.ID == "" {
			return fmt.Errorf("step %d must have an id", i)
		}
		if step.Type == "" {
			return fmt.Errorf("step %d (%s) must have a type", i, step.ID)
		}

		// Validate MCP configuration for steps that have it
		if len(step.MCPTools) > 0 || len(step.Context) > 0 {
			mcpConfig := MCPConfig{
				Tools:   step.MCPTools,
				Context: step.Context,
				Budgets: step.Budgets,
				Model:   step.Model,
				Prompts: step.Prompts,
			}
			if err := mcpConfig.Validate(); err != nil {
				return fmt.Errorf("invalid MCP config for step %s: %w", step.ID, err)
			}
		}
	}

	// Validate self-heal configuration if provided
	if c.SelfHeal != nil {
		if err := c.SelfHeal.Validate(); err != nil {
			return fmt.Errorf("invalid self_heal config: %w", err)
		}
	}

	// Validate build timeout if provided
	if c.BuildTimeout != "" {
		if _, err := c.ParseBuildTimeout(); err != nil {
			return fmt.Errorf("invalid build_timeout: %w", err)
		}
	}

	return nil
}

func (c *TransflowConfig) ParseBuildTimeout() (time.Duration, error) {
	if c.BuildTimeout == "" {
		return 10 * time.Minute, nil // default
	}

	duration, err := time.ParseDuration(c.BuildTimeout)
	if err != nil {
		return 0, fmt.Errorf("invalid build timeout format: %v", err)
	}

	if duration < 0 {
		return 0, errors.New("build timeout cannot be negative")
	}

	return duration, nil
}

func GenerateAppName(id string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("tfw-%s-%d", id, timestamp)
}

func GenerateBranchName(id string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("workflow/%s/%s", id, strconv.FormatInt(timestamp, 10))
}
