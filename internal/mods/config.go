package mods

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"strings"

	"gopkg.in/yaml.v3"
)

type RecipeCoordinates struct {
	Group    string `yaml:"group"`
	Artifact string `yaml:"artifact"`
	Version  string `yaml:"version"`
}

type RecipeEntry struct {
	Name   string            `yaml:"name"`
	Coords RecipeCoordinates `yaml:"coords"`
}

func (r *RecipeEntry) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		r.Name = strings.TrimSpace(value.Value)
		return nil
	}
	type raw struct {
		Name   string            `yaml:"name"`
		Coords RecipeCoordinates `yaml:"coords"`
	}
	var aux raw
	if err := value.Decode(&aux); err != nil {
		return err
	}
	r.Name = strings.TrimSpace(aux.Name)
	r.Coords = aux.Coords
	return nil
}

type ModStep struct {
	Type               string        `yaml:"type"`
	ID                 string        `yaml:"id"`
	Engine             string        `yaml:"engine"`
	Recipes            []RecipeEntry `yaml:"recipes"`
	MavenPluginVersion string        `yaml:"maven_plugin_version,omitempty"`
	DiscoverRecipe     *bool         `yaml:"discover_recipe,omitempty"`
	Model              string        `yaml:"model,omitempty"`
	Prompts            []string      `yaml:"prompts,omitempty"`
	MCPTools           []MCPTool     `yaml:"mcp_tools,omitempty"`
	Context            []string      `yaml:"context,omitempty"`
	Budgets            MCPBudgets    `yaml:"budgets,omitempty"`
	Parallel           bool          `yaml:"parallel,omitempty"`
	MaxParallel        int           `yaml:"max_parallel_execs,omitempty"`
}

type ModConfig struct {
	Version      string          `yaml:"version"`
	ID           string          `yaml:"id"`
	TargetRepo   string          `yaml:"target_repo"`
	TargetBranch string          `yaml:"target_branch"`
	BaseRef      string          `yaml:"base_ref"`
	Lane         string          `yaml:"lane"`
	BuildTimeout string          `yaml:"build_timeout"`
	Steps        []ModStep       `yaml:"steps"`
	SelfHeal     *SelfHealConfig `yaml:"self_heal"`
	SBOM         *SBOMConfig     `yaml:"sbom,omitempty"`
	Security     *SecurityConfig `yaml:"security,omitempty"`
	MR           *MRConfigYAML   `yaml:"mr,omitempty"`
}

// MRConfigYAML allows selecting which environment variables to use for
// Git provider configuration without embedding secrets in YAML.
// Example:
// mr:
//
//	forge: gitlab
//	repo_url_env: GITLAB_URL
//	token_env: GITLAB_TOKEN
//	labels: ["ploy", "tfl", "healing-llm"]
type MRConfigYAML struct {
	Forge      string   `yaml:"forge,omitempty"`
	RepoURLEnv string   `yaml:"repo_url_env,omitempty"`
	TokenEnv   string   `yaml:"token_env,omitempty"`
	Labels     []string `yaml:"labels,omitempty"`
}

// SBOMConfig controls SBOM behavior for this Mods run
type SBOMConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Types       []string `yaml:"types,omitempty"`         // source|artifact|container (currently only source used controller-side)
	FailOnError bool     `yaml:"fail_on_error,omitempty"` // when true, controller-side SBOM errors fail the run
}

// SecurityConfig controls optional vulnerability scanning based on SBOM
type SecurityConfig struct {
	Enabled        bool   `yaml:"enabled,omitempty"`
	MinSeverity    string `yaml:"min_severity,omitempty"`     // low|medium|high|critical
	FailOnFindings bool   `yaml:"fail_on_findings,omitempty"` // when true, fail run if findings >= MinSeverity
}

func LoadConfig(path string) (*ModConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ModConfig
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

		// Apply default SBOM config
		if cfg.SBOM == nil {
			cfg.SBOM = &SBOMConfig{Enabled: true, Types: []string{"source"}, FailOnError: false}
		} else {
			if len(cfg.SBOM.Types) == 0 {
				cfg.SBOM.Types = []string{"source"}
			}
		}
		// Security defaults
		if cfg.Security == nil {
			cfg.Security = &SecurityConfig{Enabled: false, MinSeverity: "high", FailOnFindings: true}
		} else {
			if cfg.Security.MinSeverity == "" {
				cfg.Security.MinSeverity = "high"
			}
		}
		// Cooldown defaults to empty string, so no need to set it
		// For enabled field, explicit false should remain false, no default needed
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *ModConfig) Validate() error {
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
		for idx, recipe := range step.Recipes {
			if strings.TrimSpace(recipe.Name) == "" {
				return fmt.Errorf("step %s recipe[%d] must provide name", step.ID, idx)
			}
			if strings.EqualFold(step.Type, string(StepTypeORWApply)) {
				if recipe.Coords.Group == "" || recipe.Coords.Artifact == "" || recipe.Coords.Version == "" {
					return fmt.Errorf("step %s recipe %q must define coords.group/artifact/version", step.ID, recipe.Name)
				}
			}
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

	// Validate SBOM configuration
	if c.SBOM != nil {
		allowed := map[string]bool{"source": true, "artifact": true, "container": true}
		for _, t := range c.SBOM.Types {
			if !allowed[strings.ToLower(t)] {
				return fmt.Errorf("invalid sbom type: %s (allowed: source, artifact, container)", t)
			}
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

func (c *ModConfig) ParseBuildTimeout() (time.Duration, error) {
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
	return fmt.Sprintf("mod-%s-%d", id, timestamp)
}

func GenerateBranchName(id string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("workflow/%s/%s", id, strconv.FormatInt(timestamp, 10))
}

// PreferredModel returns the first non-empty model declared in steps, if any.
// This allows mods.yaml to specify the LLM model used by planner/llm-exec flows.
func (c *ModConfig) PreferredModel() string {
	for _, s := range c.Steps {
		if strings.TrimSpace(s.Model) != "" {
			return strings.TrimSpace(s.Model)
		}
	}
	return ""
}
