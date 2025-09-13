package recipes

import "strings"

// Helper functions for filtering and searching

// matchesFilters checks if a unified recipe metadata matches the given filters
func matchesFilters(unified *UnifiedRecipeMetadata, filters RecipeFilters) bool {
	if filters.Language != "" && !strings.Contains(strings.ToLower(unified.Metadata.Type), strings.ToLower(filters.Language)) {
		return false
	}

	if filters.Category != "" && !containsInSlice(unified.Metadata.Categories, filters.Category) {
		return false
	}

	if len(filters.Tags) > 0 {
		hasAllTags := true
		for _, tag := range filters.Tags {
			if !containsInSlice(unified.Metadata.Tags, tag) {
				hasAllTags = false
				break
			}
		}
		if !hasAllTags {
			return false
		}
	}

	if filters.Author != "" && !strings.EqualFold(unified.Metadata.Author, filters.Author) {
		return false
	}

	// Confidence filtering not applicable for unified metadata
	return true
}

// containsInSlice checks if a slice contains a target string (case-insensitive substring match)
func containsInSlice(slice []string, target string) bool {
	target = strings.ToLower(target)
	for _, item := range slice {
		if strings.Contains(strings.ToLower(item), target) {
			return true
		}
	}
	return false
}
