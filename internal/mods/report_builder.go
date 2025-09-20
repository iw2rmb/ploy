package mods

import (
	"fmt"
	"strings"
	"time"
)

const maxDiffPreviewBytes = 200 * 1024 // 200KB safeguard for inline diff content

// BuildModReport constructs a ModReport from configuration and run results.
func BuildModReport(cfg *ModConfig, result *ModResult) ModReport {
	if result == nil {
		return ModReport{}
	}

	repo := ""
	if cfg != nil {
		repo = cfg.TargetRepo
	}

	report := ModReport{
		RepoName:   repo,
		WorkflowID: result.WorkflowID,
		BranchName: result.BranchName,
		MRURL:      result.MRURL,
		StartedAt:  result.StartedAt,
		EndedAt:    result.FinishedAt,
		Duration:   result.Duration,
	}

	if report.EndedAt.IsZero() && !report.StartedAt.IsZero() && result.Duration > 0 {
		report.EndedAt = report.StartedAt.Add(result.Duration)
	}

	stepTypeByID := map[string]string{}
	if cfg != nil {
		for _, step := range cfg.Steps {
			stepTypeByID[step.ID] = step.Type
		}
	}

	for _, sr := range result.StepResults {
		meta := sr.Report
		stepType := inferStepTypeForReport(sr.StepID, meta, stepTypeByID)

		node := ReportStepNode{
			ID:          sr.StepID,
			Type:        stepType,
			Success:     sr.Success,
			Message:     sr.Message,
			Duration:    sr.Duration,
			Prompts:     copyStrings(metaPrompts(meta)),
			Recipes:     copyRecipes(metaRecipes(meta)),
			References:  normalizeReferences(meta),
			ErrorSolved: metaErrorSolved(meta),
		}
		report.StepTree = append(report.StepTree, node)

		if sr.Success {
			happy := ReportStep{
				ID:          sr.StepID,
				Type:        stepType,
				Message:     sr.Message,
				Duration:    sr.Duration,
				Prompts:     copyStrings(metaPrompts(meta)),
				Recipes:     copyRecipes(metaRecipes(meta)),
				Diff:        normalizeDiff(meta),
				ErrorSolved: metaErrorSolved(meta),
			}
			report.HappyPath = append(report.HappyPath, happy)
		}
	}

	return report
}

func inferStepTypeForReport(stepID string, meta *StepReportMeta, lookup map[string]string) string {
	if meta != nil && meta.Type != "" {
		return meta.Type
	}
	if t, ok := lookup[stepID]; ok && t != "" {
		return t
	}
	return stepID
}

func metaPrompts(meta *StepReportMeta) []string {
	if meta == nil {
		return nil
	}
	return meta.Prompts
}

func metaRecipes(meta *StepReportMeta) []RecipeEntry {
	if meta == nil {
		return nil
	}
	return meta.Recipes
}

func metaErrorSolved(meta *StepReportMeta) string {
	if meta == nil {
		return ""
	}
	return meta.ErrorSolved
}

func normalizeReferences(meta *StepReportMeta) []ReportReference {
	if meta == nil {
		return nil
	}
	refs := append([]ReportReference(nil), meta.References...)
	if meta.Diff != nil && meta.Diff.Path != "" {
		hasDiff := false
		for _, r := range refs {
			if r.Kind == "diff" && r.Value == meta.Diff.Path {
				hasDiff = true
				break
			}
		}
		if !hasDiff {
			refs = append(refs, ReportReference{Kind: "diff", Label: "diff", Value: meta.Diff.Path})
		}
	}
	return refs
}

func normalizeDiff(meta *StepReportMeta) *ReportDiff {
	if meta == nil || meta.Diff == nil {
		return nil
	}
	if meta.Diff.Content == "" {
		return meta.Diff
	}
	if len(meta.Diff.Content) > maxDiffPreviewBytes {
		trimmed := meta.Diff.Content[:maxDiffPreviewBytes]
		return &ReportDiff{Path: meta.Diff.Path, Content: trimmed + "\n... (diff truncated)"}
	}
	return meta.Diff
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copyRecipes(in []RecipeEntry) []RecipeEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]RecipeEntry, len(in))
	copy(out, in)
	return out
}

// RenderModReportMarkdown renders a ModReport into a Markdown document.
func RenderModReportMarkdown(report ModReport) string {
	var sb strings.Builder

	title := report.WorkflowID
	if title == "" {
		title = "mods-run"
	}
	sb.WriteString(fmt.Sprintf("# Mods Report: %s\n\n", title))

	sb.WriteString("## Summary\n")
	sb.WriteString(fmt.Sprintf("- Repo: %s\n", orFallback(report.RepoName, "(unknown)")))
	sb.WriteString(fmt.Sprintf("- Branch: %s\n", orFallback(report.BranchName, "(not created)")))
	if report.MRURL != "" {
		sb.WriteString(fmt.Sprintf("- Merge Request: %s\n", report.MRURL))
	} else {
		sb.WriteString("- Merge Request: (not created)\n")
	}
	if !report.StartedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("- Started: %s\n", report.StartedAt.Format(time.RFC3339)))
	}
	if !report.EndedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("- Ended: %s\n", report.EndedAt.Format(time.RFC3339)))
	}
	if report.Duration > 0 {
		sb.WriteString(fmt.Sprintf("- Duration: %s\n", report.Duration))
	}

	sb.WriteString("\n## Happy Path\n")
	if len(report.HappyPath) == 0 {
		sb.WriteString("(no successful steps recorded)\n")
	} else {
		for idx, step := range report.HappyPath {
			status := "success"
			sb.WriteString(fmt.Sprintf("%d. [%s] %s", idx+1, status, step.ID))
			if step.Type != "" && step.Type != step.ID {
				sb.WriteString(fmt.Sprintf(" (%s)", step.Type))
			}
			sb.WriteString("\n")
			if step.Message != "" {
				sb.WriteString(fmt.Sprintf("   - Message: %s\n", step.Message))
			}
			if len(step.Prompts) > 0 {
				sb.WriteString(fmt.Sprintf("   - Prompts: %s\n", strings.Join(step.Prompts, "; ")))
			}
			if len(step.Recipes) > 0 {
				sb.WriteString("   - Recipes:\n")
				for _, recipe := range step.Recipes {
					sb.WriteString(fmt.Sprintf("     * %s", recipe.Name))
					if recipe.Coords.Group != "" {
						sb.WriteString(fmt.Sprintf(" (%s:%s@%s)", recipe.Coords.Group, recipe.Coords.Artifact, recipe.Coords.Version))
					}
					sb.WriteString("\n")
				}
			}
			if step.ErrorSolved != "" {
				sb.WriteString(fmt.Sprintf("   - Addressed Error: %s\n", step.ErrorSolved))
			}
			if step.Diff != nil && step.Diff.Content != "" {
				sb.WriteString("\n```diff\n")
				sb.WriteString(step.Diff.Content)
				if !strings.HasSuffix(step.Diff.Content, "\n") {
					sb.WriteString("\n")
				}
				sb.WriteString("```\n")
			}
		}
	}

	sb.WriteString("\n## Step Tree\n")
	if len(report.StepTree) == 0 {
		sb.WriteString("(no steps recorded)\n")
	} else {
		for _, node := range report.StepTree {
			writeStepNodeMarkdown(&sb, node, 0)
		}
	}

	return sb.String()
}

func writeStepNodeMarkdown(sb *strings.Builder, node ReportStepNode, indent int) {
	prefix := strings.Repeat("  ", indent)
	status := "success"
	if !node.Success {
		status = "failed"
	}
	fmt.Fprintf(sb, "%s- [%s] %s", prefix, status, node.ID)
	if node.Type != "" && node.Type != node.ID {
		fmt.Fprintf(sb, " (%s)", node.Type)
	}
	if node.Message != "" {
		fmt.Fprintf(sb, " — %s", node.Message)
	}
	sb.WriteString("\n")

	if node.ErrorSolved != "" {
		fmt.Fprintf(sb, "%s  • Addressed Error: %s\n", prefix, node.ErrorSolved)
	}
	if len(node.Prompts) > 0 {
		fmt.Fprintf(sb, "%s  • Prompts: %s\n", prefix, strings.Join(node.Prompts, "; "))
	}
	if len(node.Recipes) > 0 {
		fmt.Fprintf(sb, "%s  • Recipes:\n", prefix)
		for _, recipe := range node.Recipes {
			fmt.Fprintf(sb, "%s    - %s", prefix, recipe.Name)
			if recipe.Coords.Group != "" {
				fmt.Fprintf(sb, " (%s:%s@%s)", recipe.Coords.Group, recipe.Coords.Artifact, recipe.Coords.Version)
			}
			sb.WriteString("\n")
		}
	}
	if len(node.References) > 0 {
		fmt.Fprintf(sb, "%s  • References:\n", prefix)
		for _, ref := range node.References {
			label := ref.Label
			if label == "" {
				label = ref.Kind
			}
			fmt.Fprintf(sb, "%s    - %s: %s\n", prefix, label, ref.Value)
		}
	}

	for _, child := range node.Children {
		writeStepNodeMarkdown(sb, child, indent+1)
	}
}

func orFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
