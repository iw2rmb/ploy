package knowledgebase

import (
	"context"
	"fmt"

	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

// Options configures advisor scoring behaviour and catalog selection.
type Options struct {
	Catalog            Catalog
	MaxRecommendations int
	ScoreFloor         float64
}

// Advisor produces Mods planner guidance using catalogued incidents.
type Advisor struct {
	catalog            Catalog
	scoreFloor         float64
	maxRecommendations int
	idf                map[string]float64
	incidentVectors    []incidentVector
}

// MatchResult captures the top incident match discovered during evaluation.
type MatchResult struct {
	IncidentID string
	Score      float64
	Advice     mods.Advice
}

// NewAdvisor constructs an advisor from the provided options.
func NewAdvisor(opts Options) (Advisor, error) {
	if err := opts.Validate(); err != nil {
		return Advisor{}, err
	}
	scoreFloor := opts.ScoreFloor
	if scoreFloor < 0 {
		scoreFloor = 0
	}
	if scoreFloor > 1 {
		scoreFloor = 1
	}
	maxRecs := opts.MaxRecommendations
	if maxRecs <= 0 {
		maxRecs = 3
	}
	advisor := Advisor{
		catalog:            opts.Catalog,
		scoreFloor:         scoreFloor,
		maxRecommendations: maxRecs,
	}
	advisor.precompute()
	return advisor, nil
}

// Advise returns Mods planner recommendations for the supplied context.
func (a Advisor) Advise(ctx context.Context, req mods.AdviceRequest) (mods.Advice, error) {
	incident, score, ok := a.bestMatch(req)
	if !ok {
		return mods.Advice{}, nil
	}
	return incidentToAdvice(incident, score, a.maxRecommendations), nil
}

// Match returns the top incident match including score and advice payloads.
func (a Advisor) Match(ctx context.Context, req mods.AdviceRequest) (MatchResult, bool, error) {
	incident, score, ok := a.bestMatch(req)
	if !ok {
		return MatchResult{}, false, nil
	}
	advice := incidentToAdvice(incident, score, a.maxRecommendations)
	return MatchResult{IncidentID: incident.ID, Score: score, Advice: advice}, true, nil
}

// Validate ensures advisor option invariants hold.
func (o Options) Validate() error {
	if o.ScoreFloor < 0 || o.ScoreFloor > 1 {
		return fmt.Errorf("score floor must be between 0 and 1")
	}
	if o.MaxRecommendations < 0 {
		return fmt.Errorf("max recommendations cannot be negative")
	}
	return nil
}

var _ mods.Advisor = Advisor{}
