package mods

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// SummaryComputer handles KB summary computation and promotion logic
type SummaryComputer struct {
	storage KBStorage
	lockMgr KBLockManager
	config  *SummaryConfig
}

// SummaryConfig contains configuration for summary computation
type SummaryConfig struct {
	MinCasesForPromotion  int     // Minimum cases needed to promote a fix
	MinSuccessRate        float64 // Minimum success rate for promotion
	MaxPromotedFixes      int     // Maximum fixes to keep in promoted list
	RecencyWeight         float64 // Weight for recent successes (0.0-1.0)
	FrequencyWeight       float64 // Weight for frequency (0.0-1.0)
	SuccessRateWeight     float64 // Weight for success rate (0.0-1.0)
	MinScore              float64 // Minimum score threshold for promotion
	PromotionLookbackDays int     // Days to look back for recency scoring
}

// DefaultSummaryConfig returns reasonable defaults for summary computation
func DefaultSummaryConfig() *SummaryConfig {
	return &SummaryConfig{
		MinCasesForPromotion:  3,
		MinSuccessRate:        0.6,
		MaxPromotedFixes:      10,
		RecencyWeight:         0.3,
		FrequencyWeight:       0.4,
		SuccessRateWeight:     0.3,
		MinScore:              0.5,
		PromotionLookbackDays: 90,
	}
}

// NewSummaryComputer creates a new summary computer
func NewSummaryComputer(storage KBStorage, lockMgr KBLockManager, config *SummaryConfig) *SummaryComputer {
	if config == nil {
		config = DefaultSummaryConfig()
	}
	return &SummaryComputer{
		storage: storage,
		lockMgr: lockMgr,
		config:  config,
	}
}

// FixCandidate represents a potential fix for promotion
type FixCandidate struct {
	Kind          string
	Ref           string
	Wins          int
	Failures      int
	TotalCases    int
	SuccessRate   float64
	LastSuccessAt time.Time
	FirstSeenAt   time.Time
	Score         float64
}

// ComputeAndUpdateSummary processes cases and updates the summary for an error signature
func (sc *SummaryComputer) ComputeAndUpdateSummary(ctx context.Context, lang, signature string) error {
	lockKey := BuildSignatureLockKey(lang, signature)
	lockConfig := DefaultLockConfig()
	lockConfig.DefaultTTL = 10 * time.Second // Longer TTL for summary computation

	return sc.lockMgr.TryWithLockRetry(ctx, lockKey, lockConfig, func() error {
		return sc.computeAndUpdateSummaryLocked(ctx, lang, signature)
	})
}

// computeAndUpdateSummaryLocked performs the actual summary computation under lock
func (sc *SummaryComputer) computeAndUpdateSummaryLocked(ctx context.Context, lang, signature string) error {
	// Read all cases for this signature
	cases, err := sc.storage.ReadCases(ctx, lang, signature)
	if err != nil {
		return fmt.Errorf("failed to read cases: %w", err)
	}

	if len(cases) == 0 {
		return nil // No cases to process
	}

	// Analyze cases and identify fix candidates
	candidates := sc.analyzeCases(cases)

	// Score and rank candidates
	scoredCandidates := sc.scoreCandidates(candidates)

	// Promote top candidates
	promoted := sc.promoteTopCandidates(scoredCandidates)

	// Compute aggregate statistics
	stats := sc.computeStats(cases)

	// Create summary record
	summary := &SummaryRecord{
		Language:  lang,
		Signature: signature,
		Promoted:  promoted,
		Stats:     stats,
	}

	// Store the updated summary
	err = sc.storage.WriteSummary(ctx, lang, signature, summary)
	if err != nil {
		return fmt.Errorf("failed to write summary: %w", err)
	}

	return nil
}

// analyzeCases examines cases to identify potential fixes
func (sc *SummaryComputer) analyzeCases(cases []*CaseRecord) map[string]*FixCandidate {
	candidates := make(map[string]*FixCandidate)

	for _, caseRecord := range cases {
		if caseRecord.Attempt == nil || caseRecord.Outcome == nil {
			continue
		}

		var key, kind, ref string

		// Identify the fix type and reference
		switch caseRecord.Attempt.Type {
		case "orw_recipe":
			if caseRecord.Attempt.Recipe != "" {
				kind = "orw_recipe"
				ref = caseRecord.Attempt.Recipe
				key = "recipe:" + ref
			}
		case "llm_patch":
			if caseRecord.Attempt.PatchFingerprint != "" {
				kind = "patch_fingerprint"
				ref = caseRecord.Attempt.PatchFingerprint
				key = "patch:" + ref
			}
		case "human_step":
			// Human steps are typically not promoted as they're manual
			continue
		}

		if key == "" {
			continue // Skip invalid attempts
		}

		// Get or create candidate
		candidate, exists := candidates[key]
		if !exists {
			candidate = &FixCandidate{
				Kind:        kind,
				Ref:         ref,
				FirstSeenAt: caseRecord.Timestamp,
			}
			candidates[key] = candidate
		}

		// Update candidate statistics
		candidate.TotalCases++

		if caseRecord.Outcome.Success {
			candidate.Wins++
			if candidate.LastSuccessAt.IsZero() || caseRecord.Timestamp.After(candidate.LastSuccessAt) {
				candidate.LastSuccessAt = caseRecord.Timestamp
			}
		} else {
			candidate.Failures++
		}

		// Track earliest sighting
		if caseRecord.Timestamp.Before(candidate.FirstSeenAt) {
			candidate.FirstSeenAt = caseRecord.Timestamp
		}
	}

	// Compute success rates
	for _, candidate := range candidates {
		if candidate.TotalCases > 0 {
			candidate.SuccessRate = float64(candidate.Wins) / float64(candidate.TotalCases)
		}
	}

	return candidates
}

// scoreCandidates assigns scores to fix candidates based on multiple factors
func (sc *SummaryComputer) scoreCandidates(candidates map[string]*FixCandidate) []*FixCandidate {
	now := time.Now()
	lookbackTime := now.AddDate(0, 0, -sc.config.PromotionLookbackDays)

	var scoredCandidates []*FixCandidate

	for _, candidate := range candidates {
		// Apply minimum thresholds
		if candidate.TotalCases < sc.config.MinCasesForPromotion {
			continue
		}
		if candidate.SuccessRate < sc.config.MinSuccessRate {
			continue
		}

		// Calculate score components
		successRateScore := candidate.SuccessRate

		// Frequency score (normalized by total cases, capped at 1.0)
		frequencyScore := math.Min(1.0, float64(candidate.Wins)/10.0)

		// Recency score (based on last success time)
		var recencyScore float64
		if !candidate.LastSuccessAt.IsZero() {
			if candidate.LastSuccessAt.After(lookbackTime) {
				// More recent successes score higher
				daysSinceSuccess := float64(now.Sub(candidate.LastSuccessAt).Hours()) / 24.0
				lookbackDays := float64(sc.config.PromotionLookbackDays)
				recencyScore = math.Max(0.0, (lookbackDays-daysSinceSuccess)/lookbackDays)
			}
		}

		// Weighted composite score
		candidate.Score = (sc.config.SuccessRateWeight * successRateScore) +
			(sc.config.FrequencyWeight * frequencyScore) +
			(sc.config.RecencyWeight * recencyScore)

		// Apply minimum score threshold
		if candidate.Score >= sc.config.MinScore {
			scoredCandidates = append(scoredCandidates, candidate)
		}
	}

	// Sort by score (highest first)
	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].Score > scoredCandidates[j].Score
	})

	return scoredCandidates
}

// promoteTopCandidates selects the best candidates for promotion
func (sc *SummaryComputer) promoteTopCandidates(scoredCandidates []*FixCandidate) []PromotedFix {
	maxPromoted := sc.config.MaxPromotedFixes
	if len(scoredCandidates) < maxPromoted {
		maxPromoted = len(scoredCandidates)
	}

	var promoted []PromotedFix
	for i := 0; i < maxPromoted; i++ {
		candidate := scoredCandidates[i]

		fix := PromotedFix{
			Kind:          candidate.Kind,
			Ref:           candidate.Ref,
			Score:         candidate.Score,
			Wins:          candidate.Wins,
			Failures:      candidate.Failures,
			LastSuccessAt: candidate.LastSuccessAt,
			FirstSeenAt:   candidate.FirstSeenAt,
		}

		promoted = append(promoted, fix)
	}

	return promoted
}

// computeStats calculates aggregate statistics for cases
func (sc *SummaryComputer) computeStats(cases []*CaseRecord) *SummaryStats {
	stats := &SummaryStats{
		TotalCases:  len(cases),
		LastUpdated: time.Now(),
	}

	var totalDuration int64
	var validDurations int

	for _, caseRecord := range cases {
		if caseRecord.Outcome == nil {
			continue
		}

		if caseRecord.Outcome.Success {
			stats.SuccessCount++
		} else {
			stats.FailureCount++
		}

		// Accumulate durations for average calculation
		if caseRecord.Outcome.Duration > 0 {
			totalDuration += caseRecord.Outcome.Duration
			validDurations++
		}
	}

	// Calculate success rate
	if stats.TotalCases > 0 {
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.TotalCases)
	}

	// Calculate average duration
	if validDurations > 0 {
		stats.AvgDuration = totalDuration / int64(validDurations)
	}

	return stats
}

// GetRecommendedFixes retrieves recommended fixes for an error signature
func (sc *SummaryComputer) GetRecommendedFixes(ctx context.Context, lang, signature string, maxResults int) ([]PromotedFix, error) {
	summary, err := sc.storage.ReadSummary(ctx, lang, signature)
	if err != nil {
		return nil, fmt.Errorf("failed to read summary: %w", err)
	}

	// Return top N promoted fixes
	results := summary.Promoted
	if len(results) > maxResults && maxResults > 0 {
		results = results[:maxResults]
	}

	return results, nil
}

// UpdateSummaryAfterCase is a convenience method to update summary after adding a new case
func (sc *SummaryComputer) UpdateSummaryAfterCase(ctx context.Context, lang, signature string) error {
	// Try to update summary, but don't fail the operation if locking fails
	// The compactor job will rebuild summaries periodically anyway
	lockKey := BuildSignatureLockKey(lang, signature)
	lockConfig := DefaultLockConfig()
	lockConfig.MaxRetries = 1 // Single attempt
	lockConfig.DefaultTTL = 5 * time.Second

	err := sc.lockMgr.TryWithLockRetry(ctx, lockKey, lockConfig, func() error {
		return sc.computeAndUpdateSummaryLocked(ctx, lang, signature)
	})

	// If we can't acquire the lock, that's okay - just return success
	// The compactor will rebuild this later
	if err != nil && err.Error() == "lock already held by another session" {
		return nil
	}

	return err
}

// CompactionStats tracks compaction run statistics
type CompactionStats struct {
	ProcessedSignatures int
	UpdatedSummaries    int
	RemovedStaleData    int
	ErrorCount          int
	Duration            time.Duration
	StartTime           time.Time
}

// RunCompaction performs a full KB compaction and maintenance operation
func (sc *SummaryComputer) RunCompaction(ctx context.Context) (*CompactionStats, error) {
	stats := &CompactionStats{
		StartTime: time.Now(),
	}
	defer func() {
		stats.Duration = time.Since(stats.StartTime)
	}()

	// This would be implemented as a more comprehensive operation
	// that scans all signatures and rebuilds summaries
	// For MVP, we'll implement a basic version

	// TODO: Implement full compaction logic
	// - Scan all language/signature combinations
	// - Rebuild summaries for each
	// - Clean up stale data
	// - Update snapshot manifest

	return stats, nil
}
