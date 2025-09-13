package arf

import (
	"fmt"
	"sync"
	"time"
)

// AnalysisCache provides simple in-memory caching for LLM analysis results
type AnalysisCache struct {
	cache       map[string]*CacheEntry
	cacheMutex  sync.RWMutex
	cacheExpiry time.Duration
}

// CacheEntry represents a cached analysis result with timestamp
type CacheEntry struct {
	Result    *LLMAnalysisResult
	Timestamp time.Time
}

// NewAnalysisCache creates a new analysis cache with the given expiry duration
func NewAnalysisCache(expiry time.Duration) *AnalysisCache {
	return &AnalysisCache{
		cache:       make(map[string]*CacheEntry),
		cacheExpiry: expiry,
	}
}

// GenerateCacheKey generates a cache key from error messages
func (ac *AnalysisCache) GenerateCacheKey(errors []string) string {
	// Simple hash of error messages
	return fmt.Sprintf("%v", errors)
}

// GetFromCache retrieves a result from cache if it exists and hasn't expired
func (ac *AnalysisCache) GetFromCache(key string) *LLMAnalysisResult {
	ac.cacheMutex.RLock()
	defer ac.cacheMutex.RUnlock()

	entry, exists := ac.cache[key]
	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Since(entry.Timestamp) > ac.cacheExpiry {
		// Clean up expired entry (will be done properly in cleanup)
		return nil
	}

	return entry.Result
}

// StoreInCache stores a result in cache
func (ac *AnalysisCache) StoreInCache(key string, result *LLMAnalysisResult) {
	ac.cacheMutex.Lock()
	defer ac.cacheMutex.Unlock()

	ac.cache[key] = &CacheEntry{
		Result:    result,
		Timestamp: time.Now(),
	}

	// Simple cache cleanup - in production, use proper TTL
	if len(ac.cache) > 100 {
		// Clear oldest entries
		for k := range ac.cache {
			delete(ac.cache, k)
			if len(ac.cache) <= 50 {
				break
			}
		}
	}
}

// CleanExpiredEntries removes expired entries from the cache
func (ac *AnalysisCache) CleanExpiredEntries() {
	ac.cacheMutex.Lock()
	defer ac.cacheMutex.Unlock()

	now := time.Now()
	for key, entry := range ac.cache {
		if now.Sub(entry.Timestamp) > ac.cacheExpiry {
			delete(ac.cache, key)
		}
	}
}

// GetCacheStats returns cache statistics
func (ac *AnalysisCache) GetCacheStats() map[string]interface{} {
	ac.cacheMutex.RLock()
	defer ac.cacheMutex.RUnlock()

	return map[string]interface{}{
		"total_entries":   len(ac.cache),
		"expiry_duration": ac.cacheExpiry.String(),
	}
}
