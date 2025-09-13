package recipes

import (
	"fmt"
	"strings"
)

// Helper functions for recipe ID and name generation

func generateRecipeID(recipeClass string) string {
	// Convert class name to ID
	// org.openrewrite.java.migrate.Java11toJava17 -> java11to17
	parts := strings.Split(recipeClass, ".")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Convert CamelCase to lowercase
		id := strings.ToLower(lastPart)
		// Handle special cases
		id = strings.ReplaceAll(id, "upgradetojava", "java")
		id = strings.ReplaceAll(id, "tojava", "to")
		id = strings.ReplaceAll(id, "migration", "")
		id = strings.ReplaceAll(id, "upgrade", "")
		return strings.TrimSuffix(id, "-")
	}
	return "unknown"
}

func generateRecipeName(recipeClass string) string {
	// Convert class name to human-readable name
	// org.openrewrite.java.migrate.Java11toJava17 -> Java 11 to 17 Migration
	parts := strings.Split(recipeClass, ".")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Add spaces to CamelCase
		name := ""
		for i, r := range lastPart {
			if i > 0 && r >= 'A' && r <= 'Z' {
				name += " "
			}
			name += string(r)
		}
		// Clean up known patterns
		name = strings.ReplaceAll(name, "Java11to Java17", "Java 11 to 17")
		name = strings.ReplaceAll(name, "Java8to Java11", "Java 8 to 11")
		name = strings.ReplaceAll(name, "Spring Boot_3_2", "Spring Boot 3.2")
		if !strings.Contains(name, "Migration") && strings.Contains(name, "Java") {
			name += " Migration"
		}
		return name
	}
	return "Unknown Recipe"
}

func generateTags(recipeClass string) []string {
	tags := []string{}
	lower := strings.ToLower(recipeClass)

	if strings.Contains(lower, "java") {
		tags = append(tags, "java")
	}
	if strings.Contains(lower, "spring") {
		tags = append(tags, "spring")
	}
	if strings.Contains(lower, "boot") {
		tags = append(tags, "spring-boot")
	}
	if strings.Contains(lower, "security") {
		tags = append(tags, "security")
	}
	if strings.Contains(lower, "migrate") || strings.Contains(lower, "migration") {
		tags = append(tags, "migration")
	}
	if strings.Contains(lower, "11to17") || strings.Contains(lower, "java17") {
		tags = append(tags, "java17")
	}
	if strings.Contains(lower, "8to11") || strings.Contains(lower, "java11") {
		tags = append(tags, "java11")
	}
	if strings.Contains(lower, "junit") {
		tags = append(tags, "testing", "junit")
	}

	if len(tags) == 0 {
		tags = append(tags, "openrewrite")
	}

	return tags
}

func generateCategories(recipeClass string) []string {
	categories := []string{}
	lower := strings.ToLower(recipeClass)

	if strings.Contains(lower, "java") && (strings.Contains(lower, "migrate") || strings.Contains(lower, "upgrade")) {
		categories = append(categories, "java-migration")
	}
	if strings.Contains(lower, "spring") {
		categories = append(categories, "spring")
	}
	if strings.Contains(lower, "security") {
		categories = append(categories, "security")
	}
	if strings.Contains(lower, "test") || strings.Contains(lower, "junit") {
		categories = append(categories, "testing")
	}
	if strings.Contains(lower, "log") {
		categories = append(categories, "logging")
	}

	if len(categories) == 0 {
		categories = append(categories, "transformation")
	}

	return categories
}

func generateHash(input string) string {
	// Simple hash generation for demo
	// In production, use crypto/sha256
	return fmt.Sprintf("sha256:%x", input)
}
