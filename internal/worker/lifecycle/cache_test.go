package lifecycle

import "testing"

func TestCacheStoreAndCopy(t *testing.T) {
	c := NewCache()
	// Store nested structure
	src := map[string]any{
		"cpu":  map[string]any{"total": 4000.0, "free": 2000.0},
		"list": []any{map[string]any{"k": "v"}, 42, "x"},
	}
	c.Store(src)

	// Mutate original after storing and ensure cache is not affected
	src["cpu"].(map[string]any)["free"] = 0.0
	src["list"].([]any)[0].(map[string]any)["k"] = "mut"

	got, ok := c.LatestStatus()
	if !ok {
		t.Fatal("expected cached status available")
	}
	if got["cpu"].(map[string]any)["free"].(float64) != 2000.0 {
		t.Fatalf("unexpected cpu.free: %#v", got["cpu"])
	}
	if got["list"].([]any)[0].(map[string]any)["k"].(string) != "v" {
		t.Fatalf("deep copy not preserved: %#v", got["list"])
	}

	// Mutate returned copy; cache must remain unchanged
	got["cpu"].(map[string]any)["free"] = 1.0
	got2, ok := c.LatestStatus()
	if !ok {
		t.Fatal("expected cached status available (second read)")
	}
	if got2["cpu"].(map[string]any)["free"].(float64) != 2000.0 {
		t.Fatalf("cache mutated by caller: %#v", got2["cpu"])
	}
}
