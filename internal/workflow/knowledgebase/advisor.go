package knowledgebase

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

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

type incidentVector struct {
	incident Incident
	vector   map[string]float64
	length   float64
	text     string
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

type scoredIncident struct {
	incident Incident
	score    float64
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

// bestMatch returns the highest scoring incident that clears the advisor score floor.
func (a Advisor) bestMatch(req mods.AdviceRequest) (Incident, float64, bool) {
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

// buildTFVector converts text into normalised trigram term frequencies.
func buildTFVector(text string) map[string]float64 {
	ngrams := extractTrigrams(text)
	if len(ngrams) == 0 {
		return map[string]float64{}
	}
	freq := make(map[string]float64)
	for _, gram := range ngrams {
		freq[gram]++
	}
	total := float64(len(ngrams))
	for gram, count := range freq {
		freq[gram] = count / total
	}
	return freq
}

// buildTFIDFVector converts text into a TF-IDF weighted vector.
func buildTFIDFVector(text string, idf map[string]float64) map[string]float64 {
	tf := buildTFVector(text)
	return tfidf(tf, idf)
}

// tfidf applies IDF weighting to a TF vector.
func tfidf(tf map[string]float64, idf map[string]float64) map[string]float64 {
	vector := make(map[string]float64, len(tf))
	for gram, tfValue := range tf {
		weight := idf[gram]
		if weight == 0 {
			weight = 1
		}
		vector[gram] = tfValue * weight
	}
	return vector
}

// extractTrigrams returns the rune-level trigram list for the provided text.
func extractTrigrams(text string) []string {
	trimmed := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(text)), "\n", " ")
	runes := []rune(trimmed)
	if len(runes) < 3 {
		return nil
	}
	ngrams := make([]string, 0, len(runes)-2)
	for i := 0; i+3 <= len(runes); i++ {
		grams := runes[i : i+3]
		ngrams = append(ngrams, string(grams))
	}
	return ngrams
}

// vectorLength computes the Euclidean length of a sparse vector.
func vectorLength(vector map[string]float64) float64 {
	var sum float64
	for _, value := range vector {
		sum += value * value
	}
	return math.Sqrt(sum)
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(a, b map[string]float64, lengthA, lengthB float64) float64 {
	if lengthA == 0 || lengthB == 0 {
		return 0
	}
	var dot float64
	for gram, weight := range a {
		if v, ok := b[gram]; ok {
			dot += weight * v
		}
	}
	result := dot / (lengthA * lengthB)
	if result < 0 {
		return 0
	}
	if result > 1 {
		return 1
	}
	return result
}

// levenshteinSimilarity returns a normalised Levenshtein similarity score.
func levenshteinSimilarity(a, b string) float64 {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return 0
	}
	da := utf8.RuneCountInString(a)
	db := utf8.RuneCountInString(b)
	maxLen := da
	if db > maxLen {
		maxLen = db
	}
	if maxLen == 0 {
		return 1
	}
	distance := levenshteinDistance([]rune(a), []rune(b))
	similarity := 1 - float64(distance)/float64(maxLen)
	if similarity < 0 {
		return 0
	}
	return similarity
}

// levenshteinDistance computes the Levenshtein edit distance between runes.
func levenshteinDistance(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	rows := len(a) + 1
	cols := len(b) + 1
	dp := make([]int, rows*cols)
	index := func(i, j int) int { return i*cols + j }
	for i := 0; i < rows; i++ {
		dp[index(i, 0)] = i
	}
	for j := 0; j < cols; j++ {
		dp[index(0, j)] = j
	}
	for i := 1; i < rows; i++ {
		for j := 1; j < cols; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			deletion := dp[index(i-1, j)] + 1
			insertion := dp[index(i, j-1)] + 1
			substitution := dp[index(i-1, j-1)] + cost
			dp[index(i, j)] = minInt(deletion, insertion, substitution)
		}
	}
	return dp[index(rows-1, cols-1)]
}

// minInt returns the smallest integer in the provided list.
func minInt(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values[1:] {
		if v < min {
			min = v
		}
	}
	return min
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
