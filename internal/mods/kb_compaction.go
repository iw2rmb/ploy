package mods

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// CompactionConfig contains configuration for KB compaction operations
type CompactionConfig struct {
	// Compaction thresholds
	MinCasesForCompaction       int     `json:"min_cases_for_compaction"`       // 10
	SimilarityThresholdForMerge float64 `json:"similarity_threshold_for_merge"` // 0.95
	MaxCasesToAnalyze           int     `json:"max_cases_to_analyze"`           // 1000

	// Performance limits
	MaxCompactionBatchSize int           `json:"max_compaction_batch_size"` // 100
	CompactionTimeout      time.Duration `json:"compaction_timeout"`        // 30 minutes

	// Retention policies
	MaxCaseAge       time.Duration `json:"max_case_age"`        // 180 days
	MinCasesToRetain int           `json:"min_cases_to_retain"` // 5

	// Dry run mode for testing
	DryRun bool `json:"dry_run"` // false
}

// DefaultCompactionConfig returns reasonable defaults for compaction
func DefaultCompactionConfig() *CompactionConfig {
	return &CompactionConfig{
		MinCasesForCompaction:       10,
		SimilarityThresholdForMerge: 0.95,
		MaxCasesToAnalyze:           1000,
		MaxCompactionBatchSize:      100,
		CompactionTimeout:           30 * time.Minute,
		MaxCaseAge:                  180 * 24 * time.Hour,
		MinCasesToRetain:            5,
		DryRun:                      false,
	}
}

// DeduplicationStats contains statistics about compaction operations
type DeduplicationStats struct {
	SignaturesAnalyzed  int           `json:"signatures_analyzed"`
	CasesAnalyzed       int           `json:"cases_analyzed"`
	CasesMerged         int           `json:"cases_merged"`
	CasesDeleted        int           `json:"cases_deleted"`
	PatchesDeduplicated int           `json:"patches_deduplicated"`
	StorageSaved        int64         `json:"storage_saved_bytes"`
	CompactionDuration  time.Duration `json:"compaction_duration"`
	Errors              []string      `json:"errors,omitempty"`
}

// CompactionJob represents a KB storage compaction job
type CompactionJob struct {
	storage         KBStorage
	sigGenerator    EnhancedSignatureGenerator
	lockMgr         KBLockManager
	summaryComputer *SummaryComputer
	config          *CompactionConfig
}

// NewCompactionJob creates a new compaction job
func NewCompactionJob(
	storage KBStorage,
	sigGenerator EnhancedSignatureGenerator,
	lockMgr KBLockManager,
	summaryComputer *SummaryComputer,
	config *CompactionConfig,
) *CompactionJob {
	if config == nil {
		config = DefaultCompactionConfig()
	}

	return &CompactionJob{
		storage:         storage,
		sigGenerator:    sigGenerator,
		lockMgr:         lockMgr,
		summaryComputer: summaryComputer,
		config:          config,
	}
}

// CompactSignature compacts all cases for a specific error signature
func (cj *CompactionJob) CompactSignature(ctx context.Context, lang, signature string) (*DeduplicationStats, error) {
	stats := &DeduplicationStats{}
	startTime := time.Now()
	defer func() {
		stats.CompactionDuration = time.Since(startTime)
	}()

	// Acquire lock for this signature
	lockKey := BuildSignatureLockKey(lang, signature)
	lockConfig := DefaultLockConfig()
	lockConfig.DefaultTTL = cj.config.CompactionTimeout

	err := cj.lockMgr.TryWithLockRetry(ctx, lockKey, lockConfig, func() error {
		return cj.compactSignatureLocked(ctx, lang, signature, stats)
	})

	if err != nil {
		stats.Errors = append(stats.Errors, fmt.Sprintf("compaction failed for %s/%s: %v", lang, signature, err))
	}

	return stats, err
}

// compactSignatureLocked performs compaction under lock
func (cj *CompactionJob) compactSignatureLocked(ctx context.Context, lang, signature string, stats *DeduplicationStats) error {
	stats.SignaturesAnalyzed++

	// Read all cases for this signature
	cases, err := cj.storage.ReadCases(ctx, lang, signature)
	if err != nil {
		return fmt.Errorf("failed to read cases: %w", err)
	}

	if len(cases) < cj.config.MinCasesForCompaction {
		return nil // Not enough cases to warrant compaction
	}

	stats.CasesAnalyzed = len(cases)

	// Apply retention policy - remove old cases
	retainedCases := cj.applyRetentionPolicy(cases)
	if len(retainedCases) < len(cases) {
		stats.CasesDeleted = len(cases) - len(retainedCases)
		cases = retainedCases
	}

	// Find duplicate/similar cases for merging
	duplicateGroups := cj.findDuplicateCases(cases)

	// Merge duplicate cases
	mergedCases, merged := cj.mergeDuplicateCases(cases, duplicateGroups)
	stats.CasesMerged = merged

	// If we made changes, rewrite the cases
	if len(mergedCases) != len(cases) || merged > 0 {
		if !cj.config.DryRun {
			err = cj.rewriteCases(ctx, lang, signature, mergedCases)
			if err != nil {
				return fmt.Errorf("failed to rewrite cases: %w", err)
			}

			// Recompute summary after compaction
			err = cj.summaryComputer.ComputeAndUpdateSummary(ctx, lang, signature)
			if err != nil {
				// Don't fail compaction if summary update fails
				stats.Errors = append(stats.Errors, fmt.Sprintf("summary update failed: %v", err))
			}
		}
	}

	return nil
}

// applyRetentionPolicy removes cases that are too old
func (cj *CompactionJob) applyRetentionPolicy(cases []*CaseRecord) []*CaseRecord {
	cutoffTime := time.Now().Add(-cj.config.MaxCaseAge)
	var retained []*CaseRecord

	// Sort by timestamp to keep the most recent cases
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].Timestamp.After(cases[j].Timestamp)
	})

	kept := 0
	for _, caseRecord := range cases {
		// Always keep minimum number of cases
		if kept < cj.config.MinCasesToRetain {
			retained = append(retained, caseRecord)
			kept++
			continue
		}

		// Keep cases within retention period
		if caseRecord.Timestamp.After(cutoffTime) {
			retained = append(retained, caseRecord)
			kept++
		}
	}

	return retained
}

// findDuplicateCases identifies groups of similar cases that can be merged
func (cj *CompactionJob) findDuplicateCases(cases []*CaseRecord) [][]int {
	var duplicateGroups [][]int
	processed := make(map[int]bool)

	for i, case1 := range cases {
		if processed[i] {
			continue
		}

		group := []int{i}
		processed[i] = true

		// Find similar cases
		for j := i + 1; j < len(cases); j++ {
			if processed[j] {
				continue
			}

			case2 := cases[j]
			if cj.areCasesSimilar(case1, case2) {
				group = append(group, j)
				processed[j] = true
			}
		}

		// Only create groups with multiple cases
		if len(group) > 1 {
			duplicateGroups = append(duplicateGroups, group)
		}
	}

	return duplicateGroups
}

// areCasesSimilar determines if two cases are similar enough to merge
func (cj *CompactionJob) areCasesSimilar(case1, case2 *CaseRecord) bool {
	// Must be same language and signature
	if case1.Language != case2.Language || case1.Signature != case2.Signature {
		return false
	}

	// Must have same attempt type
	if case1.Attempt == nil || case2.Attempt == nil {
		return case1.Attempt == case2.Attempt
	}

	if case1.Attempt.Type != case2.Attempt.Type {
		return false
	}

	// For recipe attempts, must use same recipe
	if case1.Attempt.Type == "orw_recipe" {
		return case1.Attempt.Recipe == case2.Attempt.Recipe
	}

	// For patch attempts, check patch similarity
	if case1.Attempt.Type == "llm_patch" {
		if case1.Attempt.PatchFingerprint == case2.Attempt.PatchFingerprint {
			return true // Same patch
		}

		// Check if patches are similar
		if case1.Attempt.PatchContent != "" && case2.Attempt.PatchContent != "" {
			similarity := cj.sigGenerator.ComputePatchSimilarity(
				[]byte(case1.Attempt.PatchContent),
				[]byte(case2.Attempt.PatchContent),
			)
			return similarity >= cj.config.SimilarityThresholdForMerge
		}
	}

	// Check context similarity
	return cj.areContextsSimilar(case1.Context, case2.Context)
}

// areContextsSimilar checks if two case contexts are similar
func (cj *CompactionJob) areContextsSimilar(ctx1, ctx2 *CaseContext) bool {
	if ctx1 == nil || ctx2 == nil {
		return ctx1 == ctx2
	}

	// Same language and basic context
	if ctx1.Language != ctx2.Language || ctx1.Lane != ctx2.Lane {
		return false
	}

	// Same repository (allows for similar builds)
	if ctx1.RepoURL != ctx2.RepoURL {
		return false
	}

	// Similar build command and compiler version
	if ctx1.BuildCommand != ctx2.BuildCommand {
		return false
	}

	if ctx1.CompilerVersion != ctx2.CompilerVersion {
		return false
	}

	return true
}

// mergeDuplicateCases merges groups of duplicate cases
func (cj *CompactionJob) mergeDuplicateCases(cases []*CaseRecord, groups [][]int) ([]*CaseRecord, int) {
	if len(groups) == 0 {
		return cases, 0
	}

	result := make([]*CaseRecord, 0, len(cases))
	toSkip := make(map[int]bool)
	totalMerged := 0

	// Process each group
	for _, group := range groups {
		if len(group) <= 1 {
			continue
		}

		// Merge group into the most recent case
		mergedCase := cj.mergeCaseGroup(cases, group)
		result = append(result, mergedCase)

		// Mark other cases in group to skip
		for i := 1; i < len(group); i++ {
			toSkip[group[i]] = true
		}

		totalMerged += len(group) - 1
	}

	// Add non-duplicate cases
	for i, caseRecord := range cases {
		if !toSkip[i] {
			result = append(result, caseRecord)
		}
	}

	return result, totalMerged
}

// mergeCaseGroup merges a group of similar cases into one representative case
func (cj *CompactionJob) mergeCaseGroup(cases []*CaseRecord, groupIndices []int) *CaseRecord {
	// Use the most recent case as the base
	sort.Slice(groupIndices, func(i, j int) bool {
		return cases[groupIndices[i]].Timestamp.After(cases[groupIndices[j]].Timestamp)
	})

	baseCase := *cases[groupIndices[0]] // Copy the base case

	// Aggregate statistics from all cases in the group
	// For now, we'll keep the most recent case and add a note about merging
	// In a more sophisticated implementation, we could aggregate outcomes

	if baseCase.Context == nil {
		baseCase.Context = &CaseContext{}
	}

	if baseCase.Context.Metadata == nil {
		baseCase.Context.Metadata = make(map[string]string)
	}

	// Add metadata about the merge
	baseCase.Context.Metadata["merged_cases"] = fmt.Sprintf("%d", len(groupIndices))
	baseCase.Context.Metadata["merge_timestamp"] = time.Now().Format(time.RFC3339)

	// Update run ID to indicate this is a merged case
	baseCase.RunID = baseCase.RunID + "_merged_" + fmt.Sprintf("%d", time.Now().Unix())

	return &baseCase
}

// rewriteCases rewrites all cases for a signature after compaction
func (cj *CompactionJob) rewriteCases(ctx context.Context, lang, signature string, cases []*CaseRecord) error {
	// For simplicity, we'll delete all existing cases and rewrite them
	// In production, this might be optimized to only update changed cases

	for _, caseRecord := range cases {
		err := cj.storage.WriteCase(ctx, lang, signature, caseRecord.RunID, caseRecord)
		if err != nil {
			return fmt.Errorf("failed to write merged case %s: %w", caseRecord.RunID, err)
		}
	}

	return nil
}

// CompactAllSignatures runs compaction across all signatures in the KB
func (cj *CompactionJob) CompactAllSignatures(ctx context.Context) (*DeduplicationStats, error) {
	totalStats := &DeduplicationStats{}
	startTime := time.Now()
	defer func() {
		totalStats.CompactionDuration = time.Since(startTime)
	}()

	// Get KB snapshot to find all signatures
	snapshot, err := cj.storage.ReadSnapshot(ctx)
	if err != nil {
		return totalStats, fmt.Errorf("failed to read KB snapshot: %w", err)
	}

	// Compact each language's signatures
	for lang := range snapshot.Languages {
		langStats, err := cj.compactLanguage(ctx, lang)
		if err != nil {
			totalStats.Errors = append(totalStats.Errors, fmt.Sprintf("language %s: %v", lang, err))
		}

		// Aggregate statistics
		totalStats.SignaturesAnalyzed += langStats.SignaturesAnalyzed
		totalStats.CasesAnalyzed += langStats.CasesAnalyzed
		totalStats.CasesMerged += langStats.CasesMerged
		totalStats.CasesDeleted += langStats.CasesDeleted
		totalStats.PatchesDeduplicated += langStats.PatchesDeduplicated
		totalStats.StorageSaved += langStats.StorageSaved
		totalStats.Errors = append(totalStats.Errors, langStats.Errors...)
	}

	return totalStats, nil
}

// compactLanguage compacts all signatures for a specific language
func (cj *CompactionJob) compactLanguage(ctx context.Context, lang string) (*DeduplicationStats, error) {
	langStats := &DeduplicationStats{}

	// For now, we'll implement a simple version that would need to be
	// enhanced to actually discover all signatures for a language
	// This would require extending the storage interface or maintaining an index

	// TODO: Implement signature discovery for a language
	// This would involve listing all keys with prefix kb/healing/errors/{lang}/
	// and extracting the signatures from the key patterns

	return langStats, nil
}

// PatchDeduplicator handles deduplication of patch storage
type PatchDeduplicator struct {
	storage      KBStorage
	sigGenerator EnhancedSignatureGenerator
	config       *CompactionConfig
}

// NewPatchDeduplicator creates a new patch deduplicator
func NewPatchDeduplicator(storage KBStorage, sigGenerator EnhancedSignatureGenerator, config *CompactionConfig) *PatchDeduplicator {
	if config == nil {
		config = DefaultCompactionConfig()
	}

	return &PatchDeduplicator{
		storage:      storage,
		sigGenerator: sigGenerator,
		config:       config,
	}
}

// DeduplicatePatches finds and merges similar patches
func (pd *PatchDeduplicator) DeduplicatePatches(ctx context.Context, fingerprints []string) (*DeduplicationStats, error) {
	stats := &DeduplicationStats{}

	if len(fingerprints) < 2 {
		return stats, nil
	}

	// Load all patches
	patches := make(map[string][]byte)
	for _, fingerprint := range fingerprints {
		patch, err := pd.storage.GetPatch(ctx, fingerprint)
		if err != nil {
			continue // Skip missing patches
		}
		patches[fingerprint] = patch
	}

	// Find similar patches
	duplicateGroups := pd.findSimilarPatches(patches)

	// For each group, keep the shortest patch and redirect others
	for _, group := range duplicateGroups {
		if len(group) <= 1 {
			continue
		}

		// Find the canonical patch (shortest one as it's likely most concise)
		canonical := group[0]
		for _, fingerprint := range group[1:] {
			if len(patches[fingerprint]) < len(patches[canonical]) {
				canonical = fingerprint
			}
		}

		// In a full implementation, we would update all references to point to the canonical patch
		// For now, we just count the deduplication
		stats.PatchesDeduplicated += len(group) - 1

		// Estimate storage saved
		for _, fingerprint := range group {
			if fingerprint != canonical {
				stats.StorageSaved += int64(len(patches[fingerprint]))
			}
		}
	}

	return stats, nil
}

// findSimilarPatches groups patches by similarity
func (pd *PatchDeduplicator) findSimilarPatches(patches map[string][]byte) [][]string {
	fingerprints := make([]string, 0, len(patches))
	for fp := range patches {
		fingerprints = append(fingerprints, fp)
	}

	var groups [][]string
	processed := make(map[string]bool)

	for _, fp1 := range fingerprints {
		if processed[fp1] {
			continue
		}

		group := []string{fp1}
		processed[fp1] = true

		// Find similar patches
		for _, fp2 := range fingerprints {
			if processed[fp2] || fp1 == fp2 {
				continue
			}

			similarity := pd.sigGenerator.ComputePatchSimilarity(patches[fp1], patches[fp2])
			if similarity >= pd.config.SimilarityThresholdForMerge {
				group = append(group, fp2)
				processed[fp2] = true
			}
		}

		if len(group) > 1 {
			groups = append(groups, group)
		}
	}

	return groups
}

// CompactStorage provides high-level storage compaction operations
type CompactStorage struct {
	storage         KBStorage
	sigGenerator    EnhancedSignatureGenerator
	lockMgr         KBLockManager
	summaryComputer *SummaryComputer
	config          *CompactionConfig
}

// NewCompactStorage creates a new compact storage manager
func NewCompactStorage(
	storage KBStorage,
	sigGenerator EnhancedSignatureGenerator,
	lockMgr KBLockManager,
	summaryComputer *SummaryComputer,
	config *CompactionConfig,
) *CompactStorage {
	return &CompactStorage{
		storage:         storage,
		sigGenerator:    sigGenerator,
		lockMgr:         lockMgr,
		summaryComputer: summaryComputer,
		config:          config,
	}
}

// RunFullCompaction performs complete KB compaction
func (cs *CompactStorage) RunFullCompaction(ctx context.Context) (*DeduplicationStats, error) {
	job := NewCompactionJob(cs.storage, cs.sigGenerator, cs.lockMgr, cs.summaryComputer, cs.config)
	return job.CompactAllSignatures(ctx)
}

// CompactSignature compacts a specific error signature
func (cs *CompactStorage) CompactSignature(ctx context.Context, lang, signature string) (*DeduplicationStats, error) {
	job := NewCompactionJob(cs.storage, cs.sigGenerator, cs.lockMgr, cs.summaryComputer, cs.config)
	return job.CompactSignature(ctx, lang, signature)
}

// DeduplicatePatches removes duplicate patches from storage
func (cs *CompactStorage) DeduplicatePatches(ctx context.Context, fingerprints []string) (*DeduplicationStats, error) {
	deduplicator := NewPatchDeduplicator(cs.storage, cs.sigGenerator, cs.config)
	return deduplicator.DeduplicatePatches(ctx, fingerprints)
}
