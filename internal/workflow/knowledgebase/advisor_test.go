package knowledgebase_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/knowledgebase"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

func TestAdvisorReturnsTopIncidentAdvice(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{
		"schema_version": "2025-09-27.1",
		"incidents": [
			{
				"id": "npm-missing-start",
				"errors": [
					"npm ERR! Missing script: start",
					"npm ERR! A complete log of this run can be found"
				],
				"recipes": ["recipe.npm.scripts"],
				"summary": "Add a start script to package.json",
				"human_gate": true,
				"playbooks": ["mods.npm.playbook"],
				"recommendations": [
					{
						"source": "knowledge-base",
						"message": "Add \"start\" script to package.json",
						"confidence": 0.95
					}
				]
			},
			{
				"id": "go-module-missing",
				"errors": [
					"go: module example.com/missing provides package",
					"build failed"
				],
				"recipes": ["recipe.go.tidy"],
				"summary": "Run go mod tidy to restore missing module",
				"human_gate": false,
				"playbooks": ["mods.go.playbook"],
				"recommendations": [
					{
						"source": "knowledge-base",
						"message": "Run go mod tidy to restore the module",
						"confidence": 0.88
					}
				]
			}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	catalog, err := knowledgebase.LoadCatalogFile(catalogPath)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	advisor, err := knowledgebase.NewAdvisor(knowledgebase.Options{
		Catalog:            catalog,
		MaxRecommendations: 3,
		ScoreFloor:         0.3,
	})
	if err != nil {
		t.Fatalf("build advisor: %v", err)
	}

	advice, err := advisor.Advise(context.Background(), mods.AdviceRequest{
		Ticket: contracts.WorkflowTicket{
			SchemaVersion: contracts.SchemaVersion,
			TicketID:      "TICKET-123",
			Tenant:        "acme",
			Manifest: contracts.ManifestReference{
				Name:    "repo",
				Version: "1.0.0",
			},
		},
		Signals: mods.AdviceSignals{
			Errors: []string{
				"npm ERR! Missing script: start",
				"npm ERR! "},
			Manifest: contracts.ManifestReference{Name: "repo", Version: "1.0.0"},
		},
	})
	if err != nil {
		t.Fatalf("advise: %v", err)
	}

	if len(advice.Plan.SelectedRecipes) == 0 || advice.Plan.SelectedRecipes[0] != "recipe.npm.scripts" {
		t.Fatalf("expected npm recipe recommendation, got %#v", advice.Plan.SelectedRecipes)
	}
	if !advice.Plan.HumanGate {
		t.Fatalf("expected plan human gate recommendation true due to incident data")
	}
	if !advice.Human.Required {
		t.Fatalf("expected human gate recommendation to flag manual review")
	}
	if len(advice.Human.Playbooks) == 0 || advice.Human.Playbooks[0] != "mods.npm.playbook" {
		t.Fatalf("expected human playbook propagated, got %#v", advice.Human.Playbooks)
	}
	if len(advice.Recommendations) == 0 {
		t.Fatalf("expected recommendations present")
	}
	if advice.Recommendations[0].Confidence <= 0 || advice.Recommendations[0].Confidence > 1 {
		t.Fatalf("expected confidence normalised between 0 and 1, got %f", advice.Recommendations[0].Confidence)
	}
}

func TestAdvisorGracefullyHandlesEmptyCatalog(t *testing.T) {
	advisor, err := knowledgebase.NewAdvisor(knowledgebase.Options{})
	if err != nil {
		t.Fatalf("build advisor with empty options: %v", err)
	}
	advice, err := advisor.Advise(context.Background(), mods.AdviceRequest{Signals: mods.AdviceSignals{Errors: []string{"unknown failure"}}})
	if err != nil {
		t.Fatalf("advise empty catalog: %v", err)
	}
	if advice.Plan.SelectedRecipes != nil {
		t.Fatalf("expected no recipes when catalog empty, got %#v", advice.Plan.SelectedRecipes)
	}
	if advice.Human.Playbooks != nil {
		t.Fatalf("expected no playbooks when catalog empty, got %#v", advice.Human.Playbooks)
	}
	if len(advice.Recommendations) != 0 {
		t.Fatalf("expected no recommendations, got %#v", advice.Recommendations)
	}
}

func TestAdvisorHonoursScoreFloor(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{
		"schema_version": "2025-09-27.1",
		"incidents": [
			{
				"id": "lint-failure",
				"errors": ["lint failed"],
				"recipes": ["recipe.npm.lint"],
				"summary": "Run npm run lint",
				"human_gate": false,
				"playbooks": ["mods.npm.lint"],
				"recommendations": [
					{"source": "knowledge-base", "message": "Run npm run lint -- --fix", "confidence": 0.6}
				]
			}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	catalog, err := knowledgebase.LoadCatalogFile(catalogPath)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	advisor, err := knowledgebase.NewAdvisor(knowledgebase.Options{Catalog: catalog, ScoreFloor: 0.9})
	if err != nil {
		t.Fatalf("new advisor: %v", err)
	}
	advice, err := advisor.Advise(context.Background(), mods.AdviceRequest{Signals: mods.AdviceSignals{Errors: []string{"completely unrelated error"}}})
	if err != nil {
		t.Fatalf("advise score floor: %v", err)
	}
	if advice.Plan.SelectedRecipes != nil {
		t.Fatalf("expected no plan recommendations above score floor, got %#v", advice.Plan.SelectedRecipes)
	}
	if len(advice.Recommendations) != 0 {
		t.Fatalf("expected no recommendations when score floor filters results, got %#v", advice.Recommendations)
	}
}

func TestLoadCatalogFileMissing(t *testing.T) {
	_, err := knowledgebase.LoadCatalogFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatalf("expected error when catalog missing")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestOptionsValidateBounds(t *testing.T) {
	if err := (knowledgebase.Options{ScoreFloor: -0.1}).Validate(); err == nil {
		t.Fatalf("expected negative score floor to error")
	}
	if err := (knowledgebase.Options{ScoreFloor: 0.5, MaxRecommendations: -1}).Validate(); err == nil {
		t.Fatalf("expected negative max recommendations to error")
	}
}
