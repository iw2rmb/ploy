package knowledgebase

import (
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

// incidentToAdvice maps a catalog incident into the Mods advice structure.
func incidentToAdvice(incident Incident, score float64, maxRecs int) mods.Advice {
	planRecipes := trimStrings(incident.Recipes)
	planSummary := strings.TrimSpace(incident.Summary)
	recs := make([]mods.AdviceRecommendation, 0, len(incident.Recommendations))
	for _, rec := range incident.Recommendations {
		message := strings.TrimSpace(rec.Message)
		if message == "" {
			continue
		}
		confidence := rec.Confidence
		if confidence < score {
			confidence = score
		}
		if confidence > 1 {
			confidence = 1
		}
		recs = append(recs, mods.AdviceRecommendation{
			Source:     rec.Source,
			Message:    message,
			Confidence: confidence,
		})
	}
	if len(recs) > maxRecs {
		recs = recs[:maxRecs]
	}
	return mods.Advice{
		Plan: mods.AdvicePlan{
			SelectedRecipes: planRecipes,
			HumanGate:       incident.HumanGate,
			Summary:         planSummary,
		},
		Human: mods.AdviceHuman{
			Required:  incident.HumanGate,
			Playbooks: trimStrings(incident.Playbooks),
		},
		Recommendations: recs,
	}
}

// aggregateIncident normalises an incident's error text into a single buffer.
func aggregateIncident(incident Incident) string {
	parts := make([]string, 0, len(incident.Errors))
	for _, err := range incident.Errors {
		trimmed := strings.TrimSpace(err)
		if trimmed != "" {
			parts = append(parts, strings.ToLower(trimmed))
		}
	}
	return strings.Join(parts, " ")
}

// aggregateSignals joins advisor signals into a classification string.
func aggregateSignals(signals mods.AdviceSignals) string {
	parts := make([]string, 0, len(signals.Errors)+2)
	for _, err := range signals.Errors {
		trimmed := strings.TrimSpace(err)
		if trimmed != "" {
			parts = append(parts, strings.ToLower(trimmed))
		}
	}
	if signals.Manifest.Name != "" {
		parts = append(parts, strings.ToLower(strings.TrimSpace(signals.Manifest.Name)))
	}
	if signals.Manifest.Version != "" {
		parts = append(parts, strings.ToLower(strings.TrimSpace(signals.Manifest.Version)))
	}
	return strings.Join(parts, " ")
}
