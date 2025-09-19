package mods

import "context"

// CompactStorage provides high-level storage compaction operations.
type CompactStorage struct {
	storage         KBStorage
	sigGenerator    EnhancedSignatureGenerator
	lockMgr         KBLockManager
	summaryComputer *SummaryComputer
	config          *CompactionConfig
}

// NewCompactStorage creates a new compact storage manager.
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

// RunFullCompaction performs complete KB compaction.
func (cs *CompactStorage) RunFullCompaction(ctx context.Context) (*DeduplicationStats, error) {
	job := NewCompactionJob(cs.storage, cs.sigGenerator, cs.lockMgr, cs.summaryComputer, cs.config)
	return job.CompactAllSignatures(ctx)
}

// CompactSignature compacts a specific error signature.
func (cs *CompactStorage) CompactSignature(ctx context.Context, lang, signature string) (*DeduplicationStats, error) {
	job := NewCompactionJob(cs.storage, cs.sigGenerator, cs.lockMgr, cs.summaryComputer, cs.config)
	return job.CompactSignature(ctx, lang, signature)
}

// DeduplicatePatches removes duplicate patches from storage.
func (cs *CompactStorage) DeduplicatePatches(ctx context.Context, fingerprints []string) (*DeduplicationStats, error) {
	deduplicator := NewPatchDeduplicator(cs.storage, cs.sigGenerator, cs.config)
	return deduplicator.DeduplicatePatches(ctx, fingerprints)
}
