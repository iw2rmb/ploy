package mods

import "context"

// performanceTestKBStorage implements KBStorage for analytics tests without
// pulling in the deprecated compatibility fixtures.
type performanceTestKBStorage struct {
	cases    []*CaseRecord
	snapshot *SnapshotManifest
}

func newPerformanceTestKBStorage() *performanceTestKBStorage {
	return &performanceTestKBStorage{}
}

func (m *performanceTestKBStorage) WriteCase(ctx context.Context, lang, signature, runID string, caseData *CaseRecord) error {
	return nil
}

func (m *performanceTestKBStorage) ReadCases(ctx context.Context, lang, signature string) ([]*CaseRecord, error) {
	return m.cases, nil
}

func (m *performanceTestKBStorage) ReadSummary(ctx context.Context, lang, signature string) (*SummaryRecord, error) {
	return &SummaryRecord{}, nil
}

func (m *performanceTestKBStorage) WriteSummary(ctx context.Context, lang, signature string, summary *SummaryRecord) error {
	return nil
}

func (m *performanceTestKBStorage) StorePatch(ctx context.Context, fingerprint string, patch []byte) error {
	return nil
}

func (m *performanceTestKBStorage) GetPatch(ctx context.Context, fingerprint string) ([]byte, error) {
	return []byte("mock patch"), nil
}

func (m *performanceTestKBStorage) WriteSnapshot(ctx context.Context, snapshot *SnapshotManifest) error {
	m.snapshot = snapshot
	return nil
}

func (m *performanceTestKBStorage) ReadSnapshot(ctx context.Context) (*SnapshotManifest, error) {
	if m.snapshot != nil {
		return m.snapshot, nil
	}
	return &SnapshotManifest{
		Languages:  make(map[string]int),
		TotalCases: 0,
		TotalSigs:  0,
	}, nil
}

func (m *performanceTestKBStorage) Health(ctx context.Context) error {
	return nil
}
