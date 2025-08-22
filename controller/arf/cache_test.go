package arf

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryMappedCache(t *testing.T) {
	// Create temporary directory for test cache
	tempDir, err := os.MkdirTemp("", "arf-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create cache
	cache, err := NewMemoryMappedCache(tempDir, 1024*1024, 100) // 1MB, 100 entries
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Test basic operations
	t.Run("Put and Get", func(t *testing.T) {
		ast := &AST{
			FilePath: "test.java",
			Language: "java",
			Checksum: "abc123",
			Nodes: map[string]interface{}{
				"type": "CompilationUnit",
				"children": []interface{}{
					map[string]interface{}{
						"type": "ClassDeclaration",
						"name": "Test",
					},
				},
			},
			ParsedAt: time.Now(),
			Size:     1024,
		}

		// Put AST in cache
		err := cache.Put("test-key", ast)
		if err != nil {
			t.Fatalf("Failed to put AST in cache: %v", err)
		}

		// Get AST from cache
		retrieved, found := cache.Get("test-key")
		if !found {
			t.Fatal("AST not found in cache")
		}

		if retrieved.FilePath != ast.FilePath {
			t.Errorf("Expected FilePath %s, got %s", ast.FilePath, retrieved.FilePath)
		}

		if retrieved.Language != ast.Language {
			t.Errorf("Expected Language %s, got %s", ast.Language, retrieved.Language)
		}

		if retrieved.Checksum != ast.Checksum {
			t.Errorf("Expected Checksum %s, got %s", ast.Checksum, retrieved.Checksum)
		}
	})

	t.Run("Get non-existent key", func(t *testing.T) {
		_, found := cache.Get("non-existent-key")
		if found {
			t.Error("Expected key to not be found")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		ast := &AST{
			FilePath: "delete-test.java",
			Language: "java",
			Checksum: "delete123",
			ParsedAt: time.Now(),
			Size:     512,
		}

		// Put and verify
		err := cache.Put("delete-key", ast)
		if err != nil {
			t.Fatalf("Failed to put AST: %v", err)
		}

		_, found := cache.Get("delete-key")
		if !found {
			t.Fatal("AST should be found before deletion")
		}

		// Delete and verify
		err = cache.Delete("delete-key")
		if err != nil {
			t.Fatalf("Failed to delete AST: %v", err)
		}

		_, found = cache.Get("delete-key")
		if found {
			t.Error("AST should not be found after deletion")
		}
	})

	t.Run("Cache stats", func(t *testing.T) {
		// Clear cache first
		cache.Clear()

		// Add some entries
		for i := 0; i < 5; i++ {
			ast := &AST{
				FilePath: "file" + string(rune(i+'0')) + ".java",
				Language: "java",
				Checksum: "checksum" + string(rune(i+'0')),
				ParsedAt: time.Now(),
				Size:     int64(100 * (i + 1)),
			}
			cache.Put("key"+string(rune(i+'0')), ast)
		}

		// Get some entries (hits)
		cache.Get("key0")
		cache.Get("key1")
		cache.Get("key2")

		// Try to get non-existent entries (misses)
		cache.Get("nonexistent1")
		cache.Get("nonexistent2")

		stats := cache.Stats()
		if stats.Hits != 3 {
			t.Errorf("Expected 3 hits, got %d", stats.Hits)
		}

		if stats.Misses != 2 {
			t.Errorf("Expected 2 misses, got %d", stats.Misses)
		}

		if stats.Size != 5 {
			t.Errorf("Expected cache size 5, got %d", stats.Size)
		}

		expectedHitRate := float64(3) / float64(5)
		if stats.HitRate != expectedHitRate {
			t.Errorf("Expected hit rate %f, got %f", expectedHitRate, stats.HitRate)
		}
	})

	t.Run("Cache capacity limits", func(t *testing.T) {
		// Create small cache for testing eviction
		smallCache, err := NewMemoryMappedCache(filepath.Join(tempDir, "small"), 1024, 3) // 1KB, 3 entries
		if err != nil {
			t.Fatalf("Failed to create small cache: %v", err)
		}
		defer smallCache.Close()

		// Add entries beyond capacity
		for i := 0; i < 5; i++ {
			ast := &AST{
				FilePath: "large-file" + string(rune(i+'0')) + ".java",
				Language: "java",
				Checksum: "large-checksum" + string(rune(i+'0')),
				Nodes: map[string]interface{}{
					"data": make([]byte, 200), // Large enough to trigger eviction
				},
				ParsedAt: time.Now(),
				Size:     200,
			}
			smallCache.Put("large-key"+string(rune(i+'0')), ast)
		}

		stats := smallCache.Stats()
		// Should not exceed max entries due to eviction
		if stats.Size > 3 {
			t.Errorf("Cache size should not exceed 3, got %d", stats.Size)
		}
	})

	t.Run("Clear cache", func(t *testing.T) {
		// Add some entries
		for i := 0; i < 3; i++ {
			ast := &AST{
				FilePath: "clear-test" + string(rune(i+'0')) + ".java",
				Language: "java",
				Checksum: "clear" + string(rune(i+'0')),
				ParsedAt: time.Now(),
				Size:     100,
			}
			cache.Put("clear-key"+string(rune(i+'0')), ast)
		}

		// Verify entries exist
		stats := cache.Stats()
		if stats.Size == 0 {
			t.Fatal("Cache should have entries before clear")
		}

		// Clear cache
		err := cache.Clear()
		if err != nil {
			t.Fatalf("Failed to clear cache: %v", err)
		}

		// Verify cache is empty
		stats = cache.Stats()
		if stats.Size != 0 {
			t.Errorf("Cache should be empty after clear, got size %d", stats.Size)
		}

		// Verify entries are gone
		_, found := cache.Get("clear-key0")
		if found {
			t.Error("Entry should not be found after clear")
		}
	})
}

func TestCacheReload(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "arf-cache-reload-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create cache and add entries
	cache1, err := NewMemoryMappedCache(tempDir, 1024*1024, 100)
	if err != nil {
		t.Fatalf("Failed to create first cache: %v", err)
	}

	ast := &AST{
		FilePath: "persistent.java",
		Language: "java",
		Checksum: "persistent123",
		ParsedAt: time.Now(),
		Size:     256,
	}

	err = cache1.Put("persistent-key", ast)
	if err != nil {
		t.Fatalf("Failed to put AST: %v", err)
	}

	// Close first cache
	cache1.Close()

	// Create new cache with same directory
	cache2, err := NewMemoryMappedCache(tempDir, 1024*1024, 100)
	if err != nil {
		t.Fatalf("Failed to create second cache: %v", err)
	}
	defer cache2.Close()

	// Should be able to retrieve entry from previous cache
	retrieved, found := cache2.Get("persistent-key")
	if !found {
		t.Fatal("Entry should be found after cache reload")
	}

	if retrieved.FilePath != ast.FilePath {
		t.Errorf("Expected FilePath %s, got %s", ast.FilePath, retrieved.FilePath)
	}
}

func BenchmarkCachePut(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "arf-cache-bench")
	if err != nil {
		b.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cache, err := NewMemoryMappedCache(tempDir, 100*1024*1024, 10000) // 100MB, 10k entries
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	ast := &AST{
		FilePath: "benchmark.java",
		Language: "java",
		Checksum: "benchmark123",
		Nodes: map[string]interface{}{
			"type": "CompilationUnit",
			"classes": []interface{}{
				map[string]interface{}{
					"name": "BenchmarkClass",
					"methods": []interface{}{
						map[string]interface{}{
							"name": "benchmarkMethod",
							"params": []string{"int", "String"},
						},
					},
				},
			},
		},
		ParsedAt: time.Now(),
		Size:     1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "bench-key-" + string(rune(i%1000+'0')) // Cycle through 1000 keys
		cache.Put(key, ast)
	}
}

func BenchmarkCacheGet(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "arf-cache-get-bench")
	if err != nil {
		b.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cache, err := NewMemoryMappedCache(tempDir, 100*1024*1024, 10000)
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Pre-populate cache
	ast := &AST{
		FilePath: "get-benchmark.java",
		Language: "java",
		Checksum: "getbench123",
		ParsedAt: time.Now(),
		Size:     512,
	}

	for i := 0; i < 1000; i++ {
		key := "get-bench-key-" + string(rune(i+'0'))
		cache.Put(key, ast)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "get-bench-key-" + string(rune(i%1000+'0'))
		cache.Get(key)
	}
}