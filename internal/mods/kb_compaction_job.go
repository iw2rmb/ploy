package mods

import (
	"context"
	"fmt"
	"time"
)

// CompactSignature compacts all cases for a specific error signature.
func (cj *CompactionJob) CompactSignature(ctx context.Context, lang, signature string) (*DeduplicationStats, error) {
	stats := &DeduplicationStats{}
	startTime := time.Now()
	defer func() {
		stats.CompactionDuration = time.Since(startTime)
	}()

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

func (cj *CompactionJob) compactSignatureLocked(ctx context.Context, lang, signature string, stats *DeduplicationStats) error {
	stats.SignaturesAnalyzed++

	cases, err := cj.storage.ReadCases(ctx, lang, signature)
	if err != nil {
		return fmt.Errorf("failed to read cases: %w", err)
	}

	if len(cases) < cj.config.MinCasesForCompaction {
		return nil
	}

	stats.CasesAnalyzed = len(cases)

	retainedCases := cj.applyRetentionPolicy(cases)
	if len(retainedCases) < len(cases) {
		stats.CasesDeleted = len(cases) - len(retainedCases)
		cases = retainedCases
	}

	duplicateGroups := cj.findDuplicateCases(cases)
	mergedCases, merged := cj.mergeDuplicateCases(cases, duplicateGroups)
	stats.CasesMerged = merged

	if len(mergedCases) != len(cases) || merged > 0 {
		if !cj.config.DryRun {
			if err := cj.rewriteCases(ctx, lang, signature, mergedCases); err != nil {
				return fmt.Errorf("failed to rewrite cases: %w", err)
			}

			if err := cj.summaryComputer.ComputeAndUpdateSummary(ctx, lang, signature); err != nil {
				stats.Errors = append(stats.Errors, fmt.Sprintf("summary update failed: %v", err))
			}
		}
	}

	return nil
}

// CompactAllSignatures runs compaction across all signatures in the KB.
func (cj *CompactionJob) CompactAllSignatures(ctx context.Context) (*DeduplicationStats, error) {
	totalStats := &DeduplicationStats{}
	startTime := time.Now()
	defer func() {
		totalStats.CompactionDuration = time.Since(startTime)
	}()

	snapshot, err := cj.storage.ReadSnapshot(ctx)
	if err != nil {
		return totalStats, fmt.Errorf("failed to read KB snapshot: %w", err)
	}

	for lang := range snapshot.Languages {
		langStats, err := cj.compactLanguage(ctx, lang)
		if err != nil {
			totalStats.Errors = append(totalStats.Errors, fmt.Sprintf("language %s: %v", lang, err))
		}

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

func (cj *CompactionJob) compactLanguage(ctx context.Context, lang string) (*DeduplicationStats, error) {
	langStats := &DeduplicationStats{}

	// TODO: Implement signature discovery for a language.

	return langStats, nil
}

func (cj *CompactionJob) rewriteCases(ctx context.Context, lang, signature string, cases []*CaseRecord) error {
	for _, caseRecord := range cases {
		if err := cj.storage.WriteCase(ctx, lang, signature, caseRecord.RunID, caseRecord); err != nil {
			return fmt.Errorf("failed to write merged case %s: %w", caseRecord.RunID, err)
		}
	}
	return nil
}
