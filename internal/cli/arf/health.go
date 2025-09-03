package arf

import (
	"encoding/json"
	"fmt"

	"github.com/iw2rmb/ploy/api/arf"
)

// Health and cache commands

func handleARFHealthCommand() error {
	url := fmt.Sprintf("%s/arf/health", arfControllerURL)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	var health map[string]interface{}
	if err := json.Unmarshal(response, &health); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("ARF System Health: %s\n", health["status"])
	if components, ok := health["components"].(map[string]interface{}); ok {
		if engine, ok := components["engine"].(map[string]interface{}); ok {
			fmt.Printf("Available Recipes: %.0f\n", engine["available_recipes"])
		}
		if cache, ok := components["cache"].(map[string]interface{}); ok {
			fmt.Printf("Cache Hit Rate: %.2f%%\n", cache["hit_rate"].(float64)*100)
			fmt.Printf("Cache Size: %.0f entries\n", cache["size"])
		}
	}

	return nil
}

func handleARFCacheCommand(args []string) error {
	if len(args) == 0 {
		return getCacheStats()
	}

	action := args[0]
	switch action {
	case "stats":
		return getCacheStats()
	case "clear":
		return clearCache()
	default:
		fmt.Printf("Unknown cache action: %s\n", action)
		fmt.Println("Available actions: stats, clear")
		return nil
	}
}

func getCacheStats() error {
	url := fmt.Sprintf("%s/arf/cache/stats", arfControllerURL)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %w", err)
	}

	var stats arf.ASTCacheStats
	if err := json.Unmarshal(response, &stats); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("AST Cache Statistics:\n")
	fmt.Printf("Hits: %d\n", stats.Hits)
	fmt.Printf("Misses: %d\n", stats.Misses)
	fmt.Printf("Hit Rate: %.2f%%\n", stats.HitRate*100)
	fmt.Printf("Size: %d entries\n", stats.Size)
	fmt.Printf("Memory Usage: %d bytes\n", stats.MemoryUsage)

	return nil
}

func clearCache() error {
	url := fmt.Sprintf("%s/arf/cache/clear", arfControllerURL)
	_, err := makeAPIRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	fmt.Println("Cache cleared successfully")
	return nil
}
