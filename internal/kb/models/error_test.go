package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestError_GenerateSignature(t *testing.T) {
	tests := []struct {
		name           string
		message        string
		location       string
		buildLogs      []string
		expectedSig    string
		expectedUnique bool
	}{
		{
			name:     "java compilation error",
			message:  "cannot find symbol: class Optional",
			location: "Main.java:15",
			buildLogs: []string{
				"[ERROR] Main.java:[15,24] cannot find symbol",
				"[ERROR]   symbol:   class Optional",
			},
			expectedSig:    "java-compilation-missing-symbol",
			expectedUnique: true,
		},
		{
			name:     "duplicate errors should have same signature",
			message:  "cannot find symbol: class List",
			location: "Service.java:42",
			buildLogs: []string{
				"[ERROR] Service.java:[42,10] cannot find symbol",
				"[ERROR]   symbol:   class List",
			},
			expectedSig:    "java-compilation-missing-symbol",
			expectedUnique: false,
		},
		{
			name:     "different error type",
			message:  "';' expected",
			location: "App.java:23",
			buildLogs: []string{
				"[ERROR] App.java:[23,15] ';' expected",
			},
			expectedSig:    "java-syntax-semicolon",
			expectedUnique: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - Error model doesn't exist yet
			err := NewError(tt.message, tt.location, tt.buildLogs)
			assert.NotNil(t, err)

			signature := err.GenerateSignature()
			assert.NotEmpty(t, signature)
			assert.Equal(t, tt.expectedSig, signature)
			assert.NotEmpty(t, err.ID)
			assert.Equal(t, signature, err.ID) // ID should be based on signature
		})
	}
}

func TestError_Validation(t *testing.T) {
	tests := []struct {
		name        string
		error       Error
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid error",
			error: Error{
				ID:        "java-compilation-error",
				Signature: "java-compilation-missing-symbol",
				Message:   "cannot find symbol",
				Location:  "Main.java:15",
				BuildLogs: []string{"error log"},
				Created:   time.Now(),
			},
			expectError: false,
		},
		{
			name: "missing signature",
			error: Error{
				Message:  "some error",
				Location: "file.java:1",
			},
			expectError: true,
			errorMsg:    "signature cannot be empty",
		},
		{
			name: "missing message",
			error: Error{
				Signature: "java-error",
				Location:  "file.java:1",
			},
			expectError: true,
			errorMsg:    "message cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - Error.Validate() doesn't exist yet
			err := tt.error.Validate()

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestError_Normalization(t *testing.T) {
	t.Run("normalize build logs", func(t *testing.T) {
		// This should fail - normalization methods don't exist yet
		err := &Error{
			Message: "cannot find symbol: class Optional",
			BuildLogs: []string{
				"[INFO] Compiling project",
				"[ERROR] /path/to/Main.java:[15,24] cannot find symbol",
				"[ERROR]   symbol:   class Optional",
				"[INFO] BUILD FAILED",
			},
		}

		normalized := err.NormalizeBuildLogs()

		// Should keep only error logs and normalize paths
		assert.Len(t, normalized, 2)
		assert.Contains(t, normalized[0], "Main.java:[15,24]")
		assert.NotContains(t, normalized[0], "/path/to/")
		assert.Equal(t, "[ERROR]   symbol:   class Optional", normalized[1])
	})

	t.Run("normalize error message", func(t *testing.T) {
		err := &Error{
			Message: "cannot find symbol: class Optional<String>",
		}

		normalized := err.NormalizeMessage()

		// Should remove type parameters and normalize
		assert.Equal(t, "cannot find symbol: class Optional", normalized)
	})
}
