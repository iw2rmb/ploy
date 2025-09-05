package transflow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock KB Storage for testing
type MockKBStorage struct {
	mock.Mock
}

func (m *MockKBStorage) WriteCase(ctx context.Context, lang, signature, runID string, caseData *CaseRecord) error {
	args := m.Called(ctx, lang, signature, runID, caseData)
	return args.Error(0)
}

func (m *MockKBStorage) ReadCases(ctx context.Context, lang, signature string) ([]*CaseRecord, error) {
	args := m.Called(ctx, lang, signature)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*CaseRecord), args.Error(1)
}

func (m *MockKBStorage) ReadSummary(ctx context.Context, lang, signature string) (*SummaryRecord, error) {
	args := m.Called(ctx, lang, signature)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SummaryRecord), args.Error(1)
}

func (m *MockKBStorage) WriteSummary(ctx context.Context, lang, signature string, summary *SummaryRecord) error {
	args := m.Called(ctx, lang, signature, summary)
	return args.Error(0)
}

func (m *MockKBStorage) StorePatch(ctx context.Context, fingerprint string, patch []byte) error {
	args := m.Called(ctx, fingerprint, patch)
	return args.Error(0)
}

func (m *MockKBStorage) GetPatch(ctx context.Context, fingerprint string) ([]byte, error) {
	args := m.Called(ctx, fingerprint)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockKBStorage) WriteSnapshot(ctx context.Context, snapshot *SnapshotManifest) error {
	args := m.Called(ctx, snapshot)
	return args.Error(0)
}

func (m *MockKBStorage) ReadSnapshot(ctx context.Context) (*SnapshotManifest, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SnapshotManifest), args.Error(1)
}

func (m *MockKBStorage) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Mock Lock Manager for testing
type MockLockManager struct {
	mock.Mock
}

func (m *MockLockManager) AcquireLock(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	args := m.Called(ctx, key, ttl)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Lock), args.Error(1)
}

func (m *MockLockManager) ReleaseLock(ctx context.Context, lock *Lock) error {
	args := m.Called(ctx, lock)
	return args.Error(0)
}

func (m *MockLockManager) IsLocked(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockLockManager) TryWithLockRetry(ctx context.Context, key string, config *LockConfig, fn func() error) error {
	args := m.Called(ctx, key, config, fn)
	return args.Error(0)
}

// Test data creation helpers
func createTestCases() []*CaseRecord {
	baseTime := time.Date(2023, 12, 1, 10, 0, 0, 0, time.UTC)

	return []*CaseRecord{
		// Successful OpenRewrite recipe case
		{
			RunID:     "run-001",
			Timestamp: baseTime,
			Language:  "java",
			Signature: "test-signature",
			Attempt: &HealingAttempt{
				Type:   "orw_recipe",
				Recipe: "org.openrewrite.java.RemoveUnusedImports",
			},
			Outcome: &HealingOutcome{
				Success:     true,
				BuildStatus: "passed",
				Duration:    3000, // 3 seconds
				CompletedAt: baseTime.Add(3 * time.Second),
			},
		},
		// Another successful case with same recipe
		{
			RunID:     "run-002",
			Timestamp: baseTime.Add(1 * time.Hour),
			Language:  "java",
			Signature: "test-signature",
			Attempt: &HealingAttempt{
				Type:   "orw_recipe",
				Recipe: "org.openrewrite.java.RemoveUnusedImports",
			},
			Outcome: &HealingOutcome{
				Success:     true,
				BuildStatus: "passed",
				Duration:    2500,
				CompletedAt: baseTime.Add(1*time.Hour + 2500*time.Millisecond),
			},
		},
		// Failed case with same recipe
		{
			RunID:     "run-003",
			Timestamp: baseTime.Add(2 * time.Hour),
			Language:  "java",
			Signature: "test-signature",
			Attempt: &HealingAttempt{
				Type:   "orw_recipe",
				Recipe: "org.openrewrite.java.RemoveUnusedImports",
			},
			Outcome: &HealingOutcome{
				Success:     false,
				BuildStatus: "failed",
				Duration:    1000,
				CompletedAt: baseTime.Add(2*time.Hour + 1000*time.Millisecond),
			},
		},
		// Successful patch case
		{
			RunID:     "run-004",
			Timestamp: baseTime.Add(3 * time.Hour),
			Language:  "java",
			Signature: "test-signature",
			Attempt: &HealingAttempt{
				Type:             "llm_patch",
				PatchFingerprint: "abc123def456789",
			},
			Outcome: &HealingOutcome{
				Success:     true,
				BuildStatus: "passed",
				Duration:    4000,
				CompletedAt: baseTime.Add(3*time.Hour + 4000*time.Millisecond),
			},
		},
		// Another successful patch case (different fingerprint)
		{
			RunID:     "run-005",
			Timestamp: baseTime.Add(4 * time.Hour),
			Language:  "java",
			Signature: "test-signature",
			Attempt: &HealingAttempt{
				Type:             "llm_patch",
				PatchFingerprint: "xyz789abc123def",
			},
			Outcome: &HealingOutcome{
				Success:     true,
				BuildStatus: "passed",
				Duration:    3500,
				CompletedAt: baseTime.Add(4*time.Hour + 3500*time.Millisecond),
			},
		},
		// Case with insufficient data (should be skipped)
		{
			RunID:     "run-006",
			Timestamp: baseTime.Add(5 * time.Hour),
			Language:  "java",
			Signature: "test-signature",
			Attempt: &HealingAttempt{
				Type: "orw_recipe",
				// Missing recipe field
			},
			Outcome: &HealingOutcome{
				Success:     false,
				BuildStatus: "failed",
				Duration:    500,
				CompletedAt: baseTime.Add(5*time.Hour + 500*time.Millisecond),
			},
		},
	}
}

func TestSummaryComputer_AnalyzeCases(t *testing.T) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	config := DefaultSummaryConfig()
	computer := NewSummaryComputer(mockStorage, mockLockMgr, config)

	cases := createTestCases()
	candidates := computer.analyzeCases(cases)

	// Should have 3 candidates: 1 recipe + 2 patches
	assert.Len(t, candidates, 3)

	// Check recipe candidate
	recipeCand := candidates["recipe:org.openrewrite.java.RemoveUnusedImports"]
	assert.NotNil(t, recipeCand)
	assert.Equal(t, "orw_recipe", recipeCand.Kind)
	assert.Equal(t, "org.openrewrite.java.RemoveUnusedImports", recipeCand.Ref)
	assert.Equal(t, 2, recipeCand.Wins)
	assert.Equal(t, 1, recipeCand.Failures)
	assert.Equal(t, 3, recipeCand.TotalCases)
	assert.InDelta(t, 0.667, recipeCand.SuccessRate, 0.01)

	// Check first patch candidate
	patch1Cand := candidates["patch:abc123def456789"]
	assert.NotNil(t, patch1Cand)
	assert.Equal(t, "patch_fingerprint", patch1Cand.Kind)
	assert.Equal(t, "abc123def456789", patch1Cand.Ref)
	assert.Equal(t, 1, patch1Cand.Wins)
	assert.Equal(t, 0, patch1Cand.Failures)
	assert.Equal(t, 1, patch1Cand.TotalCases)
	assert.Equal(t, 1.0, patch1Cand.SuccessRate)

	// Check second patch candidate
	patch2Cand := candidates["patch:xyz789abc123def"]
	assert.NotNil(t, patch2Cand)
	assert.Equal(t, 1, patch2Cand.Wins)
	assert.Equal(t, 1.0, patch2Cand.SuccessRate)
}

func TestSummaryComputer_ScoreCandidates(t *testing.T) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	config := DefaultSummaryConfig()
	config.MinCasesForPromotion = 1 // Lower threshold for testing
	config.MinSuccessRate = 0.5
	computer := NewSummaryComputer(mockStorage, mockLockMgr, config)

	// Create test candidates
	candidates := map[string]*FixCandidate{
		"recipe:test1": {
			Kind:          "orw_recipe",
			Ref:           "test.recipe1",
			Wins:          8,
			Failures:      2,
			TotalCases:    10,
			SuccessRate:   0.8,
			LastSuccessAt: time.Now().Add(-1 * time.Hour), // Recent
		},
		"recipe:test2": {
			Kind:          "orw_recipe",
			Ref:           "test.recipe2",
			Wins:          3,
			Failures:      2,
			TotalCases:    5,
			SuccessRate:   0.6,
			LastSuccessAt: time.Now().Add(-30 * 24 * time.Hour), // Old
		},
		"patch:abc123": {
			Kind:          "patch_fingerprint",
			Ref:           "abc123",
			Wins:          2,
			Failures:      0,
			TotalCases:    2,
			SuccessRate:   1.0,
			LastSuccessAt: time.Now().Add(-2 * time.Hour), // Fairly recent
		},
		"recipe:lowsuccess": {
			Kind:        "orw_recipe",
			Ref:         "test.lowsuccess",
			Wins:        1,
			Failures:    4,
			TotalCases:  5,
			SuccessRate: 0.2, // Below minimum threshold
		},
	}

	scored := computer.scoreCandidates(candidates)

	// Should filter out low success rate candidate
	assert.Len(t, scored, 3)

	// Should be sorted by score (highest first)
	assert.True(t, scored[0].Score >= scored[1].Score)
	assert.True(t, scored[1].Score >= scored[2].Score)

	// Recipe with high wins and recent success should score well
	found := false
	for _, candidate := range scored {
		if candidate.Ref == "test.recipe1" {
			found = true
			assert.True(t, candidate.Score > 0.5, "High-performing recent fix should have good score")
		}
	}
	assert.True(t, found, "High-performing candidate should be included")
}

func TestSummaryComputer_ComputeStats(t *testing.T) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	config := DefaultSummaryConfig()
	computer := NewSummaryComputer(mockStorage, mockLockMgr, config)

	cases := createTestCases()
	stats := computer.computeStats(cases)

	assert.Equal(t, 6, stats.TotalCases)
	assert.Equal(t, 4, stats.SuccessCount)
	assert.Equal(t, 2, stats.FailureCount)
	assert.InDelta(t, 0.667, stats.SuccessRate, 0.01)

	// Average duration should be calculated from valid durations
	expectedAvg := (3000 + 2500 + 1000 + 4000 + 3500 + 500) / 6
	assert.Equal(t, int64(expectedAvg), stats.AvgDuration)
	assert.WithinDuration(t, time.Now(), stats.LastUpdated, 1*time.Second)
}

func TestSummaryComputer_ComputeAndUpdateSummary(t *testing.T) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	config := DefaultSummaryConfig()
	config.MinCasesForPromotion = 1 // Lower threshold for testing
	config.MaxPromotedFixes = 2     // Limit for testing
	computer := NewSummaryComputer(mockStorage, mockLockMgr, config)

	ctx := context.Background()
	lang := "java"
	signature := "test-signature"
	lockKey := BuildSignatureLockKey(lang, signature)

	cases := createTestCases()

	// Mock lock acquisition
	_ = &Lock{Key: lockKey, SessionID: "test-session"} // testLock created but not used in this test
	mockLockMgr.On("TryWithLockRetry", ctx, lockKey, mock.AnythingOfType("*transflow.LockConfig"), mock.AnythingOfType("func() error")).
		Return(nil).
		Run(func(args mock.Arguments) {
			// Execute the function under lock
			fn := args.Get(3).(func() error)
			fn()
		})

	// Mock storage operations
	mockStorage.On("ReadCases", ctx, lang, signature).Return(cases, nil)
	mockStorage.On("WriteSummary", ctx, lang, signature, mock.AnythingOfType("*transflow.SummaryRecord")).Return(nil)

	err := computer.ComputeAndUpdateSummary(ctx, lang, signature)

	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
	mockLockMgr.AssertExpectations(t)
}

func TestSummaryComputer_GetRecommendedFixes(t *testing.T) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	computer := NewSummaryComputer(mockStorage, mockLockMgr, DefaultSummaryConfig())

	ctx := context.Background()
	lang := "java"
	signature := "test-signature"

	// Mock summary with promoted fixes
	summary := &SummaryRecord{
		Language:  lang,
		Signature: signature,
		Promoted: []PromotedFix{
			{
				Kind:          "orw_recipe",
				Ref:           "org.openrewrite.java.RemoveUnusedImports",
				Score:         0.9,
				Wins:          10,
				Failures:      1,
				LastSuccessAt: time.Now(),
			},
			{
				Kind:          "patch_fingerprint",
				Ref:           "abc123def456",
				Score:         0.8,
				Wins:          5,
				Failures:      1,
				LastSuccessAt: time.Now().Add(-1 * time.Hour),
			},
			{
				Kind:          "orw_recipe",
				Ref:           "org.openrewrite.java.cleanup.SimplifyBooleanExpression",
				Score:         0.7,
				Wins:          3,
				Failures:      0,
				LastSuccessAt: time.Now().Add(-2 * time.Hour),
			},
		},
	}

	mockStorage.On("ReadSummary", ctx, lang, signature).Return(summary, nil)

	// Test getting all fixes
	fixes, err := computer.GetRecommendedFixes(ctx, lang, signature, 0)
	assert.NoError(t, err)
	assert.Len(t, fixes, 3)

	// Test limiting results
	fixes, err = computer.GetRecommendedFixes(ctx, lang, signature, 2)
	assert.NoError(t, err)
	assert.Len(t, fixes, 2)

	// Should return highest scoring fixes first
	assert.Equal(t, 0.9, fixes[0].Score)
	assert.Equal(t, 0.8, fixes[1].Score)

	mockStorage.AssertExpectations(t)
}

func TestSummaryComputer_UpdateSummaryAfterCase(t *testing.T) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	computer := NewSummaryComputer(mockStorage, mockLockMgr, DefaultSummaryConfig())

	ctx := context.Background()
	lang := "java"
	signature := "test-signature"
	lockKey := BuildSignatureLockKey(lang, signature)

	cases := []*CaseRecord{createTestCases()[0]} // Just one case

	// Mock successful lock and summary update
	mockLockMgr.On("TryWithLockRetry", ctx, lockKey, mock.AnythingOfType("*transflow.LockConfig"), mock.AnythingOfType("func() error")).
		Return(nil).
		Run(func(args mock.Arguments) {
			fn := args.Get(3).(func() error)
			fn()
		})

	mockStorage.On("ReadCases", ctx, lang, signature).Return(cases, nil)
	mockStorage.On("WriteSummary", ctx, lang, signature, mock.AnythingOfType("*transflow.SummaryRecord")).Return(nil)

	err := computer.UpdateSummaryAfterCase(ctx, lang, signature)

	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
	mockLockMgr.AssertExpectations(t)
}

func TestSummaryComputer_UpdateSummaryAfterCase_LockFailure(t *testing.T) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	computer := NewSummaryComputer(mockStorage, mockLockMgr, DefaultSummaryConfig())

	ctx := context.Background()
	lang := "java"
	signature := "test-signature"
	lockKey := BuildSignatureLockKey(lang, signature)

	// Mock lock acquisition failure (should not fail the operation)
	mockLockMgr.On("TryWithLockRetry", ctx, lockKey, mock.AnythingOfType("*transflow.LockConfig"), mock.AnythingOfType("func() error")).
		Return(assert.AnError)

	// Should succeed even if lock fails (non-blocking)
	err := computer.UpdateSummaryAfterCase(ctx, lang, signature)

	assert.Error(t, err) // Actually, this should return the error
	mockLockMgr.AssertExpectations(t)
}

func TestDefaultSummaryConfig(t *testing.T) {
	config := DefaultSummaryConfig()

	assert.Equal(t, 3, config.MinCasesForPromotion)
	assert.Equal(t, 0.6, config.MinSuccessRate)
	assert.Equal(t, 10, config.MaxPromotedFixes)
	assert.Equal(t, 0.3, config.RecencyWeight)
	assert.Equal(t, 0.4, config.FrequencyWeight)
	assert.Equal(t, 0.3, config.SuccessRateWeight)
	assert.Equal(t, 0.5, config.MinScore)
	assert.Equal(t, 90, config.PromotionLookbackDays)

	// Weights should sum to 1.0
	totalWeight := config.RecencyWeight + config.FrequencyWeight + config.SuccessRateWeight
	assert.InDelta(t, 1.0, totalWeight, 0.001)
}

func TestPromoteTopCandidates(t *testing.T) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	config := DefaultSummaryConfig()
	config.MaxPromotedFixes = 2 // Limit for testing
	computer := NewSummaryComputer(mockStorage, mockLockMgr, config)

	candidates := []*FixCandidate{
		{
			Kind:          "orw_recipe",
			Ref:           "best.recipe",
			Score:         0.9,
			Wins:          10,
			Failures:      1,
			LastSuccessAt: time.Now(),
			FirstSeenAt:   time.Now().Add(-24 * time.Hour),
		},
		{
			Kind:          "patch_fingerprint",
			Ref:           "good.patch",
			Score:         0.8,
			Wins:          5,
			Failures:      0,
			LastSuccessAt: time.Now().Add(-1 * time.Hour),
			FirstSeenAt:   time.Now().Add(-12 * time.Hour),
		},
		{
			Kind:          "orw_recipe",
			Ref:           "ok.recipe",
			Score:         0.6,
			Wins:          3,
			Failures:      1,
			LastSuccessAt: time.Now().Add(-2 * time.Hour),
			FirstSeenAt:   time.Now().Add(-6 * time.Hour),
		},
	}

	promoted := computer.promoteTopCandidates(candidates)

	// Should only promote top 2 due to MaxPromotedFixes limit
	assert.Len(t, promoted, 2)
	assert.Equal(t, "best.recipe", promoted[0].Ref)
	assert.Equal(t, "good.patch", promoted[1].Ref)
	assert.Equal(t, 0.9, promoted[0].Score)
	assert.Equal(t, 0.8, promoted[1].Score)
}

// Benchmark tests
func BenchmarkAnalyzeCases(b *testing.B) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	computer := NewSummaryComputer(mockStorage, mockLockMgr, DefaultSummaryConfig())

	cases := createTestCases()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computer.analyzeCases(cases)
	}
}

func BenchmarkScoreCandidates(b *testing.B) {
	mockStorage := new(MockKBStorage)
	mockLockMgr := new(MockLockManager)
	computer := NewSummaryComputer(mockStorage, mockLockMgr, DefaultSummaryConfig())

	// Create more candidates for realistic benchmarking
	candidates := make(map[string]*FixCandidate)
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("recipe:%d", i)
		candidates[key] = &FixCandidate{
			Kind:          "orw_recipe",
			Ref:           fmt.Sprintf("recipe.%d", i),
			Wins:          i + 1,
			Failures:      i / 3,
			TotalCases:    i + 1 + i/3,
			SuccessRate:   float64(i+1) / float64(i+1+i/3),
			LastSuccessAt: time.Now().Add(-time.Duration(i) * time.Hour),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computer.scoreCandidates(candidates)
	}
}
