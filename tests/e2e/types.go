package e2e

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type TransflowWorkflow struct {
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

func (w *TransflowWorkflow) ToYAML() (string, error) {
	yaml := fmt.Sprintf(`version: v1alpha1
id: %s
target_repo: %s
target_branch: refs/heads/%s
base_ref: refs/heads/%s
lane: C
build_timeout: 10m

steps:
`, w.ID, w.Repository, w.TargetBranch, w.TargetBranch)

	for _, step := range w.Steps {
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
