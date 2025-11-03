package knowledgebase

import (
	"math"

	plan "github.com/iw2rmb/ploy/internal/workflow/mods/plan"
)

type incidentVector struct {
	incident Incident
	vector   map[string]float64
	length   float64
	text     string
}

type scoredIncident struct {
	incident Incident
	score    float64
}

// precompute builds TF-IDF vectors for catalog incidents ahead of scoring.
func (a *Advisor) precompute() {
	idf := make(map[string]float64)
	vectors := make([]incidentVector, 0, len(a.catalog.Incidents))
	for _, incident := range a.catalog.Incidents {
		text := aggregateIncident(incident)
		vector := buildTFVector(text)
		vectors = append(vectors, incidentVector{incident: incident, vector: vector, text: text})
		for ngram := range vector {
			idf[ngram]++
		}
	}
	numDocs := float64(len(vectors))
	for ngram, df := range idf {
		idf[ngram] = math.Log((1+numDocs)/(1+df)) + 1
	}
	for i := range vectors {
		vec := tfidf(vectors[i].vector, idf)
		vectors[i].vector = vec
		vectors[i].length = vectorLength(vec)
	}
	a.idf = idf
	a.incidentVectors = vectors
}

// bestMatch returns the highest scoring incident that clears the advisor score floor.
func (a Advisor) bestMatch(req plan.AdviceRequest) (Incident, float64, bool) {
	if len(a.incidentVectors) == 0 {
		return Incident{}, 0, false
	}
	signals := req.Signals
	queryText := aggregateSignals(signals)
	queryVector := buildTFIDFVector(queryText, a.idf)
	queryLength := vectorLength(queryVector)
	var best scoredIncident
	hasBest := false
	for _, candidate := range a.incidentVectors {
		cosine := cosineSimilarity(queryVector, candidate.vector, queryLength, candidate.length)
		lev := levenshteinSimilarity(queryText, candidate.text)
		score := 0.7*cosine + 0.3*lev
		if score < a.scoreFloor {
			continue
		}
		if !hasBest || score > best.score || (score == best.score && candidate.incident.ID < best.incident.ID) {
			best = scoredIncident{incident: candidate.incident, score: score}
			hasBest = true
		}
	}
	if !hasBest {
		return Incident{}, 0, false
	}
	return best.incident, best.score, true
}
