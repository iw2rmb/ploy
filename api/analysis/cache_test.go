package analysis

import (
	"testing"
	"time"
)

func TestInMemoryCacheBasicOperations(t *testing.T) {
	cache := NewInMemoryCache()
	result := &AnalysisResult{ID: "r1"}

	if cached, ok := cache.Get("miss"); ok || cached != nil {
		t.Fatalf("expected empty cache miss")
	}

	if err := cache.Set("r1", result, time.Second); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	cached, ok := cache.Get("r1")
	if !ok || cached != result {
		t.Fatalf("Get returned %#v, ok=%v; want original", cached, ok)
	}

	if err := cache.Delete("r1"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if cached, ok := cache.Get("r1"); ok || cached != nil {
		t.Fatalf("expected miss after delete")
	}

	if err := cache.Set("r2", result, time.Second); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear error: %v", err)
	}
	if cached, ok := cache.Get("r2"); ok || cached != nil {
		t.Fatalf("expected miss after clear")
	}

	metrics := cache.GetMetrics()
	if entries := metrics["entries"]; entries != 0 {
		t.Fatalf("entries = %d, want 0", entries)
	}
}

func TestInMemoryCacheExpirationAndMetrics(t *testing.T) {
	cache := NewInMemoryCache()
	result := &AnalysisResult{ID: "expire"}

	if err := cache.Set("expire", result, 10*time.Millisecond); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if _, ok := cache.Get("expire"); !ok {
		t.Fatalf("expected hit before expiry")
	}

	time.Sleep(25 * time.Millisecond)
	if cached, ok := cache.Get("expire"); ok || cached != nil {
		t.Fatalf("expected miss after expiry")
	}

	metrics := cache.GetMetrics()
	if hits := metrics["hits"]; hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
	if misses := metrics["misses"]; misses == 0 {
		t.Fatalf("misses should be > 0 after expirations")
	}
	if rate := metrics["hit_rate"]; rate < 0 || rate > 100 {
		t.Fatalf("hit_rate out of range: %d", rate)
	}
}
