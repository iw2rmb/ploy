package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// KB storage interface and data structures for Mods MVP knowledge base persistence

// CaseContext contains the context information for a healing case
type CaseContext struct {
	Language         string            `json:"language"`
	Lane             string            `json:"lane,omitempty"`
	RepoURL          string            `json:"repo_url,omitempty"`
	DependencyHashes []string          `json:"dependency_hashes,omitempty"`
	CompilerVersion  string            `json:"compiler_version,omitempty"`
	BuildCommand     string            `json:"build_command,omitempty"`
	SourceFiles      []string          `json:"source_files,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// HealingAttempt represents the healing approach that was tried
type HealingAttempt struct {
	Type             string                 `json:"type"` // "orw_recipe", "llm_patch", "human_step"
	Recipe           string                 `json:"recipe,omitempty"`
	PatchFingerprint string                 `json:"patch_fingerprint,omitempty"`
	PatchContent     string                 `json:"patch_content,omitempty"`
	Instructions     map[string]interface{} `json:"instructions,omitempty"`
}

// HealingOutcome represents the result of applying the healing attempt
type HealingOutcome struct {
	Success      bool      `json:"success"`
	BuildStatus  string    `json:"build_status"` // "passed", "failed", "timeout"
	ErrorChanged bool      `json:"error_changed"`
	Duration     int64     `json:"duration_ms"`
	CompletedAt  time.Time `json:"completed_at"`
}

// SanitizedLogs contains build logs with sensitive information removed
type SanitizedLogs struct {
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	MaxLength int    `json:"max_length,omitempty"`
}

// CaseRecord represents a complete healing case record
type CaseRecord struct {
	RunID     string          `json:"run_id"`
	Timestamp time.Time       `json:"timestamp"`
	Language  string          `json:"language"`
	Signature string          `json:"signature"`
	Context   *CaseContext    `json:"context"`
	Attempt   *HealingAttempt `json:"attempt"`
	Outcome   *HealingOutcome `json:"outcome"`
	BuildLogs *SanitizedLogs  `json:"build_logs,omitempty"`
}

// PromotedFix represents a successful fix promoted to summary
type PromotedFix struct {
	Kind          string    `json:"kind"` // "orw_recipe" or "patch_fingerprint"
	Ref           string    `json:"ref"`  // recipe name or patch fingerprint
	Score         float64   `json:"score"`
	Wins          int       `json:"wins"`
	Failures      int       `json:"failures"`
	LastSuccessAt time.Time `json:"last_success_at"`
	FirstSeenAt   time.Time `json:"first_seen_at"`
}

// SummaryStats contains aggregate statistics for an error signature
type SummaryStats struct {
	TotalCases   int       `json:"total_cases"`
	SuccessCount int       `json:"success_count"`
	FailureCount int       `json:"failure_count"`
	SuccessRate  float64   `json:"success_rate"`
	LastUpdated  time.Time `json:"last_updated"`
	AvgDuration  int64     `json:"avg_duration_ms"`
}

// SummaryRecord contains promoted fixes and stats for an error signature
type SummaryRecord struct {
	Language  string        `json:"language"`
	Signature string        `json:"signature"`
	Promoted  []PromotedFix `json:"promoted"`
	Stats     *SummaryStats `json:"stats"`
}

// SnapshotManifest contains KB state information
type SnapshotManifest struct {
	Timestamp  time.Time      `json:"timestamp"`
	Languages  map[string]int `json:"languages"` // language -> case count
	TotalCases int            `json:"total_cases"`
	TotalSigs  int            `json:"total_signatures"`
	Version    string         `json:"version"`
}

// KBStorage provides persistent storage interface for Mods knowledge base
type KBStorage interface {
	// Case operations
	WriteCase(ctx context.Context, lang, signature, runID string, caseData *CaseRecord) error
	ReadCases(ctx context.Context, lang, signature string) ([]*CaseRecord, error)

	// Summary operations
	ReadSummary(ctx context.Context, lang, signature string) (*SummaryRecord, error)
	WriteSummary(ctx context.Context, lang, signature string, summary *SummaryRecord) error

	// Patch operations
	StorePatch(ctx context.Context, fingerprint string, patch []byte) error
	GetPatch(ctx context.Context, fingerprint string) ([]byte, error)

	// Snapshot operations
	WriteSnapshot(ctx context.Context, snapshot *SnapshotManifest) error
	ReadSnapshot(ctx context.Context) (*SnapshotManifest, error)

	// Health check
	Health(ctx context.Context) error
}

// SeaweedFSKBStorage implements KBStorage using SeaweedFS backend
type SeaweedFSKBStorage struct {
	storage storage.Storage
	lockMgr KBLockManager
}

// NewSeaweedFSKBStorage creates a new SeaweedFS-backed KB storage
func NewSeaweedFSKBStorage(storage storage.Storage, lockMgr KBLockManager) *SeaweedFSKBStorage {
	return &SeaweedFSKBStorage{
		storage: storage,
		lockMgr: lockMgr,
	}
}

// WriteCase stores a healing case record
func (s *SeaweedFSKBStorage) WriteCase(ctx context.Context, lang, signature, runID string, caseData *CaseRecord) error {
	key := s.buildCaseKey(lang, signature, runID)

	data, err := json.Marshal(caseData)
	if err != nil {
		return fmt.Errorf("failed to marshal case data: %w", err)
	}

	reader := bytes.NewReader(data)
	err = s.storage.Put(ctx, key, reader, storage.WithContentType("application/json"))
	if err != nil {
		return fmt.Errorf("failed to store case: %w", err)
	}

	return nil
}

// ReadCases retrieves all cases for a given error signature
func (s *SeaweedFSKBStorage) ReadCases(ctx context.Context, lang, signature string) ([]*CaseRecord, error) {
	prefix := s.buildCasesPrefix(lang, signature)

	objects, err := s.storage.List(ctx, storage.ListOptions{
		Prefix:  prefix,
		MaxKeys: 1000, // Reasonable limit
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list cases: %w", err)
	}

	var cases []*CaseRecord
	for _, obj := range objects {
		reader, err := s.storage.Get(ctx, obj.Key)
		if err != nil {
			continue // Skip failed reads
		}

		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			continue
		}

		var caseRecord CaseRecord
		if err := json.Unmarshal(data, &caseRecord); err != nil {
			continue // Skip malformed records
		}

		cases = append(cases, &caseRecord)
	}

	return cases, nil
}

// ReadSummary retrieves the summary record for an error signature
func (s *SeaweedFSKBStorage) ReadSummary(ctx context.Context, lang, signature string) (*SummaryRecord, error) {
	key := s.buildSummaryKey(lang, signature)

	reader, err := s.storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get summary: %w", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read summary data: %w", err)
	}

	var summary SummaryRecord
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, fmt.Errorf("failed to unmarshal summary: %w", err)
	}

	return &summary, nil
}

// WriteSummary stores a summary record with optional locking
func (s *SeaweedFSKBStorage) WriteSummary(ctx context.Context, lang, signature string, summary *SummaryRecord) error {
	key := s.buildSummaryKey(lang, signature)

	data, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	reader := bytes.NewReader(data)
	err = s.storage.Put(ctx, key, reader, storage.WithContentType("application/json"))
	if err != nil {
		return fmt.Errorf("failed to store summary: %w", err)
	}

	return nil
}

// StorePatch stores a patch by its content fingerprint
func (s *SeaweedFSKBStorage) StorePatch(ctx context.Context, fingerprint string, patch []byte) error {
	key := s.buildPatchKey(fingerprint)

	reader := bytes.NewReader(patch)
	err := s.storage.Put(ctx, key, reader, storage.WithContentType("text/plain"))
	if err != nil {
		return fmt.Errorf("failed to store patch: %w", err)
	}

	return nil
}

// GetPatch retrieves a patch by its fingerprint
func (s *SeaweedFSKBStorage) GetPatch(ctx context.Context, fingerprint string) ([]byte, error) {
	key := s.buildPatchKey(fingerprint)

	reader, err := s.storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get patch: %w", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read patch data: %w", err)
	}

	return data, nil
}

// WriteSnapshot stores the KB snapshot manifest
func (s *SeaweedFSKBStorage) WriteSnapshot(ctx context.Context, snapshot *SnapshotManifest) error {
	key := "kb/healing/snapshot.json"

	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	reader := bytes.NewReader(data)
	err = s.storage.Put(ctx, key, reader, storage.WithContentType("application/json"))
	if err != nil {
		return fmt.Errorf("failed to store snapshot: %w", err)
	}

	return nil
}

// ReadSnapshot retrieves the KB snapshot manifest
func (s *SeaweedFSKBStorage) ReadSnapshot(ctx context.Context) (*SnapshotManifest, error) {
	key := "kb/healing/snapshot.json"

	reader, err := s.storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot data: %w", err)
	}

	var snapshot SnapshotManifest
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	return &snapshot, nil
}

// Health performs a health check on the storage backend
func (s *SeaweedFSKBStorage) Health(ctx context.Context) error {
	return s.storage.Health(ctx)
}

// Key building helpers

func (s *SeaweedFSKBStorage) buildCaseKey(lang, signature, runID string) string {
	return path.Join("kb/healing/errors", lang, signature, "cases", runID+".json")
}

func (s *SeaweedFSKBStorage) buildCasesPrefix(lang, signature string) string {
	return path.Join("kb/healing/errors", lang, signature, "cases") + "/"
}

func (s *SeaweedFSKBStorage) buildSummaryKey(lang, signature string) string {
	return path.Join("kb/healing/errors", lang, signature, "summary.json")
}

func (s *SeaweedFSKBStorage) buildPatchKey(fingerprint string) string {
	return path.Join("kb/healing/patches", fingerprint+".patch")
}
