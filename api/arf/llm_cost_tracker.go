package arf

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// LLMProvider represents different LLM service providers
type LLMProvider string

const (
	ProviderOpenAI    LLMProvider = "openai"
	ProviderAnthropic LLMProvider = "anthropic"
	ProviderOllama    LLMProvider = "ollama"
	ProviderCustom    LLMProvider = "custom"
)

// LLMModel represents specific model configurations
type LLMModel struct {
	Provider        LLMProvider `json:"provider"`
	Name            string      `json:"name"`
	InputCostPer1K  float64     `json:"input_cost_per_1k"`  // Cost per 1000 input tokens
	OutputCostPer1K float64     `json:"output_cost_per_1k"` // Cost per 1000 output tokens
	MaxTokens       int         `json:"max_tokens"`
	ContextWindow   int         `json:"context_window"`
}

// PredefinedModels contains cost information for common models
var PredefinedModels = map[string]LLMModel{
	"gpt-4": {
		Provider:        ProviderOpenAI,
		Name:            "gpt-4",
		InputCostPer1K:  0.03,
		OutputCostPer1K: 0.06,
		MaxTokens:       8192,
		ContextWindow:   8192,
	},
	"gpt-4-turbo": {
		Provider:        ProviderOpenAI,
		Name:            "gpt-4-turbo",
		InputCostPer1K:  0.01,
		OutputCostPer1K: 0.03,
		MaxTokens:       128000,
		ContextWindow:   128000,
	},
	"gpt-3.5-turbo": {
		Provider:        ProviderOpenAI,
		Name:            "gpt-3.5-turbo",
		InputCostPer1K:  0.0005,
		OutputCostPer1K: 0.0015,
		MaxTokens:       16385,
		ContextWindow:   16385,
	},
	"claude-3-opus": {
		Provider:        ProviderAnthropic,
		Name:            "claude-3-opus",
		InputCostPer1K:  0.015,
		OutputCostPer1K: 0.075,
		MaxTokens:       200000,
		ContextWindow:   200000,
	},
	"claude-3-sonnet": {
		Provider:        ProviderAnthropic,
		Name:            "claude-3-sonnet",
		InputCostPer1K:  0.003,
		OutputCostPer1K: 0.015,
		MaxTokens:       200000,
		ContextWindow:   200000,
	},
	"claude-3-haiku": {
		Provider:        ProviderAnthropic,
		Name:            "claude-3-haiku",
		InputCostPer1K:  0.00025,
		OutputCostPer1K: 0.00125,
		MaxTokens:       200000,
		ContextWindow:   200000,
	},
}

// LLMUsageRecord tracks individual LLM API calls
type LLMUsageRecord struct {
	ID           string        `json:"id"`
	Model        string        `json:"model"`
	Provider     LLMProvider   `json:"provider"`
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	TotalTokens  int           `json:"total_tokens"`
	InputCost    float64       `json:"input_cost"`
	OutputCost   float64       `json:"output_cost"`
	TotalCost    float64       `json:"total_cost"`
	Prompt       string        `json:"prompt"`
	Response     string        `json:"response"`
	CacheHit     bool          `json:"cache_hit"`
	TransformID  string        `json:"transform_id"`
	HealingDepth int           `json:"healing_depth"`
	Timestamp    time.Time     `json:"timestamp"`
	Duration     time.Duration `json:"duration"`
	Error        string        `json:"error,omitempty"`
}

// LLMUsageMetrics aggregates usage statistics
type LLMUsageMetrics struct {
	TotalCalls        int                `json:"total_calls"`
	TotalInputTokens  int                `json:"total_input_tokens"`
	TotalOutputTokens int                `json:"total_output_tokens"`
	TotalCost         float64            `json:"total_cost"`
	CacheHits         int                `json:"cache_hits"`
	CacheMisses       int                `json:"cache_misses"`
	CacheHitRate      float64            `json:"cache_hit_rate"`
	AverageLatency    time.Duration      `json:"average_latency"`
	ModelUsage        map[string]int     `json:"model_usage"`
	HourlyUsage       map[string]float64 `json:"hourly_usage"` // hour -> cost
	DailyUsage        map[string]float64 `json:"daily_usage"`  // date -> cost
	ErrorCount        int                `json:"error_count"`
	LastUpdated       time.Time          `json:"last_updated"`
}

// LLMBudgetConfig defines cost limits and alerting thresholds
type LLMBudgetConfig struct {
	Enabled               bool    `json:"enabled"`
	MaxCostPerTransform   float64 `json:"max_cost_per_transform"`
	MaxCostPerHour        float64 `json:"max_cost_per_hour"`
	MaxCostPerDay         float64 `json:"max_cost_per_day"`
	MaxCostPerMonth       float64 `json:"max_cost_per_month"`
	AlertThresholdPercent float64 `json:"alert_threshold_percent"` // Alert when X% of budget used
	FallbackModel         string  `json:"fallback_model"`          // Switch to cheaper model when over budget
	BlockOnExceed         bool    `json:"block_on_exceed"`         // Block calls when budget exceeded
}

// LLMCacheEntry represents a cached LLM response
type LLMCacheEntry struct {
	Key          string        `json:"key"`
	Prompt       string        `json:"prompt"`
	Response     string        `json:"response"`
	Model        string        `json:"model"`
	Tokens       int           `json:"tokens"`
	Cost         float64       `json:"cost"`
	CreatedAt    time.Time     `json:"created_at"`
	LastAccessed time.Time     `json:"last_accessed"`
	AccessCount  int           `json:"access_count"`
	TTL          time.Duration `json:"ttl"`
}

// LLMCostTracker manages cost tracking, caching, and budget enforcement
type LLMCostTracker struct {
	models              map[string]LLMModel
	usageRecords        []LLMUsageRecord
	metrics             *LLMUsageMetrics
	budgetConfig        *LLMBudgetConfig
	cache               map[string]*LLMCacheEntry
	cacheMutex          sync.RWMutex
	metricsMutex        sync.RWMutex
	alertCallbacks      []func(alert BudgetAlert)
	similarityThreshold float64 // For fuzzy cache matching
}

// BudgetAlert represents a budget threshold alert
type BudgetAlert struct {
	Type        string    `json:"type"` // "transform", "hourly", "daily", "monthly"
	Current     float64   `json:"current"`
	Limit       float64   `json:"limit"`
	Percentage  float64   `json:"percentage"`
	Message     string    `json:"message"`
	Timestamp   time.Time `json:"timestamp"`
	Recommended string    `json:"recommended"` // Recommended action
}

// NewLLMCostTracker creates a new cost tracker with default configuration
func NewLLMCostTracker(budgetConfig *LLMBudgetConfig) *LLMCostTracker {
	if budgetConfig == nil {
		budgetConfig = &LLMBudgetConfig{
			Enabled:               true,
			MaxCostPerTransform:   10.0,   // $10 per transformation
			MaxCostPerHour:        50.0,   // $50 per hour
			MaxCostPerDay:         500.0,  // $500 per day
			MaxCostPerMonth:       5000.0, // $5000 per month
			AlertThresholdPercent: 80.0,   // Alert at 80% usage
			FallbackModel:         "gpt-3.5-turbo",
			BlockOnExceed:         false,
		}
	}

	return &LLMCostTracker{
		models:       PredefinedModels,
		usageRecords: make([]LLMUsageRecord, 0),
		metrics: &LLMUsageMetrics{
			ModelUsage:  make(map[string]int),
			HourlyUsage: make(map[string]float64),
			DailyUsage:  make(map[string]float64),
		},
		budgetConfig:        budgetConfig,
		cache:               make(map[string]*LLMCacheEntry),
		alertCallbacks:      make([]func(BudgetAlert), 0),
		similarityThreshold: 0.85, // 85% similarity for cache hits
	}
}

// EstimateTokens provides a rough estimation of token count
func (t *LLMCostTracker) EstimateTokens(text string) int {
	// Rough estimation: ~4 characters per token for English text
	// This is a simplified approach; production should use tiktoken or similar
	words := strings.Fields(text)
	charCount := len(text)

	// Use the higher of word count * 1.3 or char count / 4
	wordEstimate := int(float64(len(words)) * 1.3)
	charEstimate := charCount / 4

	if wordEstimate > charEstimate {
		return wordEstimate
	}
	return charEstimate
}

// CalculateCost calculates the cost for a given model and token usage
func (t *LLMCostTracker) CalculateCost(modelName string, inputTokens, outputTokens int) (float64, error) {
	model, exists := t.models[modelName]
	if !exists {
		return 0, fmt.Errorf("unknown model: %s", modelName)
	}

	inputCost := (float64(inputTokens) / 1000.0) * model.InputCostPer1K
	outputCost := (float64(outputTokens) / 1000.0) * model.OutputCostPer1K

	return inputCost + outputCost, nil
}

// RecordUsage records an LLM API call and updates metrics
func (t *LLMCostTracker) RecordUsage(ctx context.Context, record LLMUsageRecord) error {
	t.metricsMutex.Lock()
	defer t.metricsMutex.Unlock()

	// Calculate costs if not provided
	if record.TotalCost == 0 {
		cost, err := t.CalculateCost(record.Model, record.InputTokens, record.OutputTokens)
		if err != nil {
			return err
		}
		record.TotalCost = cost
	}

	record.TotalTokens = record.InputTokens + record.OutputTokens
	record.Timestamp = time.Now()

	// Add to records
	t.usageRecords = append(t.usageRecords, record)

	// Update metrics
	t.metrics.TotalCalls++
	t.metrics.TotalInputTokens += record.InputTokens
	t.metrics.TotalOutputTokens += record.OutputTokens
	t.metrics.TotalCost += record.TotalCost

	if record.CacheHit {
		t.metrics.CacheHits++
	} else {
		t.metrics.CacheMisses++
	}

	if record.Error != "" {
		t.metrics.ErrorCount++
	}

	// Update cache hit rate
	if t.metrics.TotalCalls > 0 {
		t.metrics.CacheHitRate = float64(t.metrics.CacheHits) / float64(t.metrics.TotalCalls)
	}

	// Update model usage
	t.metrics.ModelUsage[record.Model]++

	// Update hourly and daily usage
	hour := record.Timestamp.Format("2006-01-02T15")
	day := record.Timestamp.Format("2006-01-02")
	t.metrics.HourlyUsage[hour] += record.TotalCost
	t.metrics.DailyUsage[day] += record.TotalCost

	t.metrics.LastUpdated = time.Now()

	// Check budget and trigger alerts
	if t.budgetConfig.Enabled {
		t.checkBudgetAndAlert()
	}

	return nil
}

// CheckBudget verifies if a new LLM call would exceed budget limits
func (t *LLMCostTracker) CheckBudget(modelName string, estimatedInputTokens int) (bool, string, error) {
	if !t.budgetConfig.Enabled {
		return true, "", nil
	}

	t.metricsMutex.RLock()
	defer t.metricsMutex.RUnlock()

	// Estimate cost for this call
	model, exists := t.models[modelName]
	if !exists {
		return false, "", fmt.Errorf("unknown model: %s", modelName)
	}

	// Assume output tokens will be similar to input for estimation
	estimatedCost := (float64(estimatedInputTokens*2) / 1000.0) *
		((model.InputCostPer1K + model.OutputCostPer1K) / 2)

	// Check hourly budget
	currentHour := time.Now().Format("2006-01-02T15")
	hourlySpent := t.metrics.HourlyUsage[currentHour]
	if hourlySpent+estimatedCost > t.budgetConfig.MaxCostPerHour {
		if t.budgetConfig.BlockOnExceed {
			return false, fmt.Sprintf("would exceed hourly budget: $%.2f + $%.2f > $%.2f",
				hourlySpent, estimatedCost, t.budgetConfig.MaxCostPerHour), nil
		}
	}

	// Check daily budget
	currentDay := time.Now().Format("2006-01-02")
	dailySpent := t.metrics.DailyUsage[currentDay]
	if dailySpent+estimatedCost > t.budgetConfig.MaxCostPerDay {
		if t.budgetConfig.BlockOnExceed {
			return false, fmt.Sprintf("would exceed daily budget: $%.2f + $%.2f > $%.2f",
				dailySpent, estimatedCost, t.budgetConfig.MaxCostPerDay), nil
		}
	}

	return true, "", nil
}

// GetCachedResponse checks cache for similar prompts
func (t *LLMCostTracker) GetCachedResponse(prompt string, model string) (*LLMCacheEntry, bool) {
	t.cacheMutex.Lock()
	defer t.cacheMutex.Unlock()

	// Generate cache key
	key := t.generateCacheKey(prompt, model)

	// Check exact match
	if entry, exists := t.cache[key]; exists {
		// Check if entry is still valid
		if time.Since(entry.CreatedAt) < entry.TTL {
			entry.LastAccessed = time.Now()
			entry.AccessCount++
			return entry, true
		}
		// Entry expired, remove it
		delete(t.cache, key)
	}

	// For similar prompts, could implement fuzzy matching here
	// For now, return exact matches only
	return nil, false
}

// CacheResponse stores an LLM response in cache
func (t *LLMCostTracker) CacheResponse(prompt, response, model string, tokens int, cost float64) {
	t.cacheMutex.Lock()
	defer t.cacheMutex.Unlock()

	key := t.generateCacheKey(prompt, model)

	entry := &LLMCacheEntry{
		Key:          key,
		Prompt:       prompt,
		Response:     response,
		Model:        model,
		Tokens:       tokens,
		Cost:         cost,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
		AccessCount:  0,                // Start at 0, will be incremented on first access
		TTL:          30 * time.Minute, // 30 minutes default TTL
	}

	t.cache[key] = entry

	// Implement cache size limit (LRU eviction)
	if len(t.cache) > 1000 {
		t.evictOldestCacheEntries(100) // Remove 100 oldest entries
	}
}

// GetMetrics returns current usage metrics
func (t *LLMCostTracker) GetMetrics() *LLMUsageMetrics {
	t.metricsMutex.RLock()
	defer t.metricsMutex.RUnlock()

	// Return a copy to avoid concurrent modification
	metricsCopy := *t.metrics
	return &metricsCopy
}

// RegisterAlertCallback registers a function to be called on budget alerts
func (t *LLMCostTracker) RegisterAlertCallback(callback func(BudgetAlert)) {
	t.alertCallbacks = append(t.alertCallbacks, callback)
}

// Private helper methods

func (t *LLMCostTracker) generateCacheKey(prompt, model string) string {
	// Create a hash of prompt and model for cache key
	h := md5.New()
	h.Write([]byte(prompt + "|" + model))
	return hex.EncodeToString(h.Sum(nil))
}

func (t *LLMCostTracker) checkBudgetAndAlert() {
	// Check various budget thresholds and trigger alerts
	currentHour := time.Now().Format("2006-01-02T15")
	currentDay := time.Now().Format("2006-01-02")

	// Check hourly budget
	hourlySpent := t.metrics.HourlyUsage[currentHour]
	if t.budgetConfig.MaxCostPerHour > 0 {
		hourlyPercent := (hourlySpent / t.budgetConfig.MaxCostPerHour) * 100
		if hourlyPercent >= t.budgetConfig.AlertThresholdPercent {
			alert := BudgetAlert{
				Type:        "hourly",
				Current:     hourlySpent,
				Limit:       t.budgetConfig.MaxCostPerHour,
				Percentage:  hourlyPercent,
				Message:     fmt.Sprintf("Hourly LLM cost at %.1f%% of budget", hourlyPercent),
				Timestamp:   time.Now(),
				Recommended: "Consider using cheaper models or enabling more aggressive caching",
			}
			t.triggerAlert(alert)
		}
	}

	// Check daily budget
	dailySpent := t.metrics.DailyUsage[currentDay]
	if t.budgetConfig.MaxCostPerDay > 0 {
		dailyPercent := (dailySpent / t.budgetConfig.MaxCostPerDay) * 100
		if dailyPercent >= t.budgetConfig.AlertThresholdPercent {
			alert := BudgetAlert{
				Type:        "daily",
				Current:     dailySpent,
				Limit:       t.budgetConfig.MaxCostPerDay,
				Percentage:  dailyPercent,
				Message:     fmt.Sprintf("Daily LLM cost at %.1f%% of budget", dailyPercent),
				Timestamp:   time.Now(),
				Recommended: "Review transformation patterns and consider deferring non-critical operations",
			}
			t.triggerAlert(alert)
		}
	}
}

func (t *LLMCostTracker) triggerAlert(alert BudgetAlert) {
	for _, callback := range t.alertCallbacks {
		go callback(alert) // Run callbacks asynchronously
	}
}

func (t *LLMCostTracker) evictOldestCacheEntries(count int) {
	// Find and remove oldest cache entries
	// This is a simple implementation; production might use a proper LRU cache

	if len(t.cache) <= count {
		t.cache = make(map[string]*LLMCacheEntry)
		return
	}

	// Collect all entries with their last access time
	type cacheItem struct {
		key          string
		lastAccessed time.Time
	}

	items := make([]cacheItem, 0, len(t.cache))
	for key, entry := range t.cache {
		items = append(items, cacheItem{key: key, lastAccessed: entry.LastAccessed})
	}

	// Sort by last accessed time (oldest first)
	// Simple bubble sort for small datasets
	for i := 0; i < len(items)-1; i++ {
		for j := 0; j < len(items)-i-1; j++ {
			if items[j].lastAccessed.After(items[j+1].lastAccessed) {
				items[j], items[j+1] = items[j+1], items[j]
			}
		}
	}

	// Remove oldest entries
	for i := 0; i < count && i < len(items); i++ {
		delete(t.cache, items[i].key)
	}
}

// GetTransformationCost returns the total LLM cost for a specific transformation
func (t *LLMCostTracker) GetTransformationCost(transformID string) float64 {
	t.metricsMutex.RLock()
	defer t.metricsMutex.RUnlock()

	var totalCost float64
	for _, record := range t.usageRecords {
		if record.TransformID == transformID {
			totalCost += record.TotalCost
		}
	}
	return totalCost
}

// SuggestOptimalModel suggests the best model based on budget and requirements
func (t *LLMCostTracker) SuggestOptimalModel(requiredTokens int, qualityPriority float64) string {
	// qualityPriority: 0.0 = cheapest, 1.0 = best quality

	t.metricsMutex.RLock()
	currentDay := time.Now().Format("2006-01-02")
	dailySpent := t.metrics.DailyUsage[currentDay]
	remainingBudget := t.budgetConfig.MaxCostPerDay - dailySpent
	t.metricsMutex.RUnlock()

	// If very low budget, always use cheapest
	if remainingBudget < 1.0 {
		return "gpt-3.5-turbo"
	}

	// Based on quality priority, select appropriate model
	if qualityPriority > 0.8 {
		return "gpt-4-turbo"
	} else if qualityPriority > 0.5 {
		return "claude-3-sonnet"
	} else if qualityPriority > 0.3 {
		return "gpt-3.5-turbo"
	}

	return "claude-3-haiku" // Cheapest option
}
