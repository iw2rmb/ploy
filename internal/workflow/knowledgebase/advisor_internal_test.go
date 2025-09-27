package knowledgebase

import "testing"

func TestIncidentToAdviceBoostsConfidence(t *testing.T) {
	incident := Incident{
		ID:        "test",
		Recipes:   []string{"recipe.alpha"},
		Summary:   "Summary",
		HumanGate: true,
		Playbooks: []string{"mods.playbook"},
		Recommendations: []Recommendation{
			{Source: "kb", Message: "keep", Confidence: 0.2},
			{Source: "kb", Message: " ", Confidence: 0.9},
		},
	}
	advice := incidentToAdvice(incident, 0.8, 1)
	if len(advice.Recommendations) != 1 {
		t.Fatalf("expected one recommendation after trimming blanks, got %#v", advice.Recommendations)
	}
	if advice.Recommendations[0].Confidence != 0.8 {
		t.Fatalf("expected confidence boosted to score, got %f", advice.Recommendations[0].Confidence)
	}
	if !advice.Plan.HumanGate || !advice.Human.Required {
		t.Fatalf("expected human gate flags propagated")
	}
}

func TestCosineSimilarityClamps(t *testing.T) {
	if value := cosineSimilarity(map[string]float64{"x": 2}, map[string]float64{"x": 2}, 1, 1); value != 1 {
		t.Fatalf("expected cosine similarity clamped to 1, got %f", value)
	}
	if value := cosineSimilarity(map[string]float64{"x": 2}, map[string]float64{"x": -2}, 1, 1); value != 0 {
		t.Fatalf("expected cosine similarity clamped to 0 for negative dot, got %f", value)
	}
}

func TestLevenshteinSimilarityBounds(t *testing.T) {
	if value := levenshteinSimilarity("same", "same"); value != 1 {
		t.Fatalf("expected identical strings to return similarity 1, got %f", value)
	}
	if value := levenshteinSimilarity("", "value"); value != 0 {
		t.Fatalf("expected empty comparison to yield 0 similarity, got %f", value)
	}
}

func TestExtractTrigramsShortInput(t *testing.T) {
	if grams := extractTrigrams("hi"); grams != nil {
		t.Fatalf("expected no trigrams for short text, got %#v", grams)
	}
}

func TestLoadCatalogFileEmptyPath(t *testing.T) {
	if _, err := LoadCatalogFile(" "); err == nil {
		t.Fatalf("expected empty path to error")
	}
}
