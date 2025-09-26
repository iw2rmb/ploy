package runner

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

// TestConvertStageMetadataClampsAndTrims verifies planner metadata conversion trims values and clamps confidence.
func TestConvertStageMetadataClampsAndTrims(t *testing.T) {
	meta := mods.StageMetadata{
		Mods: &mods.StageModsMetadata{
			Plan: &mods.StageModsPlan{
				SelectedRecipes: []string{"  recipe.alpha  "},
				ParallelStages:  []string{"  orw-apply  ", "orw-gen"},
				HumanGate:       true,
				Summary:         "  summary  ",
			},
			Human: &mods.StageModsHuman{
				Required:  true,
				Playbooks: []string{"  playbook.mods.review  "},
			},
			Recommendations: []mods.StageModsRecommendation{
				{Source: "kb", Message: "apply", Confidence: 1.4},
				{Source: "kb", Message: " ", Confidence: -0.5},
			},
		},
	}

	converted := convertStageMetadata(meta)
	if converted.Mods == nil {
		t.Fatalf("expected mods metadata to be present")
	}
	if converted.Mods.Plan == nil || len(converted.Mods.Plan.SelectedRecipes) != 1 || converted.Mods.Plan.SelectedRecipes[0] != "recipe.alpha" {
		t.Fatalf("expected trimmed plan metadata, got %#v", converted.Mods.Plan)
	}
	if len(converted.Mods.Plan.ParallelStages) != 2 || converted.Mods.Plan.ParallelStages[0] != "orw-apply" {
		t.Fatalf("expected parallel stages trimmed, got %#v", converted.Mods.Plan.ParallelStages)
	}
	if converted.Mods.Plan.Summary != "summary" {
		t.Fatalf("expected summary trimmed, got %q", converted.Mods.Plan.Summary)
	}
	if converted.Mods.Human == nil || len(converted.Mods.Human.Playbooks) != 1 || converted.Mods.Human.Playbooks[0] != "playbook.mods.review" {
		t.Fatalf("expected trimmed human metadata, got %#v", converted.Mods.Human)
	}
	if len(converted.Mods.Recommendations) != 1 {
		t.Fatalf("expected blank recommendation filtered, got %#v", converted.Mods.Recommendations)
	}
	if converted.Mods.Recommendations[0].Confidence != 1 {
		t.Fatalf("expected confidence clamped to 1, got %#v", converted.Mods.Recommendations[0].Confidence)
	}
}

// TestBuildCheckpointModsMetadataNil ensures empty metadata returns nil.
func TestBuildCheckpointModsMetadataNil(t *testing.T) {
	if buildCheckpointModsMetadata(nil) != nil {
		t.Fatal("expected nil metadata when input is nil")
	}
	empty := &StageModsMetadata{}
	if buildCheckpointModsMetadata(empty) != nil {
		t.Fatal("expected nil metadata when stage mods are empty")
	}
}
