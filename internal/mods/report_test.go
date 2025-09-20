package mods

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildModReportHappyPathIncludesDiffs(t *testing.T) {
	start := time.Date(2025, 1, 2, 15, 4, 5, 0, time.UTC)
	end := start.Add(7 * time.Minute)

	recipes := []RecipeEntry{{
		Name:   "org.openrewrite.java.cleanup",
		Coords: RecipeCoordinates{Group: "org.openrewrite", Artifact: "rewrite-cleanup", Version: "1.6.0"},
	}}

	cfg := &ModConfig{
		ID:         "mod-report-test",
		TargetRepo: "https://git.example.com/org/service.git",
		BaseRef:    "main",
		Steps: []ModStep{{
			ID:      "orw-main",
			Type:    string(StepTypeORWApply),
			Prompts: []string{"Resolve compile error in scheduler"},
			Recipes: recipes,
		}},
	}

	diffContent := "diff --git a/app.java b/app.java\n+public void fixed() {}\n"

	result := &ModResult{
		Success:    true,
		WorkflowID: cfg.ID,
		BranchName: "mods/mod-report-test",
		MRURL:      "https://git.example.com/org/service/merge_requests/42",
		StartedAt:  start,
		FinishedAt: end,
		Duration:   end.Sub(start),
		StepResults: []StepResult{
			{
				StepID:   "clone",
				Success:  true,
				Message:  "Cloned repository",
				Duration: 2 * time.Second,
				Report: &StepReportMeta{
					Type: "system",
				},
			},
			{
				StepID:   "orw-main",
				Success:  true,
				Message:  "Applied ORW diff",
				Duration: 3 * time.Minute,
				Report: &StepReportMeta{
					Type:    string(StepTypeORWApply),
					Prompts: []string{"Resolve compile error in scheduler"},
					Recipes: recipes,
					Diff:    &ReportDiff{Path: "workspace/orw-main/diff.patch", Content: diffContent},
					References: []ReportReference{{
						Kind:  "diff",
						Label: "diff.patch",
						Value: "workspace/orw-main/diff.patch",
					}},
				},
			},
			{
				StepID:   "build",
				Success:  true,
				Message:  "Build completed successfully",
				Duration: 90 * time.Second,
				Report:   &StepReportMeta{Type: "build"},
			},
		},
	}

	report := BuildModReport(cfg, result)

	if report.RepoName != cfg.TargetRepo {
		t.Fatalf("expected repo name %q, got %q", cfg.TargetRepo, report.RepoName)
	}
	if report.MRURL != result.MRURL {
		t.Fatalf("expected MR URL %q, got %q", result.MRURL, report.MRURL)
	}
	if !report.StartedAt.Equal(start) || !report.EndedAt.Equal(end) {
		t.Fatalf("expected start/end to match run timestamps")
	}
	if len(report.HappyPath) != 3 {
		t.Fatalf("expected 3 happy-path steps, got %d", len(report.HappyPath))
	}

	applyStep := report.HappyPath[1]
	if applyStep.Type != string(StepTypeORWApply) {
		t.Fatalf("expected happy path step type %q, got %q", StepTypeORWApply, applyStep.Type)
	}
	if len(applyStep.Prompts) != 1 || applyStep.Prompts[0] != "Resolve compile error in scheduler" {
		t.Fatalf("expected prompt to be captured, got %#v", applyStep.Prompts)
	}
	if applyStep.Diff == nil || !strings.Contains(applyStep.Diff.Content, "+public void fixed() {}") {
		t.Fatalf("expected diff content to be captured, got %+v", applyStep.Diff)
	}

	var hasApplyNode bool
	for _, node := range report.StepTree {
		if node.ID == "orw-main" {
			hasApplyNode = true
			foundDiff := false
			for _, ref := range node.References {
				if ref.Kind == "diff" {
					foundDiff = true
					break
				}
			}
			if !foundDiff {
				t.Fatalf("expected diff reference on apply node, got %+v", node.References)
			}
		}
	}
	if !hasApplyNode {
		t.Fatalf("expected apply step to be present in step tree")
	}
}

func TestBuildModReportCapturesFailedStepsInTree(t *testing.T) {
	cfg := &ModConfig{
		ID:         "failed-mod",
		TargetRepo: "git@example.com/failed.git",
		BaseRef:    "main",
	}

	result := &ModResult{
		WorkflowID: cfg.ID,
		StartedAt:  time.Now().Add(-5 * time.Minute).UTC(),
		FinishedAt: time.Now().UTC(),
		Duration:   5 * time.Minute,
		Success:    false,
		StepResults: []StepResult{
			{
				StepID:  "build",
				Success: false,
				Message: "Build check failed: compile error",
				Report:  &StepReportMeta{Type: "build", ErrorSolved: "compile error"},
			},
		},
	}

	report := BuildModReport(cfg, result)

	if len(report.StepTree) == 0 {
		t.Fatalf("expected step tree entries for failed run")
	}

	n := report.StepTree[0]
	if n.Success {
		t.Fatalf("expected failed step in tree to be marked unsuccessful")
	}
	if !strings.Contains(n.Message, "compile error") {
		t.Fatalf("expected error message to be preserved, got %q", n.Message)
	}
}

func TestBuildModReportIncludesLLMBranchDiff(t *testing.T) {
	start := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Minute)
	diffDir := t.TempDir()
	diffPath := filepath.Join(diffDir, "diff.patch")
	if err := os.WriteFile(diffPath, []byte("diff --git a/file b/file\n+llm-change\n"), 0644); err != nil {
		t.Fatalf("write diff: %v", err)
	}

	result := &ModResult{
		WorkflowID: "healing-mod",
		StartedAt:  start,
		FinishedAt: end,
		Duration:   end.Sub(start),
		HealingSummary: &ModHealingSummary{
			AllResults: []BranchResult{{
				ID:       "option-1",
				Type:     string(StepTypeLLMExec),
				Status:   "completed",
				JobID:    "llm-exec-option-1",
				Notes:    "LLM exec job completed successfully, diff.patch at: " + diffPath,
				Duration: 30 * time.Second,
				DiffPath: diffPath,
				DiffKey:  "mods/mod-healing/branches/option-1/steps/llm-exec-option-1/diff.patch",
			}},
		},
	}

	report := BuildModReport(&ModConfig{}, result)

	found := false
	for _, node := range report.StepTree {
		if node.ID == "option-1" {
			found = true
			if node.Type != string(StepTypeLLMExec) {
				t.Fatalf("expected llm-exec type, got %s", node.Type)
			}
			if len(node.References) == 0 ||
				node.References[0].Value != "mods/mod-healing/branches/option-1/steps/llm-exec-option-1/diff.patch" {
				t.Fatalf("expected diff reference with key, got %+v", node.References)
			}
		}
	}
	if !found {
		t.Fatalf("expected llm branch node in step tree")
	}

	var llmStep *ReportStep
	for _, step := range report.HappyPath {
		if step.ID == "option-1" {
			llmStep = &step
			break
		}
	}
	if llmStep == nil {
		t.Fatalf("expected llm branch in happy path")
	}
	if llmStep.Diff == nil || !strings.Contains(llmStep.Diff.Content, "+llm-change") {
		t.Fatalf("expected llm diff content, got %+v", llmStep.Diff)
	}
}

func TestRenderModReportMarkdownContainsDiffAndTree(t *testing.T) {
	report := ModReport{
		RepoName:   "https://git.example.com/org/service.git",
		WorkflowID: "report-md",
		MRURL:      "https://git.example.com/org/service/merge_requests/42",
		StartedAt:  time.Date(2025, 1, 2, 15, 4, 5, 0, time.UTC),
		EndedAt:    time.Date(2025, 1, 2, 15, 14, 5, 0, time.UTC),
		Duration:   10 * time.Minute,
		HappyPath: []ReportStep{{
			ID:      "clone",
			Type:    "system",
			Message: "Cloned repository",
		}, {
			ID:      "apply",
			Type:    string(StepTypeORWApply),
			Message: "Applied ORW diff",
			Diff:    &ReportDiff{Content: "diff --git a/file b/file\n+change"},
		}},
		StepTree: []ReportStepNode{{
			ID:      "clone",
			Type:    "system",
			Success: true,
			Message: "Cloned repository",
		}, {
			ID:      "apply",
			Type:    string(StepTypeORWApply),
			Success: true,
			Message: "Applied ORW diff",
		}},
	}

	markdown := RenderModReportMarkdown(report)

	if !strings.Contains(markdown, "## Summary") {
		t.Fatalf("expected markdown summary section, got:\n%s", markdown)
	}
	if !strings.Contains(markdown, "```diff") {
		t.Fatalf("expected diff fence in markdown output, got:\n%s", markdown)
	}
	if !strings.Contains(markdown, "- [success] apply") {
		t.Fatalf("expected step tree bullets in markdown output, got:\n%s", markdown)
	}
	if _, err := url.ParseRequestURI(report.MRURL); err == nil && !strings.Contains(markdown, report.MRURL) {
		t.Fatalf("expected MR URL to be present in markdown output")
	}
}
