package knowledgebase

import (
	"math"
	"strings"
)

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
