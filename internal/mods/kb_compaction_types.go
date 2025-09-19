package mods

import "time"

// CompactionConfig contains configuration for KB compaction operations.
type CompactionConfig struct {
	MinCasesForCompaction       int           `json:"min_cases_for_compaction"`
	SimilarityThresholdForMerge float64       `json:"similarity_threshold_for_merge"`
	MaxCasesToAnalyze           int           `json:"max_cases_to_analyze"`
	MaxCompactionBatchSize      int           `json:"max_compaction_batch_size"`
	CompactionTimeout           time.Duration `json:"compaction_timeout"`
	MaxCaseAge                  time.Duration `json:"max_case_age"`
	MinCasesToRetain            int           `json:"min_cases_to_retain"`
	DryRun                      bool          `json:"dry_run"`
}

// DefaultCompactionConfig returns reasonable defaults for compaction.
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

// DeduplicationStats contains statistics about compaction operations.
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

// CompactionJob represents a KB storage compaction job.
type CompactionJob struct {
	storage         KBStorage
	sigGenerator    EnhancedSignatureGenerator
	lockMgr         KBLockManager
	summaryComputer *SummaryComputer
	config          *CompactionConfig
}

// NewCompactionJob creates a new compaction job.
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

// PatchDeduplicator handles deduplication of patch storage.
type PatchDeduplicator struct {
	storage      KBStorage
	sigGenerator EnhancedSignatureGenerator
	config       *CompactionConfig
}

// NewPatchDeduplicator creates a new patch deduplicator.
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
