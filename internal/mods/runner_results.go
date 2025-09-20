package mods

import (
	"fmt"
	"strings"
	"time"
)

// ReportDiff captures diff metadata for reporting purposes.
type ReportDiff struct {
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

// ReportReference links a step to supporting artifacts (diffs, plans, prompts, etc.).
type ReportReference struct {
	Kind  string `json:"kind,omitempty"`
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
}

// StepReportMeta stores optional metadata used when constructing reports.
type StepReportMeta struct {
	Type        string            `json:"type,omitempty"`
	Prompts     []string          `json:"prompts,omitempty"`
	Recipes     []RecipeEntry     `json:"recipes,omitempty"`
	Diff        *ReportDiff       `json:"diff,omitempty"`
	References  []ReportReference `json:"references,omitempty"`
	ErrorSolved string            `json:"error_solved,omitempty"`
}

// StepResult represents the result of executing a single step
type StepResult struct {
	StepID   string
	Success  bool
	Message  string
	Duration time.Duration
	Report   *StepReportMeta
}

// ModResult represents the overall result of a Mod execution
type ModResult struct {
	Success        bool
	WorkflowID     string
	BranchName     string
	CommitSHA      string
	BuildVersion   string
	StepResults    []StepResult
	ErrorMessage   string
	Duration       time.Duration
	HealingSummary *ModHealingSummary
	MRURL          string // GitLab merge request URL if created
	StartedAt      time.Time
	FinishedAt     time.Time
}

// ReportStep captures an entry in the happy path timeline.
type ReportStep struct {
	ID          string        `json:"id"`
	Type        string        `json:"type,omitempty"`
	Message     string        `json:"message,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	Prompts     []string      `json:"prompts,omitempty"`
	Recipes     []RecipeEntry `json:"recipes,omitempty"`
	Diff        *ReportDiff   `json:"diff,omitempty"`
	ErrorSolved string        `json:"error_solved,omitempty"`
}

// ReportStepNode is used to represent the complete step tree (successes and failures).
type ReportStepNode struct {
	ID          string            `json:"id"`
	Type        string            `json:"type,omitempty"`
	Success     bool              `json:"success"`
	Message     string            `json:"message,omitempty"`
	Duration    time.Duration     `json:"duration,omitempty"`
	References  []ReportReference `json:"references,omitempty"`
	Prompts     []string          `json:"prompts,omitempty"`
	Recipes     []RecipeEntry     `json:"recipes,omitempty"`
	Children    []ReportStepNode  `json:"children,omitempty"`
	ErrorSolved string            `json:"error_solved,omitempty"`
}

// ModReport is the canonical Mods execution report structure exposed via the API.
type ModReport struct {
	RepoName   string           `json:"repo_name"`
	WorkflowID string           `json:"workflow_id"`
	BranchName string           `json:"branch_name,omitempty"`
	MRURL      string           `json:"mr_url,omitempty"`
	StartedAt  time.Time        `json:"started_at"`
	EndedAt    time.Time        `json:"ended_at"`
	Duration   time.Duration    `json:"duration"`
	HappyPath  []ReportStep     `json:"happy_path"`
	StepTree   []ReportStepNode `json:"step_tree"`
}

// Summary returns a human-readable summary of the Mods execution
func (r *ModResult) Summary() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Workflow: %s\n", r.WorkflowID))

	if r.Success {
		sb.WriteString("Status: SUCCESS\n")
	} else {
		sb.WriteString("Status: FAILED\n")
	}

	if r.BranchName != "" {
		sb.WriteString(fmt.Sprintf("Branch: %s\n", r.BranchName))
	}

	if r.CommitSHA != "" {
		sb.WriteString(fmt.Sprintf("Commit: %s\n", r.CommitSHA))
	}

	if r.BuildVersion != "" {
		sb.WriteString(fmt.Sprintf("Build: %s\n", r.BuildVersion))
	}

	if !r.Success && r.ErrorMessage != "" {
		sb.WriteString(fmt.Sprintf("Error: %s\n", r.ErrorMessage))
	}

	sb.WriteString("Steps:\n")
	for _, step := range r.StepResults {
		status := "✓"
		if !step.Success {
			status = "✗"
		}
		sb.WriteString(fmt.Sprintf("  %s %s: %s\n", status, step.StepID, step.Message))
	}

	// Include healing summary if self-healing was enabled
	if r.HealingSummary != nil && r.HealingSummary.Enabled {
		sb.WriteString("\nSelf-Healing:\n")
		if r.HealingSummary.AttemptsCount > 0 {
			sb.WriteString(fmt.Sprintf("  Attempts: %d/%d\n", r.HealingSummary.AttemptsCount, r.HealingSummary.MaxRetries))
			sb.WriteString(fmt.Sprintf("  Successful fixes: %d\n", r.HealingSummary.TotalHealed))
			sb.WriteString(fmt.Sprintf("  Final result: %s\n", map[bool]string{true: "SUCCESS", false: "FAILED"}[r.HealingSummary.FinalSuccess]))

			for _, attempt := range r.HealingSummary.Attempts {
				status := "✗"
				if attempt.Success {
					status = "✓"
				}
				sb.WriteString(fmt.Sprintf("    %s Attempt %d: %s\n", status, attempt.AttemptNumber,
					func() string {
						if attempt.Success {
							return fmt.Sprintf("Applied %d recipe(s)", len(attempt.AppliedRecipes))
						}
						return attempt.ErrorMessage
					}()))
			}
		} else {
			sb.WriteString("  No healing attempts made\n")
		}
	}

	// Include MR URL if available
	if r.MRURL != "" {
		sb.WriteString(fmt.Sprintf("\nMerge Request: %s\n", r.MRURL))
	}

	return sb.String()
}
