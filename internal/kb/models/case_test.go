package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCase_GeneratePatchHash(t *testing.T) {
	tests := []struct {
		name      string
		patch     []byte
		expected  string
		different []byte
	}{
		{
			name:      "java import fix patch",
			patch:     []byte("diff --git a/Main.java b/Main.java\n+import java.util.Optional;"),
			expected:  "patch-a69ff695e59e8e04", // Actual hash generated
			different: []byte("diff --git a/Main.java b/Main.java\n+import java.util.List;"),
		},
		{
			name:     "empty patch",
			patch:    []byte(""),
			expected: "empty-patch-hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - Case model doesn't exist yet
			c := &Case{
				Patch: tt.patch,
			}

			hash := c.GeneratePatchHash()
			assert.NotEmpty(t, hash)
			assert.Equal(t, tt.expected, hash)

			// Same patch should generate same hash
			c2 := &Case{Patch: tt.patch}
			hash2 := c2.GeneratePatchHash()
			assert.Equal(t, hash, hash2)

			// Different patch should generate different hash
			if tt.different != nil {
				c3 := &Case{Patch: tt.different}
				hash3 := c3.GeneratePatchHash()
				assert.NotEqual(t, hash, hash3)
			}
		})
	}
}

func TestCase_Validation(t *testing.T) {
	tests := []struct {
		name        string
		case_       Case
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid case",
			case_: Case{
				ID:         "case-123",
				ErrorID:    "java-compilation-error",
				PatchHash:  "patch-hash-abc",
				Patch:      []byte("diff content"),
				Success:    true,
				Confidence: 0.85,
				Created:    time.Now(),
			},
			expectError: false,
		},
		{
			name: "missing error ID",
			case_: Case{
				ID:    "case-123",
				Patch: []byte("diff"),
			},
			expectError: true,
			errorMsg:    "error_id cannot be empty",
		},
		{
			name: "invalid confidence",
			case_: Case{
				ErrorID:    "error-123",
				Confidence: 1.5, // > 1.0
			},
			expectError: true,
			errorMsg:    "confidence must be between 0.0 and 1.0",
		},
		{
			name: "missing patch",
			case_: Case{
				ErrorID: "error-123",
				Patch:   []byte(""),
			},
			expectError: true,
			errorMsg:    "patch cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - Case.Validate() doesn't exist yet
			err := tt.case_.Validate()

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCase_SetPatchHash(t *testing.T) {
	t.Run("auto-generate patch hash on creation", func(t *testing.T) {
		// This should fail - NewCase doesn't exist yet
		patch := []byte("diff --git a/Main.java b/Main.java\n+import java.util.Optional;")
		
		c := NewCase("error-123", patch, true)
		assert.NotNil(t, c)
		assert.NotEmpty(t, c.ID)
		assert.Equal(t, "error-123", c.ErrorID)
		assert.Equal(t, patch, c.Patch)
		assert.True(t, c.Success)
		assert.NotEmpty(t, c.PatchHash)
		assert.True(t, c.Created.After(time.Now().Add(-1*time.Second)))
	})

	t.Run("patch hash should be deterministic", func(t *testing.T) {
		patch := []byte("same patch content")
		
		c1 := NewCase("error-123", patch, true)
		c2 := NewCase("error-456", patch, false) // Different error, same patch
		
		assert.Equal(t, c1.PatchHash, c2.PatchHash, "Same patch should have same hash")
	})
}

func TestCase_UpdateConfidence(t *testing.T) {
	t.Run("calculate confidence from success rate", func(t *testing.T) {
		// This should fail - UpdateConfidence method doesn't exist yet
		c := &Case{
			ErrorID: "error-123",
			Success: true,
		}

		// Mock historical data
		historicalCases := []Case{
			{Success: true},
			{Success: true},
			{Success: false},
			{Success: true},
		}

		c.UpdateConfidence(historicalCases)
		
		// Should calculate confidence based on historical success rate
		// 3 successes out of 4 = 0.75 confidence
		assert.Equal(t, 0.75, c.Confidence)
	})

	t.Run("default confidence for no history", func(t *testing.T) {
		c := &Case{
			ErrorID: "error-new",
			Success: true,
		}

		c.UpdateConfidence([]Case{})
		
		// Should use default confidence for new error patterns
		assert.Equal(t, 0.5, c.Confidence)
	})
}