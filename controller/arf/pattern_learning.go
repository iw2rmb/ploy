package arf

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// PatternLearningService manages error patterns and learning from transformation failures
type PatternLearningService interface {
	RecordPattern(ctx context.Context, pattern ErrorPatternRecord) error
	FindSimilarPatterns(ctx context.Context, errorContext ErrorContext) ([]SimilarPattern, error)
	LearnFromSuccess(ctx context.Context, success SuccessRecord) error
	GetPatternStatistics(ctx context.Context) (*PatternStatistics, error)
	UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness EffectivenessUpdate) error
	GetRecommendations(ctx context.Context, errorContext ErrorContext) ([]PatternRecommendation, error)
}

// ErrorPatternRecord represents a recorded error pattern with context
type ErrorPatternRecord struct {
	ID               string                 `json:"id"`
	ErrorType        string                 `json:"error_type"`
	ErrorMessage     string                 `json:"error_message"`
	Language         string                 `json:"language"`
	Context          ErrorContext           `json:"context"`
	FrequencyCount   int                    `json:"frequency_count"`
	FirstSeen        time.Time              `json:"first_seen"`
	LastSeen         time.Time              `json:"last_seen"`
	SuccessfulFixes  []SuccessfulFix        `json:"successful_fixes"`
	FailedAttempts   []FailedAttempt        `json:"failed_attempts"`
	Severity         PatternSeverity        `json:"severity"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// Note: ErrorContext type is defined in llm_integration.go

// SuccessfulFix represents a fix that successfully resolved a pattern
type SuccessfulFix struct {
	FixID             string            `json:"fix_id"`
	RecipeID          string            `json:"recipe_id"`
	FixDescription    string            `json:"fix_description"`
	AppliedAt         time.Time         `json:"applied_at"`
	EffectivenessScore float64          `json:"effectiveness_score"`
	ValidationScore   float64           `json:"validation_score"`
	TimeToFix         time.Duration     `json:"time_to_fix"`
	ChangesRequired   int               `json:"changes_required"`
	SideEffects       []string          `json:"side_effects,omitempty"`
	Context           map[string]string `json:"context"`
}

// FailedAttempt represents a failed fix attempt
type FailedAttempt struct {
	AttemptID      string            `json:"attempt_id"`
	RecipeID       string            `json:"recipe_id"`
	FailureReason  string            `json:"failure_reason"`
	AttemptedAt    time.Time         `json:"attempted_at"`
	ErrorsIntroduced []string        `json:"errors_introduced,omitempty"`
	Context        map[string]string `json:"context"`
}

// PatternSeverity indicates how critical a pattern is
type PatternSeverity string

const (
	PatternSeverityLow      PatternSeverity = "low"
	PatternSeverityMedium   PatternSeverity = "medium"
	PatternSeverityHigh     PatternSeverity = "high"
	PatternSeverityCritical PatternSeverity = "critical"
)

// SimilarPattern represents a pattern similar to a given error context
type SimilarPattern struct {
	Pattern          ErrorPatternRecord `json:"pattern"`
	SimilarityScore  float64            `json:"similarity_score"`
	MatchingFactors  []string           `json:"matching_factors"`
	RecommendedFixes []SuccessfulFix    `json:"recommended_fixes"`
	ConfidenceLevel  float64            `json:"confidence_level"`
}

// SuccessRecord represents a successful transformation for learning
type SuccessRecord struct {
	TransformationID string        `json:"transformation_id"`
	RecipeID         string        `json:"recipe_id"`
	ErrorsResolved   []string      `json:"errors_resolved"`
	Context          ErrorContext  `json:"context"`
	ExecutionTime    time.Duration `json:"execution_time"`
	ValidationScore  float64       `json:"validation_score"`
	ChangesApplied   int           `json:"changes_applied"`
	Timestamp        time.Time     `json:"timestamp"`
}

// PatternStatistics provides statistics about learned patterns
type PatternStatistics struct {
	TotalPatterns           int                        `json:"total_patterns"`
	PatternsByLanguage      map[string]int             `json:"patterns_by_language"`
	PatternsByType          map[string]int             `json:"patterns_by_type"`
	PatternsBySeverity      map[PatternSeverity]int    `json:"patterns_by_severity"`
	MostFrequentPatterns    []ErrorPatternRecord       `json:"most_frequent_patterns"`
	RecentPatterns          []ErrorPatternRecord       `json:"recent_patterns"`
	TopSuccessfulFixes      []SuccessfulFix            `json:"top_successful_fixes"`
	AverageFixTime          time.Duration              `json:"average_fix_time"`
	OverallSuccessRate      float64                    `json:"overall_success_rate"`
	GeneratedAt             time.Time                  `json:"generated_at"`
}

// EffectivenessUpdate updates the effectiveness metrics for a pattern fix
type EffectivenessUpdate struct {
	FixID             string    `json:"fix_id"`
	Success           bool      `json:"success"`
	EffectivenessScore float64  `json:"effectiveness_score"`
	ValidationScore   float64   `json:"validation_score"`
	TimeToFix         time.Duration `json:"time_to_fix"`
	SideEffects       []string  `json:"side_effects,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PatternRecommendation provides a recommended approach for handling an error
type PatternRecommendation struct {
	RecommendationID   string            `json:"recommendation_id"`
	RecipeID           string            `json:"recipe_id"`
	Description        string            `json:"description"`
	ConfidenceScore    float64           `json:"confidence_score"`
	EstimatedFixTime   time.Duration     `json:"estimated_fix_time"`
	RiskLevel          RiskLevel         `json:"risk_level"`
	Prerequisites      []string          `json:"prerequisites"`
	ExpectedOutcome    string            `json:"expected_outcome"`
	SimilarPatterns    []string          `json:"similar_patterns"`
	SuccessRate        float64           `json:"success_rate"`
	Context            map[string]string `json:"context"`
}

// DefaultPatternLearningService implements the PatternLearningService interface
type DefaultPatternLearningService struct {
	patterns      map[string]ErrorPatternRecord
	successes     []SuccessRecord
	patternIndex  map[string][]string // Index for fast pattern lookups
	mutex         sync.RWMutex
	
	// Configuration
	similarityThreshold    float64
	maxPatternsToConsider  int
	minFrequencyThreshold  int
	patternExpirationTime  time.Duration
}

// NewPatternLearningService creates a new pattern learning service
func NewPatternLearningService() PatternLearningService {
	return &DefaultPatternLearningService{
		patterns:              make(map[string]ErrorPatternRecord),
		successes:             make([]SuccessRecord, 0),
		patternIndex:          make(map[string][]string),
		similarityThreshold:   0.7,
		maxPatternsToConsider: 100,
		minFrequencyThreshold: 2,
		patternExpirationTime: 90 * 24 * time.Hour, // 90 days
	}
}

// RecordPattern records a new error pattern or updates an existing one
func (pls *DefaultPatternLearningService) RecordPattern(ctx context.Context, pattern ErrorPatternRecord) error {
	pls.mutex.Lock()
	defer pls.mutex.Unlock()
	
	// Validate required fields
	if pattern.ErrorType == "" {
		return fmt.Errorf("error type is required")
	}
	
	if pattern.Language == "" {
		return fmt.Errorf("language is required")
	}
	
	// Generate ID if not provided
	if pattern.ID == "" {
		pattern.ID = pls.generatePatternID(pattern)
	}
	
	// Set timestamps
	now := time.Now()
	if pattern.FirstSeen.IsZero() {
		pattern.FirstSeen = now
	}
	pattern.LastSeen = now
	
	// Check if pattern already exists
	if existing, exists := pls.patterns[pattern.ID]; exists {
		// Update existing pattern
		existing.FrequencyCount++
		existing.LastSeen = now
		
		// Merge failed attempts and successful fixes
		existing.FailedAttempts = append(existing.FailedAttempts, pattern.FailedAttempts...)
		existing.SuccessfulFixes = append(existing.SuccessfulFixes, pattern.SuccessfulFixes...)
		
		// Update severity based on frequency
		existing.Severity = pls.calculateSeverity(existing.FrequencyCount, existing.SuccessfulFixes)
		
		pls.patterns[pattern.ID] = existing
	} else {
		// New pattern
		pattern.FrequencyCount = 1
		if pattern.Severity == "" {
			pattern.Severity = SeverityLow
		}
		
		pls.patterns[pattern.ID] = pattern
		
		// Update indices
		pls.updatePatternIndex(pattern)
	}
	
	return nil
}

// generatePatternID creates a unique ID for a pattern
func (pls *DefaultPatternLearningService) generatePatternID(pattern ErrorPatternRecord) string {
	// Create hash based on key characteristics
	key := fmt.Sprintf("%s_%s_%s_%s", 
		pattern.ErrorType, 
		pattern.Language,
		normalizeErrorMessage(pattern.ErrorMessage),
		pattern.Context.SourceFile)
	
	return fmt.Sprintf("pattern_%x", []byte(key))
}

// normalizeErrorMessage normalizes error messages for pattern matching
func normalizeErrorMessage(message string) string {
	// Remove variable names, line numbers, and other dynamic content
	normalized := strings.ToLower(message)
	
	// Replace common variable patterns
	normalized = strings.ReplaceAll(normalized, "variable", "VAR")
	normalized = strings.ReplaceAll(normalized, "method", "METHOD")
	normalized = strings.ReplaceAll(normalized, "class", "CLASS")
	
	// Remove numbers (line numbers, etc.)
	words := strings.Fields(normalized)
	filtered := make([]string, 0)
	
	for _, word := range words {
		// Keep non-numeric words and common error patterns
		if !isNumeric(word) || isCommonErrorPattern(word) {
			filtered = append(filtered, word)
		}
	}
	
	return strings.Join(filtered, " ")
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// isCommonErrorPattern checks if a word is a common error pattern to keep
func isCommonErrorPattern(word string) bool {
	patterns := []string{"404", "500", "200", "null", "undefined"}
	for _, pattern := range patterns {
		if word == pattern {
			return true
		}
	}
	return false
}

// updatePatternIndex updates search indices for fast pattern lookup
func (pls *DefaultPatternLearningService) updatePatternIndex(pattern ErrorPatternRecord) {
	// Index by error type
	errorTypeKey := "error_type:" + pattern.ErrorType
	pls.patternIndex[errorTypeKey] = append(pls.patternIndex[errorTypeKey], pattern.ID)
	
	// Index by language
	languageKey := "language:" + pattern.Language
	pls.patternIndex[languageKey] = append(pls.patternIndex[languageKey], pattern.ID)
	
	// Index by file extension
	if pattern.Context.SourceFile != "" {
		ext := getFileExtension(pattern.Context.SourceFile)
		if ext != "" {
			extKey := "extension:" + ext
			pls.patternIndex[extKey] = append(pls.patternIndex[extKey], pattern.ID)
		}
	}
}

// getFileExtension extracts file extension from path
func getFileExtension(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) > 1 {
		return strings.ToLower(parts[len(parts)-1])
	}
	return ""
}

// calculateSeverity calculates pattern severity based on frequency and success rate
func (pls *DefaultPatternLearningService) calculateSeverity(frequency int, fixes []SuccessfulFix) PatternSeverity {
	// High frequency patterns are more severe
	if frequency >= 50 {
		return SeverityCritical
	} else if frequency >= 20 {
		return SeverityHigh
	} else if frequency >= 10 {
		return SeverityMedium
	}
	
	// If there are no successful fixes, it's more severe
	if len(fixes) == 0 && frequency >= 5 {
		return SeverityHigh
	}
	
	return SeverityLow
}

// FindSimilarPatterns finds patterns similar to the given error context
func (pls *DefaultPatternLearningService) FindSimilarPatterns(ctx context.Context, errorContext ErrorContext) ([]SimilarPattern, error) {
	pls.mutex.RLock()
	defer pls.mutex.RUnlock()
	
	var candidates []ErrorPatternRecord
	
	// Find potential candidates using indices
	candidateIDs := make(map[string]bool)
	
	// Look for patterns with same file extension
	ext := getFileExtension(errorContext.SourceFile)
	if ext != "" {
		extKey := "extension:" + ext
		for _, id := range pls.patternIndex[extKey] {
			candidateIDs[id] = true
		}
	}
	
	// Look for patterns with same build tool
	if errorContext.Metadata["build_tool"] != "" {
		for _, pattern := range pls.patterns {
			if pattern.Context.Metadata["build_tool"] == errorContext.Metadata["build_tool"] {
				candidateIDs[pattern.ID] = true
			}
		}
	}
	
	// Convert candidates to list
	for id := range candidateIDs {
		if pattern, exists := pls.patterns[id]; exists {
			candidates = append(candidates, pattern)
		}
	}
	
	// If no candidates from indices, consider all patterns (limited)
	if len(candidates) == 0 {
		count := 0
		for _, pattern := range pls.patterns {
			candidates = append(candidates, pattern)
			count++
			if count >= pls.maxPatternsToConsider {
				break
			}
		}
	}
	
	// Calculate similarity scores
	var similarPatterns []SimilarPattern
	
	for _, candidate := range candidates {
		score := pls.calculateSimilarity(errorContext, candidate.Context)
		
		if score >= pls.similarityThreshold {
			// Find recommended fixes (top 3 by effectiveness)
			recommendedFixes := pls.getTopFixes(candidate.SuccessfulFixes, 3)
			
			// Calculate confidence based on pattern maturity and success rate
			confidence := pls.calculateConfidence(candidate, score)
			
			similarPattern := SimilarPattern{
				Pattern:          candidate,
				SimilarityScore:  score,
				MatchingFactors:  pls.identifyMatchingFactors(errorContext, candidate.Context),
				RecommendedFixes: recommendedFixes,
				ConfidenceLevel:  confidence,
			}
			
			similarPatterns = append(similarPatterns, similarPattern)
		}
	}
	
	// Sort by similarity score (highest first)
	sort.Slice(similarPatterns, func(i, j int) bool {
		return similarPatterns[i].SimilarityScore > similarPatterns[j].SimilarityScore
	})
	
	// Limit results
	if len(similarPatterns) > 10 {
		similarPatterns = similarPatterns[:10]
	}
	
	return similarPatterns, nil
}

// calculateSimilarity calculates similarity score between two error contexts
func (pls *DefaultPatternLearningService) calculateSimilarity(ctx1, ctx2 ErrorContext) float64 {
	var score float64
	var factors int
	
	// File path similarity (30% weight)
	if pathSimilarity := pls.calculatePathSimilarity(ctx1.SourceFile, ctx2.SourceFile); pathSimilarity > 0 {
		score += pathSimilarity * 0.3
	}
	factors++
	
	// Build tool similarity (15% weight)
	if getStringValue(ctx1.Metadata, "build_tool") == getStringValue(ctx2.Metadata, "build_tool") && getStringValue(ctx1.Metadata, "build_tool") != "" {
		score += 0.15
	}
	factors++
	
	// Dependencies similarity (20% weight)
	depSimilarity := pls.calculateSliceSimilarity(getStringSliceValue(ctx1.Metadata, "dependencies"), getStringSliceValue(ctx2.Metadata, "dependencies"))
	score += depSimilarity * 0.2
	factors++
	
	// Frameworks similarity (15% weight)
	frameworkSimilarity := pls.calculateSliceSimilarity(getStringSliceValue(ctx1.Metadata, "frameworks"), getStringSliceValue(ctx2.Metadata, "frameworks"))
	score += frameworkSimilarity * 0.15
	factors++
	
	// Surrounding code similarity (20% weight)
	codeSimilarity := pls.calculateCodeSimilarity(getStringValue(ctx1.Metadata, "surrounding_code"), getStringValue(ctx2.Metadata, "surrounding_code"))
	score += codeSimilarity * 0.2
	factors++
	
	return score
}

// calculatePathSimilarity calculates similarity between file paths
func (pls *DefaultPatternLearningService) calculatePathSimilarity(path1, path2 string) float64 {
	if path1 == path2 {
		return 1.0
	}
	
	// Check if same file extension
	ext1 := getFileExtension(path1)
	ext2 := getFileExtension(path2)
	
	if ext1 == ext2 && ext1 != "" {
		return 0.8 // High similarity for same extension
	}
	
	// Check directory structure similarity
	dirs1 := strings.Split(path1, "/")
	dirs2 := strings.Split(path2, "/")
	
	commonDirs := 0
	maxDirs := len(dirs1)
	if len(dirs2) > maxDirs {
		maxDirs = len(dirs2)
	}
	
	minLen := len(dirs1)
	if len(dirs2) < minLen {
		minLen = len(dirs2)
	}
	
	for i := 0; i < minLen; i++ {
		if dirs1[i] == dirs2[i] {
			commonDirs++
		}
	}
	
	if maxDirs > 0 {
		return float64(commonDirs) / float64(maxDirs)
	}
	
	return 0.0
}

// calculateSliceSimilarity calculates similarity between two string slices
func (pls *DefaultPatternLearningService) calculateSliceSimilarity(slice1, slice2 []string) float64 {
	if len(slice1) == 0 && len(slice2) == 0 {
		return 1.0
	}
	
	if len(slice1) == 0 || len(slice2) == 0 {
		return 0.0
	}
	
	// Convert to sets for intersection calculation
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	
	for _, item := range slice1 {
		set1[item] = true
	}
	
	for _, item := range slice2 {
		set2[item] = true
	}
	
	// Calculate intersection
	intersection := 0
	for item := range set1 {
		if set2[item] {
			intersection++
		}
	}
	
	// Calculate union
	union := len(set1) + len(set2) - intersection
	
	if union > 0 {
		return float64(intersection) / float64(union)
	}
	
	return 0.0
}

// calculateCodeSimilarity calculates similarity between code snippets
func (pls *DefaultPatternLearningService) calculateCodeSimilarity(code1, code2 string) float64 {
	if code1 == code2 {
		return 1.0
	}
	
	if code1 == "" || code2 == "" {
		return 0.0
	}
	
	// Simple token-based similarity
	tokens1 := strings.Fields(code1)
	tokens2 := strings.Fields(code2)
	
	return pls.calculateSliceSimilarity(tokens1, tokens2)
}

// identifyMatchingFactors identifies what factors contributed to the match
func (pls *DefaultPatternLearningService) identifyMatchingFactors(ctx1, ctx2 ErrorContext) []string {
	factors := make([]string, 0)
	
	if getFileExtension(ctx1.SourceFile) == getFileExtension(ctx2.SourceFile) {
		factors = append(factors, "same_file_type")
	}
	
	if getStringValue(ctx1.Metadata, "build_tool") == getStringValue(ctx2.Metadata, "build_tool") && getStringValue(ctx1.Metadata, "build_tool") != "" {
		factors = append(factors, "same_build_tool")
	}
	
	if len(getStringSliceValue(ctx1.Metadata, "dependencies")) > 0 && len(getStringSliceValue(ctx2.Metadata, "dependencies")) > 0 {
		commonDeps := pls.calculateSliceSimilarity(getStringSliceValue(ctx1.Metadata, "dependencies"), getStringSliceValue(ctx2.Metadata, "dependencies"))
		if commonDeps > 0.3 {
			factors = append(factors, "similar_dependencies")
		}
	}
	
	if len(getStringSliceValue(ctx1.Metadata, "frameworks")) > 0 && len(getStringSliceValue(ctx2.Metadata, "frameworks")) > 0 {
		commonFrameworks := pls.calculateSliceSimilarity(getStringSliceValue(ctx1.Metadata, "frameworks"), getStringSliceValue(ctx2.Metadata, "frameworks"))
		if commonFrameworks > 0.3 {
			factors = append(factors, "similar_frameworks")
		}
	}
	
	if getStringValue(ctx1.Metadata, "surrounding_code") != "" && getStringValue(ctx2.Metadata, "surrounding_code") != "" {
		codeSim := pls.calculateCodeSimilarity(getStringValue(ctx1.Metadata, "surrounding_code"), getStringValue(ctx2.Metadata, "surrounding_code"))
		if codeSim > 0.4 {
			factors = append(factors, "similar_code_structure")
		}
	}
	
	return factors
}

// getTopFixes returns the top N fixes by effectiveness score
func (pls *DefaultPatternLearningService) getTopFixes(fixes []SuccessfulFix, n int) []SuccessfulFix {
	if len(fixes) == 0 {
		return []SuccessfulFix{}
	}
	
	// Sort by effectiveness score
	sortedFixes := make([]SuccessfulFix, len(fixes))
	copy(sortedFixes, fixes)
	
	sort.Slice(sortedFixes, func(i, j int) bool {
		return sortedFixes[i].EffectivenessScore > sortedFixes[j].EffectivenessScore
	})
	
	if len(sortedFixes) > n {
		return sortedFixes[:n]
	}
	
	return sortedFixes
}

// calculateConfidence calculates confidence level for a similar pattern
func (pls *DefaultPatternLearningService) calculateConfidence(pattern ErrorPatternRecord, similarityScore float64) float64 {
	// Base confidence on similarity score
	confidence := similarityScore
	
	// Adjust based on pattern maturity (frequency)
	frequencyBonus := float64(pattern.FrequencyCount) / 100.0
	if frequencyBonus > 0.3 {
		frequencyBonus = 0.3 // Cap the bonus
	}
	confidence += frequencyBonus
	
	// Adjust based on success rate of fixes
	if len(pattern.SuccessfulFixes) > 0 {
		totalAttempts := len(pattern.SuccessfulFixes) + len(pattern.FailedAttempts)
		if totalAttempts > 0 {
			successRate := float64(len(pattern.SuccessfulFixes)) / float64(totalAttempts)
			confidence = confidence * successRate
		}
	} else if len(pattern.FailedAttempts) > 0 {
		// No successful fixes but has failed attempts - lower confidence
		confidence *= 0.5
	}
	
	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return confidence
}

// LearnFromSuccess records a successful transformation for pattern learning
func (pls *DefaultPatternLearningService) LearnFromSuccess(ctx context.Context, success SuccessRecord) error {
	pls.mutex.Lock()
	defer pls.mutex.Unlock()
	
	// Validate required fields
	if success.TransformationID == "" {
		return fmt.Errorf("transformation ID is required")
	}
	
	if success.RecipeID == "" {
		return fmt.Errorf("recipe ID is required")
	}
	
	// Set timestamp if not provided
	if success.Timestamp.IsZero() {
		success.Timestamp = time.Now()
	}
	
	// Store success record
	pls.successes = append(pls.successes, success)
	
	// Update related patterns with successful fix information
	for _, errorType := range success.ErrorsResolved {
		// Find patterns that match this error type and context
		for id, pattern := range pls.patterns {
			if pls.isPatternMatchForSuccess(pattern, success, errorType) {
				// Add successful fix to pattern
				fix := SuccessfulFix{
					FixID:             fmt.Sprintf("fix_%s_%d", success.TransformationID, len(pattern.SuccessfulFixes)),
					RecipeID:          success.RecipeID,
					FixDescription:    fmt.Sprintf("Recipe %s successfully resolved %s", success.RecipeID, errorType),
					AppliedAt:         success.Timestamp,
					EffectivenessScore: success.ValidationScore,
					ValidationScore:   success.ValidationScore,
					TimeToFix:         success.ExecutionTime,
					ChangesRequired:   success.ChangesApplied,
					Context: map[string]string{
						"transformation_id": success.TransformationID,
						"error_resolved":    errorType,
					},
				}
				
				pattern.SuccessfulFixes = append(pattern.SuccessfulFixes, fix)
				pattern.Severity = pls.calculateSeverity(pattern.FrequencyCount, pattern.SuccessfulFixes)
				
				pls.patterns[id] = pattern
			}
		}
	}
	
	return nil
}

// isPatternMatchForSuccess checks if a pattern matches a successful transformation
func (pls *DefaultPatternLearningService) isPatternMatchForSuccess(pattern ErrorPatternRecord, success SuccessRecord, errorType string) bool {
	// Check if error types match
	if pattern.ErrorType != errorType {
		return false
	}
	
	// Check context similarity
	similarity := pls.calculateSimilarity(pattern.Context, success.Context)
	return similarity > 0.5 // Lower threshold for learning from success
}

// GetPatternStatistics returns comprehensive statistics about learned patterns
func (pls *DefaultPatternLearningService) GetPatternStatistics(ctx context.Context) (*PatternStatistics, error) {
	pls.mutex.RLock()
	defer pls.mutex.RUnlock()
	
	stats := &PatternStatistics{
		TotalPatterns:      len(pls.patterns),
		PatternsByLanguage: make(map[string]int),
		PatternsByType:     make(map[string]int),
		PatternsBySeverity: make(map[PatternSeverity]int),
		GeneratedAt:        time.Now(),
	}
	
	var totalFixTime time.Duration
	var fixCount int
	var successfulFixes int
	var totalAttempts int
	
	// Collect all patterns for analysis
	allPatterns := make([]ErrorPatternRecord, 0, len(pls.patterns))
	allFixes := make([]SuccessfulFix, 0)
	
	for _, pattern := range pls.patterns {
		allPatterns = append(allPatterns, pattern)
		
		// Update counters
		stats.PatternsByLanguage[pattern.Language]++
		stats.PatternsByType[pattern.ErrorType]++
		stats.PatternsBySeverity[pattern.Severity]++
		
		// Collect fix statistics
		successfulFixes += len(pattern.SuccessfulFixes)
		totalAttempts += len(pattern.SuccessfulFixes) + len(pattern.FailedAttempts)
		
		for _, fix := range pattern.SuccessfulFixes {
			allFixes = append(allFixes, fix)
			totalFixTime += fix.TimeToFix
			fixCount++
		}
	}
	
	// Calculate average fix time
	if fixCount > 0 {
		stats.AverageFixTime = time.Duration(int64(totalFixTime) / int64(fixCount))
	}
	
	// Calculate overall success rate
	if totalAttempts > 0 {
		stats.OverallSuccessRate = float64(successfulFixes) / float64(totalAttempts)
	}
	
	// Get most frequent patterns (top 10)
	sort.Slice(allPatterns, func(i, j int) bool {
		return allPatterns[i].FrequencyCount > allPatterns[j].FrequencyCount
	})
	
	stats.MostFrequentPatterns = allPatterns
	if len(stats.MostFrequentPatterns) > 10 {
		stats.MostFrequentPatterns = stats.MostFrequentPatterns[:10]
	}
	
	// Get recent patterns (last 10)
	sort.Slice(allPatterns, func(i, j int) bool {
		return allPatterns[i].LastSeen.After(allPatterns[j].LastSeen)
	})
	
	stats.RecentPatterns = allPatterns
	if len(stats.RecentPatterns) > 10 {
		stats.RecentPatterns = stats.RecentPatterns[:10]
	}
	
	// Get top successful fixes
	sort.Slice(allFixes, func(i, j int) bool {
		return allFixes[i].EffectivenessScore > allFixes[j].EffectivenessScore
	})
	
	stats.TopSuccessfulFixes = allFixes
	if len(stats.TopSuccessfulFixes) > 10 {
		stats.TopSuccessfulFixes = stats.TopSuccessfulFixes[:10]
	}
	
	return stats, nil
}

// UpdatePatternEffectiveness updates effectiveness metrics for a pattern fix
func (pls *DefaultPatternLearningService) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness EffectivenessUpdate) error {
	pls.mutex.Lock()
	defer pls.mutex.Unlock()
	
	pattern, exists := pls.patterns[patternID]
	if !exists {
		return fmt.Errorf("pattern not found: %s", patternID)
	}
	
	// Find and update the specific fix
	updated := false
	for i, fix := range pattern.SuccessfulFixes {
		if fix.FixID == effectiveness.FixID {
			// Update effectiveness metrics
			fix.EffectivenessScore = effectiveness.EffectivenessScore
			fix.ValidationScore = effectiveness.ValidationScore
			fix.TimeToFix = effectiveness.TimeToFix
			fix.SideEffects = effectiveness.SideEffects
			
			pattern.SuccessfulFixes[i] = fix
			updated = true
			break
		}
	}
	
	if !updated && !effectiveness.Success {
		// Add as failed attempt if this was a failure
		failedAttempt := FailedAttempt{
			AttemptID:      effectiveness.FixID,
			FailureReason:  "Effectiveness update indicates failure",
			AttemptedAt:    effectiveness.UpdatedAt,
			ErrorsIntroduced: effectiveness.SideEffects,
		}
		pattern.FailedAttempts = append(pattern.FailedAttempts, failedAttempt)
		updated = true
	}
	
	if updated {
		// Recalculate severity
		pattern.Severity = pls.calculateSeverity(pattern.FrequencyCount, pattern.SuccessfulFixes)
		pls.patterns[patternID] = pattern
	}
	
	return nil
}

// GetRecommendations provides pattern-based recommendations for handling an error
func (pls *DefaultPatternLearningService) GetRecommendations(ctx context.Context, errorContext ErrorContext) ([]PatternRecommendation, error) {
	// Find similar patterns first
	similarPatterns, err := pls.FindSimilarPatterns(ctx, errorContext)
	if err != nil {
		return nil, fmt.Errorf("failed to find similar patterns: %w", err)
	}
	
	var recommendations []PatternRecommendation
	
	for _, similar := range similarPatterns {
		// Create recommendations based on successful fixes
		for _, fix := range similar.RecommendedFixes {
			recommendation := PatternRecommendation{
				RecommendationID: fmt.Sprintf("rec_%s_%s", similar.Pattern.ID, fix.FixID),
				RecipeID:         fix.RecipeID,
				Description:      fix.FixDescription,
				ConfidenceScore:  similar.ConfidenceLevel * fix.EffectivenessScore,
				EstimatedFixTime: fix.TimeToFix,
				RiskLevel:        pls.assessRiskLevel(fix),
				ExpectedOutcome:  fmt.Sprintf("Expected to resolve error with %.0f%% effectiveness", fix.EffectivenessScore*100),
				SimilarPatterns:  []string{similar.Pattern.ID},
				SuccessRate:      fix.EffectivenessScore,
				Context: map[string]string{
					"pattern_id":      similar.Pattern.ID,
					"similarity":      fmt.Sprintf("%.2f", similar.SimilarityScore),
					"pattern_frequency": fmt.Sprintf("%d", similar.Pattern.FrequencyCount),
				},
			}
			
			// Add prerequisites based on fix context
			recommendation.Prerequisites = pls.extractPrerequisites(fix)
			
			recommendations = append(recommendations, recommendation)
		}
	}
	
	// Sort by confidence score (highest first)
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].ConfidenceScore > recommendations[j].ConfidenceScore
	})
	
	// Limit to top 5 recommendations
	if len(recommendations) > 5 {
		recommendations = recommendations[:5]
	}
	
	return recommendations, nil
}

// assessRiskLevel assesses the risk level of applying a fix
func (pls *DefaultPatternLearningService) assessRiskLevel(fix SuccessfulFix) RiskLevel {
	// High number of changes = higher risk
	if fix.ChangesRequired > 20 {
		return RiskLevelHigh
	} else if fix.ChangesRequired > 10 {
		return RiskLevelModerate
	}
	
	// Side effects increase risk
	if len(fix.SideEffects) > 0 {
		return RiskLevelModerate
	}
	
	// Low effectiveness score = higher risk
	if fix.EffectivenessScore < 0.7 {
		return RiskLevelModerate
	}
	
	return RiskLevelLow
}

// extractPrerequisites extracts prerequisites from fix context
func (pls *DefaultPatternLearningService) extractPrerequisites(fix SuccessfulFix) []string {
	prerequisites := make([]string, 0)
	
	// Add generic prerequisites based on context
	if fix.ChangesRequired > 10 {
		prerequisites = append(prerequisites, "Create backup before applying changes")
	}
	
	if fix.TimeToFix > 5*time.Minute {
		prerequisites = append(prerequisites, "Ensure sufficient time for transformation")
	}
	
	if len(fix.SideEffects) > 0 {
		prerequisites = append(prerequisites, "Review potential side effects carefully")
	}
	
	if fix.ValidationScore < 0.8 {
		prerequisites = append(prerequisites, "Manual review recommended after transformation")
	}
	
	return prerequisites
}

// Helper functions for accessing metadata fields
func getStringValue(metadata map[string]interface{}, key string) string {
	if val, ok := metadata[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func getStringSliceValue(metadata map[string]interface{}, key string) []string {
	if val, ok := metadata[key]; ok {
		if slice, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		} else if slice, ok := val.([]string); ok {
			return slice
		}
	}
	return []string{}
}