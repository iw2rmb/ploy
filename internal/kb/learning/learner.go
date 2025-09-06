// Package learning orchestrates the KB learning pipeline for error patterns and patches.
//
// This package provides the main learning orchestration for the Knowledge Base system,
// coordinating between storage, fingerprinting, and model components to build
// intelligent recommendations for automated error resolution.
//
// Key components:
//   - KBLearner: Main learning pipeline orchestrator
//   - Config: Learning behavior configuration (thresholds, limits, etc.)
//   - Learning pipeline: Error processing, case management, summary updates
//   - Recommendation engine: Best patch selection based on confidence scores
//
// Learning workflow:
//  1. Process build errors and normalize to canonical signatures
//  2. Deduplicate similar patches using semantic fingerprinting
//  3. Update confidence scores based on historical success rates
//  4. Maintain summary statistics for performance tracking
//  5. Provide high-confidence patch recommendations
//
// Integration points:
//   - Uses internal/kb/storage for data persistence
//   - Uses internal/kb/fingerprint for patch similarity analysis
//   - Uses internal/kb/models for all data structures
//   - Used by transflow pipeline for automated healing
//
// See Also:
//   - internal/kb/storage: For persistence operations
//   - internal/kb/fingerprint: For patch analysis
//   - internal/kb/models: For data structure definitions
package learning

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/iw2rmb/ploy/internal/kb/fingerprint"
	"github.com/iw2rmb/ploy/internal/kb/models"
	"github.com/iw2rmb/ploy/internal/kb/storage"
)

// KBLearner orchestrates the learning pipeline for error patterns and patches
type KBLearner struct {
	storage       *storage.KBStorage
	fingerprinter *fingerprint.PatchFingerprinter
	config        *Config
}

// Config configures the KB learning behavior
type Config struct {
	// Minimum confidence threshold for successful patches
	MinConfidence float64

	// Maximum number of cases to keep per error
	MaxCasesPerError int

	// Similarity threshold for case deduplication (0.0 - 1.0)
	SimilarityThreshold float64

	// Enable debug logging
	EnableDebugLogging bool
}

// NewKBLearner creates a new KB learning pipeline
func NewKBLearner(storage *storage.KBStorage, config *Config) *KBLearner {
	if config == nil {
		config = &Config{
			MinConfidence:       0.7,
			MaxCasesPerError:    50,
			SimilarityThreshold: 0.8,
			EnableDebugLogging:  false,
		}
	}

	return &KBLearner{
		storage:       storage,
		fingerprinter: fingerprint.NewPatchFingerprinter(),
		config:        config,
	}
}

// LearnFromError processes a build error and attempts to learn from any provided patch
func (kbl *KBLearner) LearnFromError(ctx context.Context, errorMsg string, location string, buildLogs []string, patch []byte, success bool) error {
	// Generate canonical error signature
	err := models.NewError(errorMsg, location, buildLogs)

	// Store or update error in KB
	existingError, retrieveErr := kbl.storage.RetrieveError(ctx, err.Signature)
	if retrieveErr != nil {
		// Error doesn't exist, store new one
		if storeErr := kbl.storage.StoreError(ctx, err); storeErr != nil {
			return fmt.Errorf("failed to store error: %w", storeErr)
		}
		kbl.debugLog("Stored new error: %s", err.ID)
	} else {
		err = existingError
		kbl.debugLog("Found existing error: %s", err.ID)
	}

	// If no patch provided, just record the error
	if len(patch) == 0 {
		kbl.debugLog("No patch provided for error %s", err.ID)
		return nil
	}

	// Process the patch and create a learning case
	return kbl.processPatchCase(ctx, err, patch, success)
}

// processPatchCase processes a patch for an error and creates a learning case
func (kbl *KBLearner) processPatchCase(ctx context.Context, err *models.Error, patch []byte, success bool) error {
	// Generate patch fingerprint for logging
	_ = kbl.fingerprinter.GenerateFingerprint(patch)

	// Check for similar existing cases (deduplication)
	existingCases, listErr := kbl.storage.ListCasesByError(ctx, err.ID)
	if listErr != nil {
		return fmt.Errorf("failed to list existing cases: %w", listErr)
	}

	// Find similar case for deduplication
	similarCase := kbl.findSimilarCase(existingCases, patch)

	if similarCase != nil {
		// Update existing similar case
		return kbl.updateSimilarCase(ctx, similarCase, success)
	}

	// Create new case using constructor
	newCase := models.NewCase(err.ID, patch, success)
	newCase.Confidence = kbl.calculateInitialConfidence(success)

	if storeErr := kbl.storage.StoreCase(ctx, newCase); storeErr != nil {
		return fmt.Errorf("failed to store case: %w", storeErr)
	}

	kbl.debugLog("Created new case %s for error %s (success: %t)", newCase.ID, err.ID, success)

	// Update summary statistics
	return kbl.updateErrorSummary(ctx, err.ID)
}

// findSimilarCase finds a case with similar patch content for deduplication
func (kbl *KBLearner) findSimilarCase(cases []*models.Case, patch []byte) *models.Case {
	for _, existingCase := range cases {
		similarity := kbl.fingerprinter.CalculateSimilarity(existingCase.Patch, patch)
		if similarity >= kbl.config.SimilarityThreshold {
			kbl.debugLog("Found similar case with similarity %.2f", similarity)
			return existingCase
		}
	}
	return nil
}

// updateSimilarCase updates an existing case with new success/failure data
func (kbl *KBLearner) updateSimilarCase(ctx context.Context, existingCase *models.Case, success bool) error {
	// Update case confidence based on new outcome
	// Create a pseudo-historical case for confidence calculation
	historicalCases := []models.Case{*existingCase}
	if success {
		// Add a successful pseudo-case to improve confidence
		successCase := models.Case{Success: true}
		historicalCases = append(historicalCases, successCase)
	} else {
		// Add a failed pseudo-case to reduce confidence
		failureCase := models.Case{Success: false}
		historicalCases = append(historicalCases, failureCase)
	}

	// Update confidence based on new outcome
	existingCase.UpdateConfidence(historicalCases)

	kbl.debugLog("Updated case %s: new confidence %.2f",
		existingCase.ID, existingCase.Confidence)

	// Store updated case
	return kbl.storage.StoreCase(ctx, existingCase)
}

// calculateInitialConfidence calculates initial confidence for a new case
func (kbl *KBLearner) calculateInitialConfidence(success bool) float64 {
	if success {
		return 0.8 // High initial confidence for successful patches
	}
	return 0.2 // Low initial confidence for failed patches
}

// updateErrorSummary updates summary statistics for an error
func (kbl *KBLearner) updateErrorSummary(ctx context.Context, errorID string) error {
	cases, err := kbl.storage.ListCasesByError(ctx, errorID)
	if err != nil {
		return fmt.Errorf("failed to list cases for summary: %w", err)
	}

	if len(cases) == 0 {
		return nil
	}

	// Calculate summary statistics
	summary := kbl.calculateSummaryStats(errorID, cases)

	// Store updated summary
	if storeErr := kbl.storage.StoreSummary(ctx, summary); storeErr != nil {
		return fmt.Errorf("failed to store summary: %w", storeErr)
	}

	kbl.debugLog("Updated summary for error %s: %d cases, %.1f%% success rate",
		errorID, summary.CaseCount, summary.SuccessRate*100)

	return nil
}

// calculateSummaryStats calculates summary statistics from cases
func (kbl *KBLearner) calculateSummaryStats(errorID string, cases []*models.Case) *models.Summary {
	successfulCases := 0
	patchStats := make(map[string]*models.PatchSummary)

	for _, c := range cases {
		if c.Success && c.Confidence >= kbl.config.MinConfidence {
			successfulCases++
		}

		// Track patch statistics
		patchHash := c.PatchHash
		if patchStats[patchHash] == nil {
			patchStats[patchHash] = &models.PatchSummary{
				Hash:         patchHash,
				AttemptCount: 0,
				SuccessRate:  0.0,
			}
		}

		patchStats[patchHash].AttemptCount++
	}

	// Calculate success rates for patches
	topPatches := make([]models.PatchSummary, 0, len(patchStats))
	for hash, patchStat := range patchStats {
		// Count successful cases for this patch hash
		successCount := 0
		for _, c := range cases {
			if c.PatchHash == hash && c.Success {
				successCount++
			}
		}

		if patchStat.AttemptCount > 0 {
			patchStat.SuccessRate = float64(successCount) / float64(patchStat.AttemptCount)
		}
		topPatches = append(topPatches, *patchStat)
	}

	// Sort patches by success rate (implementation would need sorting)
	// For now, just take first few patches

	return &models.Summary{
		ErrorID:     errorID,
		CaseCount:   len(cases),
		SuccessRate: float64(successfulCases) / float64(len(cases)),
		TopPatches:  topPatches,
		Updated:     time.Now(),
	}
}

// GetBestPatch retrieves the best patch recommendation for an error
func (kbl *KBLearner) GetBestPatch(ctx context.Context, errorSignature string) (*models.Case, error) {
	// Retrieve error by signature
	err, retrieveErr := kbl.storage.RetrieveError(ctx, errorSignature)
	if retrieveErr != nil {
		return nil, fmt.Errorf("error not found: %w", retrieveErr)
	}

	// Get all cases for this error
	cases, listErr := kbl.storage.ListCasesByError(ctx, err.ID)
	if listErr != nil {
		return nil, fmt.Errorf("failed to list cases: %w", listErr)
	}

	if len(cases) == 0 {
		return nil, fmt.Errorf("no cases found for error")
	}

	// Find case with highest confidence above threshold
	var bestCase *models.Case
	highestConfidence := kbl.config.MinConfidence

	for _, c := range cases {
		if c.Success && c.Confidence > highestConfidence {
			bestCase = c
			highestConfidence = c.Confidence
		}
	}

	if bestCase == nil {
		return nil, fmt.Errorf("no high-confidence patches available")
	}

	kbl.debugLog("Recommending patch with confidence %.2f for error %s",
		bestCase.Confidence, err.ID)

	return bestCase, nil
}

// debugLog logs debug messages if debug logging is enabled
func (kbl *KBLearner) debugLog(format string, args ...interface{}) {
	if kbl.config.EnableDebugLogging {
		log.Printf("[KB-LEARNER] "+format, args...)
	}
}
