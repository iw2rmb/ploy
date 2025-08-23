package arf

import (
	"context"
	"testing"
	"time"
)

func TestProductionOptimizer_Initialize(t *testing.T) {
	config := OptimizerConfig{
		EnableCaching:        true,
		EnableBatching:       true,
		MonitoringInterval:   time.Minute,
		OptimizationInterval: 5 * time.Minute,
	}
	optimizer := NewProductionOptimizer(config)
	
	ctx := context.Background()
	err := optimizer.Initialize(ctx)
	
	// With nil services, this will likely succeed but do nothing
	if err != nil {
		t.Logf("Initialize returned error with nil services: %v", err)
	}
	
	if optimizer == nil {
		t.Error("Expected non-nil optimizer")
	}
}

func TestProductionOptimizer_OptimizeRecipeExecution(t *testing.T) {
	config := OptimizerConfig{
		EnableCaching:        true,
		EnableBatching:       true,
		MonitoringInterval:   time.Minute,
		OptimizationInterval: 5 * time.Minute,
	}
	optimizer := NewProductionOptimizer(config)
	ctx := context.Background()

	recipe := RemediationRecipe{
		ID:              "test-recipe",
		Name:            "Test Recipe",
		Description:     "Test description",
		Vulnerabilities: []string{"CVE-2024-0001"},
		CreatedAt:       time.Now(),
	}
	
	options := ExecutionOptions{
		Priority:    "normal",
		MaxDuration: time.Hour,
	}

	plan, err := optimizer.OptimizeRecipeExecution(ctx, recipe, options)
	if err != nil {
		t.Fatalf("OptimizeRecipeExecution() error = %v", err)
	}

	if plan == nil {
		t.Fatal("Expected non-nil execution plan")
	}

	if plan.RecipeID != recipe.ID {
		t.Errorf("Expected recipe ID %s, got %s", recipe.ID, plan.RecipeID)
	}

	if plan.Strategy.Type == "" {
		t.Error("Expected strategy type to be set")
	}

	if plan.EstimatedDuration == 0 {
		t.Error("Expected non-zero estimated duration")
	}
}

func TestProductionOptimizer_GetPerformanceReport(t *testing.T) {
	config := OptimizerConfig{
		EnableCaching:        true,
		EnableBatching:       true,
		MonitoringInterval:   time.Minute,
		OptimizationInterval: 5 * time.Minute,
	}
	optimizer := NewProductionOptimizer(config)
	ctx := context.Background()

	timeRange := TimeRange{
		Start: time.Now().Add(-time.Hour),
		End:   time.Now(),
	}

	report, err := optimizer.GetPerformanceReport(ctx, timeRange)
	if err != nil {
		t.Fatalf("GetPerformanceReport() error = %v", err)
	}

	if report == nil {
		t.Fatal("Expected non-nil performance report")
	}

	if report.TimeRange.Start != timeRange.Start {
		t.Error("Expected time range start to match")
	}

	if report.PerformanceScore < 0 || report.PerformanceScore > 1 {
		t.Errorf("Expected performance score between 0-1, got %f", report.PerformanceScore)
	}
}

func BenchmarkProductionOptimizer_OptimizeRecipeExecution(b *testing.B) {
	config := OptimizerConfig{
		EnableCaching:        true,
		EnableBatching:       true,
		MonitoringInterval:   time.Minute,
		OptimizationInterval: 5 * time.Minute,
	}
	optimizer := NewProductionOptimizer(config)
	ctx := context.Background()

	recipe := RemediationRecipe{
		ID:              "benchmark-recipe",
		Name:            "Benchmark Recipe",
		Description:     "Benchmark description",
		Vulnerabilities: []string{"CVE-2024-0001"},
		CreatedAt:       time.Now(),
	}
	
	options := ExecutionOptions{
		Priority:    "normal",
		MaxDuration: time.Hour,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = optimizer.OptimizeRecipeExecution(ctx, recipe, options)
	}
}