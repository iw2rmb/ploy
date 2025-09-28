package knowledgebase

import (
	"strings"
	"unicode/utf8"
)

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
