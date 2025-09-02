package arf

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestLLMCostTracker_EstimateTokens(t *testing.T) {
	tracker := NewLLMCostTracker(nil)

	tests := []struct {
		name      string
		text      string
		minTokens int
		maxTokens int
	}{
		{
			name:      "empty string",
			text:      "",
			minTokens: 0,
			maxTokens: 0,
		},
		{
			name:      "simple sentence",
			text:      "Hello world, this is a test.",
			minTokens: 5,
			maxTokens: 15,
		},
		{
			name:      "code snippet",
			text:      "func main() { fmt.Println(\"Hello, World!\") }",
			minTokens: 8,
			maxTokens: 20,
		},
		{
			name:      "long text",
			text:      "The quick brown fox jumps over the lazy dog. This is a longer sentence to test token estimation accuracy for various text lengths.",
			minTokens: 20,
			maxTokens: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tracker.EstimateTokens(tt.text)
			if tokens < tt.minTokens || tokens > tt.maxTokens {
				t.Errorf("EstimateTokens() = %d, want between %d and %d", tokens, tt.minTokens, tt.maxTokens)
			}
		})
	}
}

func TestLLMCostTracker_CalculateCost(t *testing.T) {
	tracker := NewLLMCostTracker(nil)

	tests := []struct {
		name         string
		model        string
		inputTokens  int
		outputTokens int
		expectedCost float64
		expectError  bool
	}{
		{
			name:         "GPT-4 standard usage",
			model:        "gpt-4",
			inputTokens:  1000,
			outputTokens: 500,
			expectedCost: 0.03 + 0.03, // $0.03 per 1k input + $0.06 per 1k * 0.5
		},
		{
			name:         "GPT-3.5 turbo usage",
			model:        "gpt-3.5-turbo",
			inputTokens:  2000,
			outputTokens: 1000,
			expectedCost: 0.001 + 0.0015, // $0.0005 * 2 + $0.0015 * 1
		},
		{
			name:         "Claude-3 Opus usage",
			model:        "claude-3-opus",
			inputTokens:  5000,
			outputTokens: 2000,
			expectedCost: 0.075 + 0.15, // $0.015 * 5 + $0.075 * 2
		},
		{
			name:         "Unknown model",
			model:        "unknown-model",
			inputTokens:  1000,
			outputTokens: 1000,
			expectedCost: 0,
			expectError:  true,
		},
		{
			name:         "Zero tokens",
			model:        "gpt-4",
			inputTokens:  0,
			outputTokens: 0,
			expectedCost: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, err := tracker.CalculateCost(tt.model, tt.inputTokens, tt.outputTokens)

			if tt.expectError {
				if err == nil {
					t.Errorf("CalculateCost() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("CalculateCost() unexpected error: %v", err)
				return
			}

			// Allow small floating point differences
			if diff := cost - tt.expectedCost; diff < -0.0001 || diff > 0.0001 {
				t.Errorf("CalculateCost() = %.4f, want %.4f", cost, tt.expectedCost)
			}
		})
	}
}

func TestLLMCostTracker_RecordUsage(t *testing.T) {
	tracker := NewLLMCostTracker(nil)
	ctx := context.Background()

	// Record multiple usage records
	records := []LLMUsageRecord{
		{
			Model:        "gpt-4",
			InputTokens:  1000,
			OutputTokens: 500,
			CacheHit:     false,
			TransformID:  "transform-1",
		},
		{
			Model:        "gpt-3.5-turbo",
			InputTokens:  2000,
			OutputTokens: 1000,
			CacheHit:     true,
			TransformID:  "transform-1",
		},
		{
			Model:        "gpt-4",
			InputTokens:  500,
			OutputTokens: 250,
			CacheHit:     false,
			TransformID:  "transform-2",
			Error:        "API timeout",
		},
	}

	for _, record := range records {
		if err := tracker.RecordUsage(ctx, record); err != nil {
			t.Fatalf("RecordUsage() error: %v", err)
		}
	}

	// Verify metrics
	metrics := tracker.GetMetrics()

	if metrics.TotalCalls != 3 {
		t.Errorf("TotalCalls = %d, want 3", metrics.TotalCalls)
	}

	if metrics.TotalInputTokens != 3500 {
		t.Errorf("TotalInputTokens = %d, want 3500", metrics.TotalInputTokens)
	}

	if metrics.TotalOutputTokens != 1750 {
		t.Errorf("TotalOutputTokens = %d, want 1750", metrics.TotalOutputTokens)
	}

	if metrics.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", metrics.CacheHits)
	}

	if metrics.CacheMisses != 2 {
		t.Errorf("CacheMisses = %d, want 2", metrics.CacheMisses)
	}

	expectedCacheHitRate := 1.0 / 3.0
	if diff := metrics.CacheHitRate - expectedCacheHitRate; diff < -0.01 || diff > 0.01 {
		t.Errorf("CacheHitRate = %.2f, want %.2f", metrics.CacheHitRate, expectedCacheHitRate)
	}

	if metrics.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", metrics.ErrorCount)
	}

	if metrics.ModelUsage["gpt-4"] != 2 {
		t.Errorf("ModelUsage[gpt-4] = %d, want 2", metrics.ModelUsage["gpt-4"])
	}

	if metrics.ModelUsage["gpt-3.5-turbo"] != 1 {
		t.Errorf("ModelUsage[gpt-3.5-turbo] = %d, want 1", metrics.ModelUsage["gpt-3.5-turbo"])
	}
}

func TestLLMCostTracker_BudgetChecking(t *testing.T) {
	budgetConfig := &LLMBudgetConfig{
		Enabled:               true,
		MaxCostPerTransform:   1.0,
		MaxCostPerHour:        0.1, // Very low for testing
		MaxCostPerDay:         1.0,
		AlertThresholdPercent: 80.0,
		BlockOnExceed:         true,
	}

	tracker := NewLLMCostTracker(budgetConfig)
	ctx := context.Background()

	// Record usage that approaches hourly limit
	record := LLMUsageRecord{
		Model:        "gpt-4",
		InputTokens:  1000,
		OutputTokens: 500,
		TotalCost:    0.06, // $0.06
	}

	if err := tracker.RecordUsage(ctx, record); err != nil {
		t.Fatalf("RecordUsage() error: %v", err)
	}

	// Check if next call would exceed budget
	allowed, reason, err := tracker.CheckBudget("gpt-4", 1000)
	if err != nil {
		t.Fatalf("CheckBudget() error: %v", err)
	}

	if allowed {
		t.Errorf("CheckBudget() = true, want false (should block due to hourly limit)")
	}

	if reason == "" {
		t.Errorf("CheckBudget() reason is empty, want explanation")
	}
}

func TestLLMCostTracker_Caching(t *testing.T) {
	tracker := NewLLMCostTracker(nil)

	prompt := "Analyze this error: undefined symbol"
	response := "The undefined symbol error typically means..."
	model := "gpt-4"

	// Cache a response
	tracker.CacheResponse(prompt, response, model, 100, 0.01)

	// Try to retrieve it
	entry, found := tracker.GetCachedResponse(prompt, model)
	if !found {
		t.Errorf("GetCachedResponse() found = false, want true")
	}

	if entry == nil {
		t.Fatalf("GetCachedResponse() entry = nil, want non-nil")
	}

	if entry.Response != response {
		t.Errorf("GetCachedResponse() response = %s, want %s", entry.Response, response)
	}

	if entry.AccessCount != 1 {
		t.Errorf("GetCachedResponse() AccessCount = %d, want 1", entry.AccessCount)
	}

	// Try with different model (should not find)
	_, found = tracker.GetCachedResponse(prompt, "gpt-3.5-turbo")
	if found {
		t.Errorf("GetCachedResponse() with different model found = true, want false")
	}

	// Try with different prompt (should not find)
	_, found = tracker.GetCachedResponse("Different prompt", model)
	if found {
		t.Errorf("GetCachedResponse() with different prompt found = true, want false")
	}
}

func TestLLMCostTracker_CacheExpiration(t *testing.T) {
	tracker := NewLLMCostTracker(nil)

	prompt := "Test prompt"
	response := "Test response"
	model := "gpt-4"

	// Cache a response with very short TTL
	tracker.CacheResponse(prompt, response, model, 100, 0.01)

	// Manually set TTL to expired
	tracker.cacheMutex.Lock()
	key := tracker.generateCacheKey(prompt, model)
	if entry, exists := tracker.cache[key]; exists {
		entry.TTL = 1 * time.Nanosecond
		entry.CreatedAt = time.Now().Add(-1 * time.Hour) // Expired
	}
	tracker.cacheMutex.Unlock()

	// Should not find expired entry
	_, found := tracker.GetCachedResponse(prompt, model)
	if found {
		t.Errorf("GetCachedResponse() found = true for expired entry, want false")
	}
}

func TestLLMCostTracker_BudgetAlerts(t *testing.T) {
	alertReceived := false
	var receivedAlert BudgetAlert

	budgetConfig := &LLMBudgetConfig{
		Enabled:               true,
		MaxCostPerHour:        0.1,
		MaxCostPerDay:         10.0, // High daily budget so only hourly triggers
		AlertThresholdPercent: 50.0, // Alert at 50% for testing
		BlockOnExceed:         false,
	}

	tracker := NewLLMCostTracker(budgetConfig)

	// Register alert callback
	tracker.RegisterAlertCallback(func(alert BudgetAlert) {
		// Only capture hourly alerts for this test
		if alert.Type == "hourly" {
			alertReceived = true
			receivedAlert = alert
		}
	})

	ctx := context.Background()

	// Record usage that triggers alert (> 50% of hourly budget)
	record := LLMUsageRecord{
		Model:        "gpt-4",
		InputTokens:  1000,
		OutputTokens: 500,
		TotalCost:    0.06, // 60% of $0.1 hourly budget
	}

	if err := tracker.RecordUsage(ctx, record); err != nil {
		t.Fatalf("RecordUsage() error: %v", err)
	}

	// Give alert callback time to execute
	time.Sleep(100 * time.Millisecond)

	if !alertReceived {
		t.Errorf("Budget alert not received")
	}

	if receivedAlert.Type != "hourly" {
		t.Errorf("Alert type = %s, want hourly", receivedAlert.Type)
	}

	if receivedAlert.Percentage < 50.0 {
		t.Errorf("Alert percentage = %.1f, want >= 50.0", receivedAlert.Percentage)
	}
}

func TestLLMCostTracker_GetTransformationCost(t *testing.T) {
	tracker := NewLLMCostTracker(nil)
	ctx := context.Background()

	// Record usage for different transformations
	records := []LLMUsageRecord{
		{
			Model:        "gpt-4",
			InputTokens:  1000,
			OutputTokens: 500,
			TotalCost:    0.06,
			TransformID:  "transform-1",
		},
		{
			Model:        "gpt-3.5-turbo",
			InputTokens:  2000,
			OutputTokens: 1000,
			TotalCost:    0.002,
			TransformID:  "transform-1",
		},
		{
			Model:        "gpt-4",
			InputTokens:  500,
			OutputTokens: 250,
			TotalCost:    0.03,
			TransformID:  "transform-2",
		},
	}

	for _, record := range records {
		if err := tracker.RecordUsage(ctx, record); err != nil {
			t.Fatalf("RecordUsage() error: %v", err)
		}
	}

	// Check transformation costs
	cost1 := tracker.GetTransformationCost("transform-1")
	expectedCost1 := 0.062
	if diff := cost1 - expectedCost1; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("GetTransformationCost(transform-1) = %.4f, want %.4f", cost1, expectedCost1)
	}

	cost2 := tracker.GetTransformationCost("transform-2")
	expectedCost2 := 0.03
	if diff := cost2 - expectedCost2; diff < -0.0001 || diff > 0.0001 {
		t.Errorf("GetTransformationCost(transform-2) = %.4f, want %.4f", cost2, expectedCost2)
	}

	// Non-existent transformation
	cost3 := tracker.GetTransformationCost("transform-3")
	if cost3 != 0 {
		t.Errorf("GetTransformationCost(transform-3) = %.4f, want 0", cost3)
	}
}

func TestLLMCostTracker_SuggestOptimalModel(t *testing.T) {
	budgetConfig := &LLMBudgetConfig{
		Enabled:       true,
		MaxCostPerDay: 10.0,
	}
	tracker := NewLLMCostTracker(budgetConfig)

	tests := []struct {
		name            string
		dailySpent      float64
		requiredTokens  int
		qualityPriority float64
		expectedModel   string
	}{
		{
			name:            "high quality priority",
			dailySpent:      0,
			requiredTokens:  1000,
			qualityPriority: 0.9,
			expectedModel:   "gpt-4-turbo",
		},
		{
			name:            "medium quality priority",
			dailySpent:      0,
			requiredTokens:  1000,
			qualityPriority: 0.6,
			expectedModel:   "claude-3-sonnet",
		},
		{
			name:            "low quality priority",
			dailySpent:      0,
			requiredTokens:  1000,
			qualityPriority: 0.2,
			expectedModel:   "claude-3-haiku",
		},
		{
			name:            "very low budget remaining",
			dailySpent:      9.5, // Only $0.5 remaining
			requiredTokens:  1000,
			qualityPriority: 0.9,
			expectedModel:   "gpt-3.5-turbo", // Should use cheapest despite high priority
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set daily spent
			if tt.dailySpent > 0 {
				tracker.metricsMutex.Lock()
				currentDay := time.Now().Format("2006-01-02")
				tracker.metrics.DailyUsage[currentDay] = tt.dailySpent
				tracker.metricsMutex.Unlock()
			}

			model := tracker.SuggestOptimalModel(tt.requiredTokens, tt.qualityPriority)
			if model != tt.expectedModel {
				t.Errorf("SuggestOptimalModel() = %s, want %s", model, tt.expectedModel)
			}
		})
	}
}

func TestLLMCostTracker_CacheEviction(t *testing.T) {
	tracker := NewLLMCostTracker(nil)

	// Add many cache entries to trigger eviction
	for i := 0; i < 1100; i++ {
		prompt := fmt.Sprintf("Prompt %d", i)
		response := fmt.Sprintf("Response %d", i)
		tracker.CacheResponse(prompt, response, "gpt-4", 100, 0.01)
	}

	// Cache should be limited to 1000 entries
	tracker.cacheMutex.RLock()
	cacheSize := len(tracker.cache)
	tracker.cacheMutex.RUnlock()

	if cacheSize > 1000 {
		t.Errorf("Cache size = %d, want <= 1000", cacheSize)
	}
}

func BenchmarkLLMCostTracker_EstimateTokens(b *testing.B) {
	tracker := NewLLMCostTracker(nil)
	text := "The quick brown fox jumps over the lazy dog. This is a sample text for benchmarking token estimation."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.EstimateTokens(text)
	}
}

func BenchmarkLLMCostTracker_RecordUsage(b *testing.B) {
	tracker := NewLLMCostTracker(nil)
	ctx := context.Background()

	record := LLMUsageRecord{
		Model:        "gpt-4",
		InputTokens:  1000,
		OutputTokens: 500,
		TransformID:  "benchmark-transform",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tracker.RecordUsage(ctx, record)
	}
}

func BenchmarkLLMCostTracker_CacheOperations(b *testing.B) {
	tracker := NewLLMCostTracker(nil)

	b.Run("CacheWrite", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			prompt := fmt.Sprintf("Prompt %d", i%100) // Reuse some keys
			tracker.CacheResponse(prompt, "Response", "gpt-4", 100, 0.01)
		}
	})

	b.Run("CacheRead", func(b *testing.B) {
		// Pre-populate cache
		for i := 0; i < 100; i++ {
			prompt := fmt.Sprintf("Prompt %d", i)
			tracker.CacheResponse(prompt, "Response", "gpt-4", 100, 0.01)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			prompt := fmt.Sprintf("Prompt %d", i%100)
			tracker.GetCachedResponse(prompt, "gpt-4")
		}
	})
}
