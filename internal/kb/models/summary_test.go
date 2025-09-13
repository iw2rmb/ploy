package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSummary_CalculateSuccessRate(t *testing.T) {
	tests := []struct {
		name          string
		cases         []Case
		expectedRate  float64
		expectedCount int
	}{
		{
			name: "mixed success and failure",
			cases: []Case{
				{Success: true},
				{Success: true},
				{Success: false},
				{Success: true},
			},
			expectedRate:  0.75, // 3 out of 4 successful
			expectedCount: 4,
		},
		{
			name:          "no cases",
			cases:         []Case{},
			expectedRate:  0.0,
			expectedCount: 0,
		},
		{
			name: "all successful",
			cases: []Case{
				{Success: true},
				{Success: true},
			},
			expectedRate:  1.0,
			expectedCount: 2,
		},
		{
			name: "all failed",
			cases: []Case{
				{Success: false},
				{Success: false},
			},
			expectedRate:  0.0,
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - Summary model doesn't exist yet
			s := &Summary{
				ErrorID: "test-error",
			}

			s.CalculateSuccessRate(tt.cases)

			assert.Equal(t, tt.expectedRate, s.SuccessRate)
			assert.Equal(t, tt.expectedCount, s.CaseCount)
			assert.True(t, s.Updated.After(time.Now().Add(-1*time.Second)))
		})
	}
}

func TestSummary_GenerateTopPatches(t *testing.T) {
	t.Run("generate top patches from cases", func(t *testing.T) {
		// This should fail - PatchSummary doesn't exist yet
		cases := []Case{
			{PatchHash: "hash-1", Success: true, Confidence: 0.9},
			{PatchHash: "hash-1", Success: true, Confidence: 0.8}, // Same patch, different confidence
			{PatchHash: "hash-2", Success: false, Confidence: 0.3},
			{PatchHash: "hash-3", Success: true, Confidence: 0.7},
			{PatchHash: "hash-1", Success: false, Confidence: 0.1}, // Same patch, failed
		}

		s := &Summary{
			ErrorID: "test-error",
		}

		s.GenerateTopPatches(cases, 2) // Limit to top 2 patches

		assert.Len(t, s.TopPatches, 2)

		// Should be sorted by success rate, hash-3 should be first (1.0 > 0.67)
		topPatch := s.TopPatches[0]
		assert.Equal(t, "hash-3", topPatch.Hash)
		assert.Equal(t, 1.0, topPatch.SuccessRate) // 1 success out of 1 attempt
		assert.Equal(t, 1, topPatch.AttemptCount)
		assert.Equal(t, 0.7, topPatch.AvgConfidence) // Average of 0.7

		// Second patch should be hash-1
		secondPatch := s.TopPatches[1]
		assert.Equal(t, "hash-1", secondPatch.Hash)
		assert.InDelta(t, 0.67, secondPatch.SuccessRate, 0.01) // 2 success out of 3 attempts
		assert.Equal(t, 3, secondPatch.AttemptCount)
		assert.InDelta(t, 0.6, secondPatch.AvgConfidence, 0.01) // Average of 0.9, 0.8, 0.1
	})

	t.Run("empty cases should result in empty patches", func(t *testing.T) {
		s := &Summary{ErrorID: "test-error"}

		s.GenerateTopPatches([]Case{}, 5)

		assert.Empty(t, s.TopPatches)
	})
}

func TestSummary_Validation(t *testing.T) {
	tests := []struct {
		name        string
		summary     Summary
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid summary",
			summary: Summary{
				ErrorID:     "java-compilation-error",
				CaseCount:   10,
				SuccessRate: 0.8,
				TopPatches: []PatchSummary{
					{Hash: "hash-1", SuccessRate: 0.9},
				},
				Updated: time.Now(),
			},
			expectError: false,
		},
		{
			name: "missing error ID",
			summary: Summary{
				CaseCount:   5,
				SuccessRate: 0.6,
			},
			expectError: true,
			errorMsg:    "error_id cannot be empty",
		},
		{
			name: "invalid success rate",
			summary: Summary{
				ErrorID:     "error-123",
				SuccessRate: 1.5, // > 1.0
			},
			expectError: true,
			errorMsg:    "success_rate must be between 0.0 and 1.0",
		},
		{
			name: "negative case count",
			summary: Summary{
				ErrorID:   "error-123",
				CaseCount: -1,
			},
			expectError: true,
			errorMsg:    "case_count cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - Summary.Validate() doesn't exist yet
			err := tt.summary.Validate()

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPatchSummary_Validation(t *testing.T) {
	tests := []struct {
		name        string
		patch       PatchSummary
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid patch summary",
			patch: PatchSummary{
				Hash:          "hash-abc123",
				SuccessRate:   0.85,
				AttemptCount:  10,
				AvgConfidence: 0.78,
			},
			expectError: false,
		},
		{
			name: "missing hash",
			patch: PatchSummary{
				SuccessRate: 0.5,
			},
			expectError: true,
			errorMsg:    "hash cannot be empty",
		},
		{
			name: "invalid success rate",
			patch: PatchSummary{
				Hash:        "hash-123",
				SuccessRate: -0.1, // < 0.0
			},
			expectError: true,
			errorMsg:    "success_rate must be between 0.0 and 1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - PatchSummary.Validate() doesn't exist yet
			err := tt.patch.Validate()

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
