//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type ModWorkflow struct {
	ID              string
	Repository      string
	TargetBranch    string
	Steps           []WorkflowStep
	SelfHeal        SelfHealConfig
	ExpectedOutcome Outcome
	MaxDuration     time.Duration
}

type WorkflowStep struct {
	Type    string
	ID      string
	Engine  string
	Recipes []string
	// OpenRewrite explicit coordinates (for orw-apply)
	RecipeGroup        string
	RecipeArtifact     string
	RecipeVersion      string
	MavenPluginVersion string
}

type SelfHealConfig struct {
	Enabled    bool
	MaxRetries int
	KBLearning bool
}

type Outcome int

const (
	OutcomeSuccess Outcome = iota
	OutcomeHealedSuccess
	OutcomeFailure
)

type WorkflowResult struct {
	ID                  string
	Duration            time.Duration
	Success             bool
	Output              string
	Error               string
	ModID               string
	ConfigPath          string
	ConfigYAML          string
	WorkflowBranch      string
	BuildVersion        string
	MRUrl               string
	InitialBuildSuccess bool
	HealingAttempted    bool
	HealingAttempts     []HealingAttempt
	ResourceUsage       *ResourceStats
}

type HealingAttempt struct {
	ErrorSignature string
	Success        bool
	Patch          string
	Confidence     float64
}

type ResourceStats struct {
	MaxMemoryMB int
	CPUPercent  float64
}

func (w *ModWorkflow) ToYAML() (string, error) {
	yaml := fmt.Sprintf(`version: v1alpha1
id: %s
target_repo: %s
target_branch: %s
base_ref: %s
lane: C
build_timeout: 10m

steps:
`, w.ID, w.Repository, w.TargetBranch, w.TargetBranch)

	for _, step := range w.Steps {
		if step.Type == "orw-apply" {
			yaml += fmt.Sprintf(`  - type: orw-apply
    id: %s
    recipes:
`, step.ID)
			for _, recipe := range step.Recipes {
				yaml += fmt.Sprintf(`      - %s
`, recipe)
			}
			if step.RecipeGroup != "" {
				yaml += fmt.Sprintf("    recipe_group: %s\n", step.RecipeGroup)
			}
			if step.RecipeArtifact != "" {
				yaml += fmt.Sprintf("    recipe_artifact: %s\n", step.RecipeArtifact)
			}
			if step.RecipeVersion != "" {
				yaml += fmt.Sprintf("    recipe_version: %s\n", step.RecipeVersion)
			}
			if step.MavenPluginVersion != "" {
				yaml += fmt.Sprintf("    maven_plugin_version: %s\n", step.MavenPluginVersion)
			}
		} else {
			// Legacy recipe step (engine-based) retained for backwards compatibility
			yaml += fmt.Sprintf(`  - type: %s
    id: %s
    engine: %s
    recipes:
`, step.Type, step.ID, step.Engine)
			for _, recipe := range step.Recipes {
				yaml += fmt.Sprintf(`      - %s
`, recipe)
			}
		}
	}

	if w.SelfHeal.Enabled {
		yaml += fmt.Sprintf(`
self_heal:
  enabled: true
  kb_learning: %t
  max_retries: %d
  cooldown: 30s
`, w.SelfHeal.KBLearning, w.SelfHeal.MaxRetries)
	}

	return yaml, nil
}

func (r *WorkflowResult) parseFromOutput(output string) {
	// Parse mod_id from controller output variants
	// 1) JSON-like: mod_id: "<id>"
	execJSON := regexp.MustCompile(`(?i)mod_id\s*[:=]\s*\"([a-zA-Z0-9\-]+)\"`)
	if m := execJSON.FindStringSubmatch(output); len(m) > 1 {
		r.ModID = m[1]
	}
	// 2) Human line: Execution ID: <id>
	if r.ModID == "" {
		execLine := regexp.MustCompile(`(?i)execution\s+id\s*[:=]\s*([a-zA-Z0-9\-]+)`)
		if m := execLine.FindStringSubmatch(output); len(m) > 1 {
			r.ModID = m[1]
		}
	}
	// Parse workflow branch
	branchRegex := regexp.MustCompile(`(?i)branch:\s+([^\s]+)`)
	if matches := branchRegex.FindStringSubmatch(output); len(matches) > 1 {
		r.WorkflowBranch = matches[1]
	}

	// Parse build version
	versionRegex := regexp.MustCompile(`(?i)build.version:\s+([^\s]+)`)
	if matches := versionRegex.FindStringSubmatch(output); len(matches) > 1 {
		r.BuildVersion = matches[1]
	}

	// Parse MR URL
	mrRegex := regexp.MustCompile(`(?i)merge.request:\s+(https://[^\s]+)`)
	if matches := mrRegex.FindStringSubmatch(output); len(matches) > 1 {
		r.MRUrl = matches[1]
	}

	// Parse healing attempts
	r.HealingAttempted = strings.Contains(strings.ToLower(output), "healing")
	r.InitialBuildSuccess = !strings.Contains(strings.ToLower(output), "build failed")

	// Parse healing details
	if strings.Contains(strings.ToLower(output), "healing attempt") {
		attempts := parseHealingAttempts(output)
		r.HealingAttempts = attempts
	}
}
