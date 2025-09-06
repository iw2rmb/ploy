package models

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Case represents a single learning case in the KB
type Case struct {
	ID         string    `json:"id"`         // UUID for this learning case
	ErrorID    string    `json:"error_id"`   // Links to Error via signature
	PatchHash  string    `json:"patch_hash"` // Fingerprint of successful patch
	Patch      []byte    `json:"patch"`      // Actual patch content (diff)
	Success    bool      `json:"success"`    // Whether this patch worked
	Confidence float64   `json:"confidence"` // Success probability (0.0-1.0)
	Created    time.Time `json:"created"`
}

// NewCase creates a new Case with generated ID and patch hash
func NewCase(errorID string, patch []byte, success bool) *Case {
	c := &Case{
		ID:      uuid.New().String(),
		ErrorID: errorID,
		Patch:   patch,
		Success: success,
		Created: time.Now(),
	}

	c.PatchHash = c.GeneratePatchHash()
	c.Confidence = 0.5 // Default confidence

	return c
}

// GeneratePatchHash creates a deterministic hash for the patch content
func (c *Case) GeneratePatchHash() string {
	if len(c.Patch) == 0 {
		return "empty-patch-hash"
	}

	hash := sha256.Sum256(c.Patch)
	return fmt.Sprintf("patch-%x", hash[:8])
}

// UpdateConfidence calculates confidence based on historical cases
func (c *Case) UpdateConfidence(historicalCases []Case) {
	if len(historicalCases) == 0 {
		c.Confidence = 0.5 // Default for new patterns
		return
	}

	// Calculate success rate from historical cases
	successCount := 0
	for _, hc := range historicalCases {
		if hc.Success {
			successCount++
		}
	}

	c.Confidence = float64(successCount) / float64(len(historicalCases))
}

// Validate checks if the case is valid
func (c *Case) Validate() error {
	if c.ErrorID == "" {
		return fmt.Errorf("error_id cannot be empty")
	}
	if c.Confidence < 0.0 || c.Confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0")
	}
	if len(c.Patch) == 0 {
		return fmt.Errorf("patch cannot be empty")
	}
	return nil
}
