package arf

import (
	"context"
	"testing"
	"time"
)

func TestPatternLearningServiceCreation(t *testing.T) {
	pls := NewPatternLearningService()
	
	if pls == nil {
		t.Fatal("Expected non-nil pattern learning service")
	}
}

func TestPatternRecording(t *testing.T) {
	pls := NewPatternLearningService()
	ctx := context.Background()
	
	pattern := ErrorPatternRecord{
		ErrorType:    "compilation_error",
		ErrorMessage: "cannot resolve symbol",
		Language:     "java",
		Context: ErrorContext{
			SourceFile:   "src/main/java/Main.java",
			LineNumber:   42,
			Metadata: map[string]interface{}{
				"build_tool":    "maven",
				"dependencies": []string{"junit", "mockito"},
			},
		},
		Severity: SeverityMedium,
	}
	
	err := pls.RecordPattern(ctx, pattern)
	if err != nil {
		t.Fatalf("Expected no error recording pattern, got: %v", err)
	}
	
	// Record the same pattern again (should increment frequency)
	err = pls.RecordPattern(ctx, pattern)
	if err != nil {
		t.Fatalf("Expected no error recording duplicate pattern, got: %v", err)
	}
	
	// Get statistics to verify recording
	stats, err := pls.GetPatternStatistics(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting statistics, got: %v", err)
	}
	
	if stats.TotalPatterns != 1 {
		t.Errorf("Expected 1 pattern, got %d", stats.TotalPatterns)
	}
	
	if stats.PatternsByLanguage["java"] != 1 {
		t.Errorf("Expected 1 Java pattern, got %d", stats.PatternsByLanguage["java"])
	}
	
	if stats.PatternsByType["compilation_error"] != 1 {
		t.Errorf("Expected 1 compilation error pattern, got %d", stats.PatternsByType["compilation_error"])
	}
}

func TestPatternValidation(t *testing.T) {
	pls := NewPatternLearningService()
	ctx := context.Background()
	
	// Test missing error type
	invalidPattern1 := ErrorPatternRecord{
		Language: "java",
	}
	
	err := pls.RecordPattern(ctx, invalidPattern1)
	if err == nil {
		t.Error("Expected error for pattern without error type")
	}
	
	// Test missing language
	invalidPattern2 := ErrorPatternRecord{
		ErrorType: "compilation_error",
	}
	
	err = pls.RecordPattern(ctx, invalidPattern2)
	if err == nil {
		t.Error("Expected error for pattern without language")
	}
}

func TestSimilarPatternFinding(t *testing.T) {
	pls := NewPatternLearningService()
	ctx := context.Background()
	
	// Record some patterns
	patterns := []ErrorPatternRecord{
		{
			ErrorType:    "import_error",
			ErrorMessage: "cannot resolve import",
			Language:     "java",
			Context: ErrorContext{
				SourceFile:   "src/main/java/Service.java",
				Metadata: map[string]interface{}{
					"build_tool":    "maven",
					"dependencies": []string{"spring-boot", "junit"},
				},
			},
			SuccessfulFixes: []SuccessfulFix{
				{
					FixID:             "fix-1",
					RecipeID:          "add-import-recipe",
					EffectivenessScore: 0.9,
					ValidationScore:   0.95,
					TimeToFix:         30 * time.Second,
				},
			},
		},
		{
			ErrorType:    "compilation_error",
			ErrorMessage: "method not found",
			Language:     "java",
			Context: ErrorContext{
				SourceFile: "src/main/java/Controller.java",
				Metadata: map[string]interface{}{
					"build_tool": "maven",
				},
			},
		},
		{
			ErrorType:    "import_error",
			ErrorMessage: "package does not exist",
			Language:     "python",
			Context: ErrorContext{
				SourceFile: "src/service.py",
				Metadata: map[string]interface{}{
					"build_tool": "pip",
				},
			},
		},
	}
	
	for _, pattern := range patterns {
		err := pls.RecordPattern(ctx, pattern)
		if err != nil {
			t.Fatalf("Failed to record pattern: %v", err)
		}
	}
	
	// Search for similar patterns
	searchContext := ErrorContext{
		SourceFile: "src/main/java/NewService.java",
		Metadata: map[string]interface{}{
			"build_tool":    "maven",
			"dependencies": []string{"spring-boot", "hibernate"},
		},
	}
	
	similar, err := pls.FindSimilarPatterns(ctx, searchContext)
	if err != nil {
		t.Fatalf("Expected no error finding patterns, got: %v", err)
	}
	
	if len(similar) == 0 {
		t.Error("Expected to find similar patterns")
	}
	
	// Should find Java patterns more similar than Python patterns
	foundJavaPattern := false
	for _, sim := range similar {
		if sim.Pattern.Language == "java" {
			foundJavaPattern = true
			break
		}
	}
	
	if !foundJavaPattern {
		t.Error("Expected to find Java patterns as similar")
	}
	
	// First result should have highest similarity
	if len(similar) > 1 && similar[0].SimilarityScore < similar[1].SimilarityScore {
		t.Error("Expected results to be sorted by similarity score")
	}
}

func TestSuccessLearning(t *testing.T) {
	pls := NewPatternLearningService()
	ctx := context.Background()
	
	// First record a pattern
	pattern := ErrorPatternRecord{
		ErrorType:    "dependency_error",
		ErrorMessage: "package not found",
		Language:     "go",
		Context: ErrorContext{
			SourceFile: "main.go",
			Metadata: map[string]interface{}{
				"build_tool":    "go",
				"dependencies": []string{"gin", "gorm"},
			},
		},
	}
	
	err := pls.RecordPattern(ctx, pattern)
	if err != nil {
		t.Fatalf("Failed to record pattern: %v", err)
	}
	
	// Record a successful transformation
	success := SuccessRecord{
		TransformationID: "transform-123",
		RecipeID:         "dependency-fix-recipe",
		ErrorsResolved:   []string{"dependency_error"},
		Context: ErrorContext{
			SourceFile: "main.go",
			Metadata: map[string]interface{}{
				"build_tool":    "go",
				"dependencies": []string{"gin", "gorm"},
			},
		},
		ExecutionTime:    45 * time.Second,
		ValidationScore:  0.92,
		ChangesApplied:   3,
		Timestamp:        time.Now(),
	}
	
	err = pls.LearnFromSuccess(ctx, success)
	if err != nil {
		t.Fatalf("Expected no error learning from success, got: %v", err)
	}
	
	// The pattern should now have a successful fix
	searchContext := ErrorContext{
		SourceFile: "main.go",
		Metadata: map[string]interface{}{
			"build_tool": "go",
		},
	}
	
	similar, err := pls.FindSimilarPatterns(ctx, searchContext)
	if err != nil {
		t.Fatalf("Expected no error finding patterns after success, got: %v", err)
	}
	
	if len(similar) == 0 {
		t.Error("Expected to find patterns after learning from success")
	}
	
	// Should have recommended fixes
	foundRecommendedFix := false
	for _, sim := range similar {
		if len(sim.RecommendedFixes) > 0 {
			foundRecommendedFix = true
			break
		}
	}
	
	if !foundRecommendedFix {
		t.Error("Expected to find recommended fixes after learning from success")
	}
}

func TestPatternStatistics(t *testing.T) {
	pls := NewPatternLearningService()
	ctx := context.Background()
	
	// Record multiple patterns with different characteristics
	patterns := []ErrorPatternRecord{
		{
			ErrorType: "syntax_error",
			Language:  "javascript",
			Context: ErrorContext{
				SourceFile: "app.js",
				Metadata: map[string]interface{}{
					"build_tool": "npm",
				},
			},
			Severity: SeverityHigh,
		},
		{
			ErrorType: "type_error",
			Language:  "typescript",
			Context: ErrorContext{
				SourceFile: "service.ts",
				Metadata: map[string]interface{}{
					"build_tool": "npm",
				},
			},
			Severity: SeverityMedium,
		},
		{
			ErrorType: "syntax_error",
			Language:  "python",
			Context: ErrorContext{
				SourceFile: "main.py",
				Metadata: map[string]interface{}{
					"build_tool": "pip",
				},
			},
			Severity: SeverityLow,
		},
	}
	
	for _, pattern := range patterns {
		err := pls.RecordPattern(ctx, pattern)
		if err != nil {
			t.Fatalf("Failed to record pattern: %v", err)
		}
	}
	
	// Get statistics
	stats, err := pls.GetPatternStatistics(ctx)
	if err != nil {
		t.Fatalf("Expected no error getting statistics, got: %v", err)
	}
	
	if stats.TotalPatterns != 3 {
		t.Errorf("Expected 3 patterns, got %d", stats.TotalPatterns)
	}
	
	if stats.PatternsByType["syntax_error"] != 2 {
		t.Errorf("Expected 2 syntax error patterns, got %d", stats.PatternsByType["syntax_error"])
	}
	
	if stats.PatternsByLanguage["javascript"] != 1 {
		t.Errorf("Expected 1 JavaScript pattern, got %d", stats.PatternsByLanguage["javascript"])
	}
	
	if stats.PatternsBySeverity[SeverityHigh] != 1 {
		t.Errorf("Expected 1 high severity pattern, got %d", stats.PatternsBySeverity[SeverityHigh])
	}
	
	if stats.GeneratedAt.IsZero() {
		t.Error("Expected statistics timestamp to be set")
	}
}

func TestEffectivenessUpdate(t *testing.T) {
	pls := NewPatternLearningService()
	ctx := context.Background()
	
	// Record pattern with successful fix
	pattern := ErrorPatternRecord{
		ID:        "test-pattern-1",
		ErrorType: "test_error",
		Language:  "java",
		SuccessfulFixes: []SuccessfulFix{
			{
				FixID:             "fix-1",
				RecipeID:          "test-recipe",
				EffectivenessScore: 0.5, // Low initial score
				ValidationScore:   0.6,
			},
		},
	}
	
	err := pls.RecordPattern(ctx, pattern)
	if err != nil {
		t.Fatalf("Failed to record pattern: %v", err)
	}
	
	// Update effectiveness
	effectiveness := EffectivenessUpdate{
		FixID:             "fix-1",
		Success:           true,
		EffectivenessScore: 0.9, // Improved score
		ValidationScore:   0.95,
		TimeToFix:         60 * time.Second,
		UpdatedAt:         time.Now(),
	}
	
	err = pls.UpdatePatternEffectiveness(ctx, "test-pattern-1", effectiveness)
	if err != nil {
		t.Fatalf("Expected no error updating effectiveness, got: %v", err)
	}
	
	// Test updating non-existent pattern
	err = pls.UpdatePatternEffectiveness(ctx, "non-existent", effectiveness)
	if err == nil {
		t.Error("Expected error updating non-existent pattern")
	}
}

func TestRecommendations(t *testing.T) {
	pls := NewPatternLearningService()
	ctx := context.Background()
	
	// Record pattern with successful fixes
	pattern := ErrorPatternRecord{
		ErrorType:    "null_pointer_error",
		ErrorMessage: "null pointer exception",
		Language:     "java",
		Context: ErrorContext{
			SourceFile: "src/Service.java",
			Metadata: map[string]interface{}{
				"build_tool":    "maven",
				"dependencies": []string{"spring"},
			},
		},
		SuccessfulFixes: []SuccessfulFix{
			{
				FixID:             "fix-null-check",
				RecipeID:          "add-null-check-recipe",
				FixDescription:    "Add null checks before object access",
				EffectivenessScore: 0.95,
				ValidationScore:   0.9,
				TimeToFix:         2 * time.Minute,
				ChangesRequired:   5,
			},
			{
				FixID:             "fix-optional",
				RecipeID:          "use-optional-recipe",
				FixDescription:    "Replace nullable types with Optional",
				EffectivenessScore: 0.85,
				ValidationScore:   0.88,
				TimeToFix:         5 * time.Minute,
				ChangesRequired:   15,
			},
		},
	}
	
	err := pls.RecordPattern(ctx, pattern)
	if err != nil {
		t.Fatalf("Failed to record pattern: %v", err)
	}
	
	// Get recommendations for similar context
	errorContext := ErrorContext{
		SourceFile: "src/Controller.java",
		Metadata: map[string]interface{}{
			"build_tool":    "maven",
			"dependencies": []string{"spring", "hibernate"},
		},
	}
	
	recommendations, err := pls.GetRecommendations(ctx, errorContext)
	if err != nil {
		t.Fatalf("Expected no error getting recommendations, got: %v", err)
	}
	
	if len(recommendations) == 0 {
		t.Error("Expected to get recommendations")
	}
	
	// Should be sorted by confidence score
	if len(recommendations) > 1 && recommendations[0].ConfidenceScore < recommendations[1].ConfidenceScore {
		t.Error("Expected recommendations to be sorted by confidence score")
	}
	
	// Check recommendation structure
	rec := recommendations[0]
	if rec.RecipeID == "" {
		t.Error("Expected recommendation to have recipe ID")
	}
	
	if rec.Description == "" {
		t.Error("Expected recommendation to have description")
	}
	
	if rec.ConfidenceScore <= 0 {
		t.Error("Expected positive confidence score")
	}
	
	if rec.EstimatedFixTime <= 0 {
		t.Error("Expected positive estimated fix time")
	}
}

func TestSimilarityCalculations(t *testing.T) {
	pls := NewPatternLearningService().(*DefaultPatternLearningService)
	
	// Test path similarity
	score := pls.calculatePathSimilarity("src/main/java/Service.java", "src/main/java/Controller.java")
	if score <= 0 {
		t.Error("Expected positive similarity score for similar paths")
	}
	
	// Test different file extensions should have lower similarity than same directory same extension
	score = pls.calculatePathSimilarity("src/Service.java", "src/Service.py")
	sameDirSameExtScore := pls.calculatePathSimilarity("src/Service.java", "src/Controller.java")
	if score >= sameDirSameExtScore {
		t.Errorf("Expected lower similarity for different file extensions: %f >= %f", score, sameDirSameExtScore)
	}
	
	// Test slice similarity
	slice1 := []string{"spring", "junit", "mockito"}
	slice2 := []string{"spring", "hibernate", "junit"}
	
	score = pls.calculateSliceSimilarity(slice1, slice2)
	if score <= 0 {
		t.Error("Expected positive similarity for overlapping slices")
	}
	
	// Empty slices should have similarity of 1.0
	score = pls.calculateSliceSimilarity([]string{}, []string{})
	if score != 1.0 {
		t.Errorf("Expected similarity of 1.0 for empty slices, got %f", score)
	}
	
	// No overlap should have similarity of 0
	score = pls.calculateSliceSimilarity([]string{"a"}, []string{"b"})
	if score != 0.0 {
		t.Errorf("Expected similarity of 0.0 for no overlap, got %f", score)
	}
}

func TestPatternIDGeneration(t *testing.T) {
	pls := NewPatternLearningService().(*DefaultPatternLearningService)
	
	pattern1 := ErrorPatternRecord{
		ErrorType:    "compilation_error",
		ErrorMessage: "cannot resolve symbol foo",
		Language:     "java",
		Context: ErrorContext{
			SourceFile: "src/Service.java",
		},
	}
	
	pattern2 := ErrorPatternRecord{
		ErrorType:    "compilation_error",
		ErrorMessage: "cannot resolve symbol bar",
		Language:     "java",
		Context: ErrorContext{
			SourceFile: "src/Service.java",
		},
	}
	
	id1 := pls.generatePatternID(pattern1)
	id2 := pls.generatePatternID(pattern2)
	
	if id1 == "" {
		t.Error("Expected non-empty pattern ID")
	}
	
	if id1 == id2 {
		t.Error("Expected different IDs for different patterns")
	}
	
	// Same pattern should generate same ID
	id3 := pls.generatePatternID(pattern1)
	if id1 != id3 {
		t.Error("Expected same ID for same pattern")
	}
}

func TestErrorMessageNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"Cannot resolve symbol at line 42",
			"cannot resolve symbol at line",
		},
		{
			"Variable name123 not found",
			"VAR name123 not found",
		},
		{
			"Method execute() does not exist",
			"METHOD execute() does not exist",
		},
		{
			"Class MyClass undefined",
			"CLASS myCLASS undefined",
		},
	}
	
	for _, test := range tests {
		result := normalizeErrorMessage(test.input)
		if result != test.expected {
			t.Errorf("Expected '%s', got '%s' for input '%s'", test.expected, result, test.input)
		}
	}
}

func TestPatternRiskAssessment(t *testing.T) {
	pls := NewPatternLearningService().(*DefaultPatternLearningService)
	
	// Low risk fix
	lowRiskFix := SuccessfulFix{
		ChangesRequired:   3,
		EffectivenessScore: 0.9,
		SideEffects:      []string{},
	}
	
	risk := pls.assessRiskLevel(lowRiskFix)
	if risk != RiskLevelLow {
		t.Errorf("Expected low risk, got %s", risk)
	}
	
	// High risk fix (many changes)
	highRiskFix := SuccessfulFix{
		ChangesRequired:   25,
		EffectivenessScore: 0.8,
		SideEffects:      []string{},
	}
	
	risk = pls.assessRiskLevel(highRiskFix)
	if risk != RiskLevelHigh {
		t.Errorf("Expected high risk for many changes, got %s", risk)
	}
	
	// Medium risk fix (side effects)
	mediumRiskFix := SuccessfulFix{
		ChangesRequired:   5,
		EffectivenessScore: 0.9,
		SideEffects:      []string{"may affect performance"},
	}
	
	risk = pls.assessRiskLevel(mediumRiskFix)
	if risk != RiskLevelModerate {
		t.Errorf("Expected moderate risk for side effects, got %s", risk)
	}
}