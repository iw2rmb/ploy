package models

import (
	"fmt"
	"sort"
	"time"
)

// Summary represents aggregated learning data for an error type
type Summary struct {
	ErrorID     string         `json:"error_id"`     // Links to Error signature
	CaseCount   int            `json:"case_count"`   // Number of cases seen
	SuccessRate float64        `json:"success_rate"` // Overall success rate
	TopPatches  []PatchSummary `json:"top_patches"`  // Most successful patches
	Updated     time.Time      `json:"updated"`
}

// PatchSummary represents aggregated data for a specific patch
type PatchSummary struct {
	Hash          string  `json:"hash"`           // Patch fingerprint
	SuccessRate   float64 `json:"success_rate"`   // Success rate for this patch
	AttemptCount  int     `json:"attempt_count"`  // Total attempts with this patch
	AvgConfidence float64 `json:"avg_confidence"` // Average confidence score
}

// CalculateSuccessRate calculates overall success rate from cases
func (s *Summary) CalculateSuccessRate(cases []Case) {
	if len(cases) == 0 {
		s.SuccessRate = 0.0
		s.CaseCount = 0
		s.Updated = time.Now()
		return
	}

	successCount := 0
	for _, c := range cases {
		if c.Success {
			successCount++
		}
	}

	s.SuccessRate = float64(successCount) / float64(len(cases))
	s.CaseCount = len(cases)
	s.Updated = time.Now()
}

// GenerateTopPatches generates top patch summaries from cases
func (s *Summary) GenerateTopPatches(cases []Case, limit int) {
	if len(cases) == 0 {
		s.TopPatches = []PatchSummary{}
		return
	}

	// Group cases by patch hash
	patchGroups := make(map[string][]Case)
	for _, c := range cases {
		patchGroups[c.PatchHash] = append(patchGroups[c.PatchHash], c)
	}

	// Calculate patch summaries
	var patchSummaries []PatchSummary
	for hash, patchCases := range patchGroups {
		successCount := 0
		totalConfidence := 0.0

		for _, pc := range patchCases {
			if pc.Success {
				successCount++
			}
			totalConfidence += pc.Confidence
		}

		summary := PatchSummary{
			Hash:          hash,
			SuccessRate:   float64(successCount) / float64(len(patchCases)),
			AttemptCount:  len(patchCases),
			AvgConfidence: totalConfidence / float64(len(patchCases)),
		}
		patchSummaries = append(patchSummaries, summary)
	}

	// Sort by success rate (descending)
	sort.Slice(patchSummaries, func(i, j int) bool {
		return patchSummaries[i].SuccessRate > patchSummaries[j].SuccessRate
	})

	// Limit to top patches
	if len(patchSummaries) > limit {
		patchSummaries = patchSummaries[:limit]
	}

	s.TopPatches = patchSummaries
}

// Validate checks if the summary is valid
func (s *Summary) Validate() error {
	if s.ErrorID == "" {
		return fmt.Errorf("error_id cannot be empty")
	}
	if s.SuccessRate < 0.0 || s.SuccessRate > 1.0 {
		return fmt.Errorf("success_rate must be between 0.0 and 1.0")
	}
	if s.CaseCount < 0 {
		return fmt.Errorf("case_count cannot be negative")
	}
	return nil
}

// Validate checks if the patch summary is valid
func (ps *PatchSummary) Validate() error {
	if ps.Hash == "" {
		return fmt.Errorf("hash cannot be empty")
	}
	if ps.SuccessRate < 0.0 || ps.SuccessRate > 1.0 {
		return fmt.Errorf("success_rate must be between 0.0 and 1.0")
	}
	return nil
}
