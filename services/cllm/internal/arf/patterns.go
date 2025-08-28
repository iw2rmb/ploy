package arf

import (
	"log/slog"
	"regexp"
	"strings"
)

// PatternMatcher detects common error patterns in Java transformation errors
type PatternMatcher struct {
	patterns map[string]*ErrorPattern
	logger   *slog.Logger
}

// ErrorPattern represents a known error pattern with resolution strategies
type ErrorPattern struct {
	ID          string
	Name        string
	Category    string
	Regex       *regexp.Regexp
	Keywords    []string
	Confidence  float64
	Description string
	Resolution  PatternResolution
	Examples    []string
	References  []string
}

// NewPatternMatcher creates a new pattern matcher with common Java error patterns
func NewPatternMatcher(logger *slog.Logger) *PatternMatcher {
	pm := &PatternMatcher{
		patterns: make(map[string]*ErrorPattern),
		logger:   logger,
	}
	pm.initializePatterns()
	return pm
}

// FindPatterns analyzes errors and finds matching patterns
func (pm *PatternMatcher) FindPatterns(errors []ErrorDetails, context CodeContext) ([]PatternMatch, error) {
	var matches []PatternMatch
	
	for _, err := range errors {
		for _, pattern := range pm.patterns {
			if pm.matchesPattern(err, pattern) {
				confidence := pm.calculatePatternConfidence(err, pattern, context)
				
				match := PatternMatch{
					PatternID:   pattern.ID,
					PatternName: pattern.Name,
					Confidence:  confidence,
					Description: pattern.Description,
					Category:    pattern.Category,
					Frequency:   1, // Could be enhanced to track frequency
					Resolution:  pattern.Resolution,
					Examples:    pattern.Examples,
					References:  pattern.References,
				}
				matches = append(matches, match)
			}
		}
	}
	
	// Remove duplicates and sort by confidence
	matches = pm.deduplicateMatches(matches)
	
	pm.logger.Debug("Pattern matching completed",
		"patterns_found", len(matches),
		"total_errors", len(errors))
	
	return matches, nil
}

// initializePatterns sets up common Java error patterns
func (pm *PatternMatcher) initializePatterns() {
	patterns := []*ErrorPattern{
		// Java 8 to 11+ migration patterns
		{
			ID:       "java8_to_11_modules",
			Name:     "Module System Migration",
			Category: "migration",
			Regex:    regexp.MustCompile(`(package .* does not exist|module .* not found)`),
			Keywords: []string{"module", "requires", "exports", "package does not exist"},
			Confidence: 0.9,
			Description: "Java 9+ module system compatibility issues when migrating from Java 8",
			Resolution: PatternResolution{
				Strategy:    "configure",
				Steps:       []string{"Add module-info.java", "Configure module dependencies", "Update build configuration"},
				Complexity:  "medium",
				Automated:   false,
				RiskLevel:   "medium",
				TestingTips: []string{"Test module boundaries", "Verify classpath vs modulepath"},
			},
			Examples: []string{
				"Add module-info.java with proper requires clauses",
				"Configure maven-compiler-plugin for module compilation",
			},
			References: []string{
				"https://docs.oracle.com/javase/9/docs/api/java.base/java/lang/module/package-summary.html",
			},
		},
		{
			ID:       "deprecated_api_usage",
			Name:     "Deprecated API Usage",
			Category: "deprecation",
			Regex:    regexp.MustCompile(`.*deprecated.*|.*has been deprecated.*`),
			Keywords: []string{"deprecated", "removal", "forRemoval"},
			Confidence: 0.8,
			Description: "Usage of deprecated APIs that may be removed in newer Java versions",
			Resolution: PatternResolution{
				Strategy:    "replace",
				Steps:       []string{"Identify replacement API", "Update method calls", "Test functionality"},
				Complexity:  "low",
				Automated:   true,
				RiskLevel:   "low",
				TestingTips: []string{"Verify equivalent functionality", "Check performance implications"},
			},
			Examples: []string{
				"Replace Thread.stop() with interrupt mechanism",
				"Use Files.readString() instead of deprecated IOUtils",
			},
		},
		{
			ID:       "javax_to_jakarta",
			Name:     "Javax to Jakarta Migration",
			Category: "framework",
			Regex:    regexp.MustCompile(`javax\.(servlet|persistence|annotation|ejb|jms)`),
			Keywords: []string{"javax.servlet", "javax.persistence", "javax.annotation"},
			Confidence: 0.95,
			Description: "Java EE to Jakarta EE namespace migration required",
			Resolution: PatternResolution{
				Strategy:    "replace",
				Steps:       []string{"Replace javax.* imports with jakarta.*", "Update dependency versions", "Verify compatibility"},
				Complexity:  "medium",
				Automated:   true,
				RiskLevel:   "medium",
				TestingTips: []string{"Test all Jakarta EE features", "Verify server compatibility"},
			},
			Examples: []string{
				"javax.servlet.http.HttpServlet → jakarta.servlet.http.HttpServlet",
				"javax.persistence.Entity → jakarta.persistence.Entity",
			},
			References: []string{
				"https://jakarta.ee/specifications/platform/9/",
			},
		},
		{
			ID:       "missing_dependency",
			Name:     "Missing Dependency",
			Category: "dependency",
			Regex:    regexp.MustCompile(`cannot find symbol|package .* does not exist|class not found`),
			Keywords: []string{"cannot find symbol", "package does not exist", "class not found", "NoClassDefFoundError"},
			Confidence: 0.85,
			Description: "Required dependencies are missing from the project configuration",
			Resolution: PatternResolution{
				Strategy:    "update",
				Steps:       []string{"Identify missing dependency", "Add to build configuration", "Resolve version conflicts"},
				Complexity:  "low",
				Automated:   true,
				RiskLevel:   "low",
				TestingTips: []string{"Verify dependency resolution", "Check for version conflicts"},
			},
			Examples: []string{
				"Add missing Spring Boot dependency",
				"Include required Apache Commons library",
			},
		},
		{
			ID:       "version_conflict",
			Name:     "Dependency Version Conflict",
			Category: "dependency",
			Regex:    regexp.MustCompile(`version conflict|incompatible types|method not found`),
			Keywords: []string{"version conflict", "incompatible", "method not found", "NoSuchMethodError"},
			Confidence: 0.75,
			Description: "Conflicts between different versions of dependencies",
			Resolution: PatternResolution{
				Strategy:    "update",
				Steps:       []string{"Identify conflicting versions", "Use dependency management", "Exclude transitive dependencies"},
				Complexity:  "medium",
				Automated:   false,
				RiskLevel:   "medium",
				TestingTips: []string{"Use mvn dependency:tree", "Test runtime behavior"},
			},
			Examples: []string{
				"Use dependencyManagement to enforce versions",
				"Exclude conflicting transitive dependencies",
			},
		},
		{
			ID:       "java_version_incompatibility",
			Name:     "Java Version Incompatibility",
			Category: "migration",
			Regex:    regexp.MustCompile(`unsupported class file version|compiled by a more recent version`),
			Keywords: []string{"class file version", "unsupported version", "more recent version"},
			Confidence: 0.9,
			Description: "Code compiled with a newer Java version than the runtime",
			Resolution: PatternResolution{
				Strategy:    "configure",
				Steps:       []string{"Update Java runtime version", "Or recompile with target version", "Update build configuration"},
				Complexity:  "low",
				Automated:   false,
				RiskLevel:   "low",
				TestingTips: []string{"Verify Java version consistency", "Test on target environment"},
			},
			Examples: []string{
				"Update maven.compiler.source and target",
				"Use consistent Java version across environments",
			},
		},
		{
			ID:       "spring_boot_migration",
			Name:     "Spring Boot Version Migration",
			Category: "framework",
			Regex:    regexp.MustCompile(`org\.springframework\.|@SpringBootApplication|@EnableAutoConfiguration`),
			Keywords: []string{"springframework", "SpringBootApplication", "ConfigurationProperties"},
			Confidence: 0.8,
			Description: "Spring Boot version migration issues",
			Resolution: PatternResolution{
				Strategy:    "update",
				Steps:       []string{"Update Spring Boot version", "Migrate configuration properties", "Update deprecated annotations"},
				Complexity:  "medium",
				Automated:   false,
				RiskLevel:   "medium",
				TestingTips: []string{"Test auto-configuration", "Verify actuator endpoints", "Check security configuration"},
			},
			Examples: []string{
				"Migrate from @ConfigurationProperties to @ConstructorBinding",
				"Update security configuration for Spring Security 6",
			},
			References: []string{
				"https://spring.io/projects/spring-boot#support",
			},
		},
		{
			ID:       "compilation_encoding",
			Name:     "Character Encoding Issues",
			Category: "configuration",
			Regex:    regexp.MustCompile(`unmappable character|encoding|charset`),
			Keywords: []string{"unmappable character", "encoding", "charset", "UTF-8"},
			Confidence: 0.8,
			Description: "Character encoding issues during compilation",
			Resolution: PatternResolution{
				Strategy:    "configure",
				Steps:       []string{"Set encoding to UTF-8", "Update build configuration", "Verify source file encoding"},
				Complexity:  "low",
				Automated:   true,
				RiskLevel:   "low",
				TestingTips: []string{"Test special characters", "Verify file encoding consistency"},
			},
			Examples: []string{
				"Add -Dfile.encoding=UTF-8 to JVM args",
				"Set maven.compiler.encoding to UTF-8",
			},
		},
		{
			ID:       "generic_type_erasure",
			Name:     "Generic Type Erasure Issues",
			Category: "language",
			Regex:    regexp.MustCompile(`cannot be cast to|ClassCastException|raw types`),
			Keywords: []string{"cannot be cast", "ClassCastException", "raw types", "unchecked"},
			Confidence: 0.7,
			Description: "Issues related to Java generic type erasure and raw types",
			Resolution: PatternResolution{
				Strategy:    "replace",
				Steps:       []string{"Add proper generic type parameters", "Remove raw type usage", "Use bounded wildcards"},
				Complexity:  "medium",
				Automated:   false,
				RiskLevel:   "medium",
				TestingTips: []string{"Test type safety", "Verify cast operations", "Check compiler warnings"},
			},
			Examples: []string{
				"List<String> instead of raw List",
				"Map<String, Object> with proper bounds",
			},
		},
		{
			ID:       "annotation_processing",
			Name:     "Annotation Processing Issues",
			Category: "configuration",
			Regex:    regexp.MustCompile(`annotation.*not.*found|processor.*not.*found|apt`),
			Keywords: []string{"annotation processor", "apt", "processor not found"},
			Confidence: 0.8,
			Description: "Issues with annotation processors during compilation",
			Resolution: PatternResolution{
				Strategy:    "configure",
				Steps:       []string{"Add annotation processor dependency", "Configure compiler plugin", "Enable annotation processing"},
				Complexity:  "medium",
				Automated:   false,
				RiskLevel:   "low",
				TestingTips: []string{"Test generated code", "Verify processor classpath", "Check IDE configuration"},
			},
			Examples: []string{
				"Add Lombok processor dependency",
				"Configure maven-compiler-plugin for annotation processing",
			},
		},
	}
	
	// Add patterns to the matcher
	for _, pattern := range patterns {
		pm.patterns[pattern.ID] = pattern
	}
	
	pm.logger.Debug("Pattern matcher initialized", "patterns_count", len(pm.patterns))
}

// matchesPattern checks if an error matches a specific pattern
func (pm *PatternMatcher) matchesPattern(err ErrorDetails, pattern *ErrorPattern) bool {
	errorText := strings.ToLower(err.Message)
	
	// Check regex match
	if pattern.Regex != nil && pattern.Regex.MatchString(errorText) {
		return true
	}
	
	// Check keyword matches
	matchCount := 0
	for _, keyword := range pattern.Keywords {
		if strings.Contains(errorText, strings.ToLower(keyword)) {
			matchCount++
		}
	}
	
	// Require at least one keyword match for non-regex patterns
	if len(pattern.Keywords) > 0 {
		return matchCount > 0
	}
	
	return false
}

// calculatePatternConfidence calculates confidence for a pattern match with enhanced dependency awareness
func (pm *PatternMatcher) calculatePatternConfidence(err ErrorDetails, pattern *ErrorPattern, context CodeContext) float64 {
	confidence := pattern.Confidence
	
	// Enhanced framework-based confidence adjustment
	if pattern.Category == "framework" {
		confidence = pm.adjustFrameworkConfidence(confidence, pattern, context)
	}
	
	// Enhanced dependency-based confidence adjustment
	if pattern.Category == "dependency" {
		confidence = pm.adjustDependencyConfidence(confidence, pattern, context, err)
	}
	
	// Version-specific pattern detection
	confidence = pm.adjustVersionSpecificConfidence(confidence, pattern, context, err)
	
	// Adjust based on error type priority
	if err.Type == "compilation" && pattern.Category == "dependency" {
		confidence += 0.1
	}
	
	// Adjust based on error severity
	if err.Severity == "error" {
		confidence += 0.05
	} else if err.Severity == "warning" {
		confidence -= 0.05 // Lower confidence for warnings
	}
	
	// Build tool specific adjustments
	confidence = pm.adjustBuildToolConfidence(confidence, pattern, context)
	
	// Cap confidence at 0.95
	if confidence > 0.95 {
		confidence = 0.95
	}
	
	return confidence
}

// adjustFrameworkConfidence adjusts confidence based on framework dependencies
func (pm *PatternMatcher) adjustFrameworkConfidence(confidence float64, pattern *ErrorPattern, context CodeContext) float64 {
	for _, dep := range context.Dependencies {
		groupID := strings.ToLower(dep.GroupID)
		
		// Spring Boot specific patterns
		if strings.Contains(groupID, "spring") {
			if pattern.ID == "spring_boot_migration" {
				confidence += 0.15
				// Higher confidence for specific Spring version migrations
				if pm.isSpringVersionMigration(dep.Version) {
					confidence += 0.1
				}
			}
		}
		
		// Jakarta EE migration patterns
		if strings.Contains(groupID, "jakarta") && pattern.ID == "javax_to_jakarta" {
			confidence += 0.2
		}
		
		// Legacy javax patterns (strong indicator of needed migration)
		if strings.Contains(groupID, "javax") && pattern.ID == "javax_to_jakarta" {
			confidence += 0.25
		}
	}
	
	return confidence
}

// adjustDependencyConfidence adjusts confidence based on dependency analysis
func (pm *PatternMatcher) adjustDependencyConfidence(confidence float64, pattern *ErrorPattern, context CodeContext, err ErrorDetails) float64 {
	// Check if error mentions specific dependencies we can see
	errorLower := strings.ToLower(err.Message)
	
	for _, dep := range context.Dependencies {
		depName := strings.ToLower(dep.ArtifactID)
		
		// If error mentions a dependency we have, it's likely a version conflict
		if strings.Contains(errorLower, depName) && pattern.ID == "version_conflict" {
			confidence += 0.2
		}
		
		// Missing dependency patterns
		if pattern.ID == "missing_dependency" && pm.isDependencyImplied(err.Message, dep) {
			confidence += 0.15
		}
	}
	
	return confidence
}

// adjustVersionSpecificConfidence handles version-specific patterns
func (pm *PatternMatcher) adjustVersionSpecificConfidence(confidence float64, pattern *ErrorPattern, context CodeContext, err ErrorDetails) float64 {
	// Java version specific patterns
	if pattern.ID == "java_version_incompatibility" {
		if context.FrameworkVersion != "" {
			javaVersion := pm.extractJavaVersion(context.FrameworkVersion)
			if javaVersion >= 11 && strings.Contains(err.Message, "class file version") {
				confidence += 0.2
			}
		}
	}
	
	// Java 8 to 11+ migration patterns
	if pattern.ID == "java8_to_11_modules" {
		if context.FrameworkVersion != "" {
			javaVersion := pm.extractJavaVersion(context.FrameworkVersion)
			if javaVersion >= 11 && strings.Contains(err.Message, "module") {
				confidence += 0.25
			}
		}
	}
	
	return confidence
}

// adjustBuildToolConfidence adjusts confidence based on build tool
func (pm *PatternMatcher) adjustBuildToolConfidence(confidence float64, pattern *ErrorPattern, context CodeContext) float64 {
	// Maven-specific patterns
	if context.BuildTool == "maven" {
		if pattern.Category == "dependency" {
			confidence += 0.05 // Maven makes dependency issues more common
		}
	}
	
	// Gradle-specific patterns
	if context.BuildTool == "gradle" {
		if pattern.ID == "version_conflict" {
			confidence += 0.1 // Gradle version conflicts are common
		}
	}
	
	return confidence
}

// Helper methods for enhanced pattern matching

// isSpringVersionMigration checks if this is a known Spring version migration
func (pm *PatternMatcher) isSpringVersionMigration(version string) bool {
	// Common Spring Boot migration versions
	migrationVersions := []string{"2.0", "2.1", "2.2", "2.3", "2.4", "2.5", "2.6", "2.7", "3.0"}
	
	for _, migVer := range migrationVersions {
		if strings.HasPrefix(version, migVer) {
			return true
		}
	}
	return false
}

// isDependencyImplied checks if an error implies a missing dependency
func (pm *PatternMatcher) isDependencyImplied(errorMessage string, dep Dependency) bool {
	errorLower := strings.ToLower(errorMessage)
	groupLower := strings.ToLower(dep.GroupID)
	artifactLower := strings.ToLower(dep.ArtifactID)
	
	// Check if error message contains package names from this dependency
	commonMappings := map[string][]string{
		"spring-core":    {"springframework"},
		"spring-boot":    {"springframework.boot"},
		"jakarta.servlet-api": {"jakarta.servlet"},
		"hibernate-core": {"hibernate", "org.hibernate"},
	}
	
	if packages, exists := commonMappings[artifactLower]; exists {
		for _, pkg := range packages {
			if strings.Contains(errorLower, pkg) {
				return true
			}
		}
	}
	
	// Generic check: if error contains parts of group or artifact ID
	if strings.Contains(errorLower, groupLower) || strings.Contains(errorLower, artifactLower) {
		return true
	}
	
	return false
}

// extractJavaVersion extracts numeric Java version from version string
func (pm *PatternMatcher) extractJavaVersion(versionStr string) int {
	versionStr = strings.ToLower(strings.TrimSpace(versionStr))
	
	// Handle common Java version formats
	if strings.HasPrefix(versionStr, "1.8") || versionStr == "8" {
		return 8
	}
	if strings.HasPrefix(versionStr, "11") || versionStr == "11" {
		return 11
	}
	if strings.HasPrefix(versionStr, "17") || versionStr == "17" {
		return 17
	}
	if strings.HasPrefix(versionStr, "21") || versionStr == "21" {
		return 21
	}
	
	return 8 // Default to Java 8 if unclear
}

// deduplicateMatches removes duplicate pattern matches and sorts by confidence
func (pm *PatternMatcher) deduplicateMatches(matches []PatternMatch) []PatternMatch {
	seen := make(map[string]bool)
	unique := make([]PatternMatch, 0)
	
	for _, match := range matches {
		if !seen[match.PatternID] {
			seen[match.PatternID] = true
			unique = append(unique, match)
		}
	}
	
	// Sort by confidence (highest first)
	for i := 0; i < len(unique)-1; i++ {
		for j := i + 1; j < len(unique); j++ {
			if unique[j].Confidence > unique[i].Confidence {
				unique[i], unique[j] = unique[j], unique[i]
			}
		}
	}
	
	return unique
}

// GetPatternByID retrieves a pattern by its ID
func (pm *PatternMatcher) GetPatternByID(id string) (*ErrorPattern, bool) {
	pattern, exists := pm.patterns[id]
	return pattern, exists
}

// ListPatterns returns all available patterns
func (pm *PatternMatcher) ListPatterns() map[string]*ErrorPattern {
	return pm.patterns
}

// AddCustomPattern adds a custom error pattern
func (pm *PatternMatcher) AddCustomPattern(pattern *ErrorPattern) {
	pm.patterns[pattern.ID] = pattern
	pm.logger.Debug("Added custom pattern", "pattern_id", pattern.ID, "pattern_name", pattern.Name)
}