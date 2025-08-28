package analysis

import (
	"fmt"
	"regexp"
	"strings"
)

// PatternDatabase holds all error patterns for matching
type PatternDatabase struct {
	patterns map[string][]*ErrorPattern
}

// NewPatternDatabase creates a new pattern database with default patterns
func NewPatternDatabase() *PatternDatabase {
	db := &PatternDatabase{
		patterns: make(map[string][]*ErrorPattern),
	}
	db.loadDefaultPatterns()
	return db
}

// loadDefaultPatterns loads the default error patterns
func (db *PatternDatabase) loadDefaultPatterns() {
	// Java compilation error patterns
	db.addJavaCompilationPatterns()
	
	// Java runtime error patterns
	db.addJavaRuntimePatterns()
	
	// Java migration patterns (Java 11 to 17)
	db.addJavaMigrationPatterns()
	
	// Common code quality patterns
	db.addCodeQualityPatterns()
}

// addJavaCompilationPatterns adds Java compilation error patterns
func (db *PatternDatabase) addJavaCompilationPatterns() {
	javaPatterns := []*ErrorPattern{
		{
			ID:                 "java_missing_semicolon",
			Name:               "Missing Semicolon",
			Description:        "Statement is missing a semicolon",
			Category:           "compilation",
			Language:           "java",
			Pattern:            `';' expected|expected ';'`,
			Keywords:           []string{"semicolon", "expected"},
			Severity:           "high",
			SuggestionTemplate: "Add a semicolon at the end of line %d",
		},
		{
			ID:                 "java_undefined_symbol",
			Name:               "Undefined Symbol",
			Description:        "Symbol cannot be resolved",
			Category:           "compilation",
			Language:           "java",
			Pattern:            `cannot find symbol|symbol:\s+(\w+)|cannot resolve symbol`,
			Keywords:           []string{"cannot", "find", "symbol", "resolve"},
			Severity:           "high",
			SuggestionTemplate: "Check if '%s' is properly imported or defined",
		},
		{
			ID:                 "java_incompatible_types",
			Name:               "Incompatible Types",
			Description:        "Type mismatch in assignment or method call",
			Category:           "compilation",
			Language:           "java",
			Pattern:            `incompatible types|cannot be converted to|type mismatch`,
			Keywords:           []string{"incompatible", "types", "cannot", "converted"},
			Severity:           "high",
			SuggestionTemplate: "Check type compatibility between %s and %s",
		},
		{
			ID:                 "java_missing_return",
			Name:               "Missing Return Statement",
			Description:        "Method is missing a return statement",
			Category:           "compilation",
			Language:           "java",
			Pattern:            `missing return statement|must return a result`,
			Keywords:           []string{"missing", "return", "statement"},
			Severity:           "high",
			SuggestionTemplate: "Add a return statement in method '%s'",
		},
		{
			ID:                 "java_unreachable_code",
			Name:               "Unreachable Code",
			Description:        "Code is unreachable",
			Category:           "compilation",
			Language:           "java",
			Pattern:            `unreachable statement|dead code`,
			Keywords:           []string{"unreachable", "dead", "code"},
			Severity:           "medium",
			SuggestionTemplate: "Remove or refactor unreachable code at line %d",
		},
		{
			ID:                 "java_duplicate_class",
			Name:               "Duplicate Class",
			Description:        "Class is already defined",
			Category:           "compilation",
			Language:           "java",
			Pattern:            `duplicate class|class .+ is already defined`,
			Keywords:           []string{"duplicate", "class", "already", "defined"},
			Severity:           "high",
			SuggestionTemplate: "Rename or remove duplicate class '%s'",
		},
		{
			ID:                 "java_package_not_found",
			Name:               "Package Not Found",
			Description:        "Package does not exist",
			Category:           "compilation",
			Language:           "java",
			Pattern:            `package .+ does not exist|cannot find package`,
			Keywords:           []string{"package", "does", "not", "exist"},
			Severity:           "high",
			SuggestionTemplate: "Check if package '%s' is in classpath or add dependency",
		},
	}
	
	db.patterns["java"] = append(db.patterns["java"], javaPatterns...)
}

// addJavaRuntimePatterns adds Java runtime error patterns
func (db *PatternDatabase) addJavaRuntimePatterns() {
	runtimePatterns := []*ErrorPattern{
		{
			ID:                 "java_null_pointer",
			Name:               "Null Pointer Exception",
			Description:        "Attempting to use null reference",
			Category:           "runtime",
			Language:           "java",
			Pattern:            `NullPointerException|Cannot invoke .+ because .+ is null`,
			Keywords:           []string{"NullPointerException", "null", "invoke"},
			Severity:           "critical",
			SuggestionTemplate: "Add null check before accessing '%s'",
		},
		{
			ID:                 "java_class_not_found",
			Name:               "Class Not Found",
			Description:        "Class not found at runtime",
			Category:           "runtime",
			Language:           "java",
			Pattern:            `ClassNotFoundException|NoClassDefFoundError`,
			Keywords:           []string{"ClassNotFoundException", "NoClassDefFoundError"},
			Severity:           "critical",
			SuggestionTemplate: "Ensure class '%s' is in runtime classpath",
		},
		{
			ID:                 "java_array_index_bounds",
			Name:               "Array Index Out of Bounds",
			Description:        "Array index is out of bounds",
			Category:           "runtime",
			Language:           "java",
			Pattern:            `ArrayIndexOutOfBoundsException|Index \d+ out of bounds`,
			Keywords:           []string{"ArrayIndexOutOfBoundsException", "index", "bounds"},
			Severity:           "high",
			SuggestionTemplate: "Check array bounds before accessing index %d",
		},
		{
			ID:                 "java_illegal_argument",
			Name:               "Illegal Argument",
			Description:        "Method received illegal argument",
			Category:           "runtime",
			Language:           "java",
			Pattern:            `IllegalArgumentException|illegal argument`,
			Keywords:           []string{"IllegalArgumentException", "illegal", "argument"},
			Severity:           "high",
			SuggestionTemplate: "Validate arguments before calling method '%s'",
		},
		{
			ID:                 "java_concurrent_modification",
			Name:               "Concurrent Modification",
			Description:        "Collection modified during iteration",
			Category:           "runtime",
			Language:           "java",
			Pattern:            `ConcurrentModificationException`,
			Keywords:           []string{"ConcurrentModificationException", "concurrent"},
			Severity:           "high",
			SuggestionTemplate: "Use iterator.remove() or concurrent collection",
		},
		{
			ID:                 "java_out_of_memory",
			Name:               "Out of Memory",
			Description:        "JVM ran out of memory",
			Category:           "runtime",
			Language:           "java",
			Pattern:            `OutOfMemoryError|Java heap space`,
			Keywords:           []string{"OutOfMemoryError", "heap", "space"},
			Severity:           "critical",
			SuggestionTemplate: "Increase heap size or optimize memory usage",
		},
	}
	
	db.patterns["java"] = append(db.patterns["java"], runtimePatterns...)
}

// addJavaMigrationPatterns adds Java 11 to 17 migration patterns
func (db *PatternDatabase) addJavaMigrationPatterns() {
	migrationPatterns := []*ErrorPattern{
		{
			ID:                 "java_text_blocks",
			Name:               "Text Blocks Available",
			Description:        "Multi-line strings can use text blocks (Java 15+)",
			Category:           "migration",
			Language:           "java",
			Pattern:            `".*\\n.*"|StringBuilder.*append.*\\n`,
			Keywords:           []string{"multiline", "string", "concatenation"},
			Severity:           "low",
			SuggestionTemplate: "Consider using text blocks (''') for multi-line strings",
		},
		{
			ID:                 "java_switch_expression",
			Name:               "Switch Expression Available",
			Description:        "Traditional switch can be converted to switch expression (Java 14+)",
			Category:           "migration",
			Language:           "java",
			Pattern:            `switch\s*\([^)]+\)\s*\{[^}]*case\s+`,
			Keywords:           []string{"switch", "case"},
			Severity:           "low",
			SuggestionTemplate: "Consider using switch expressions with -> syntax",
		},
		{
			ID:                 "java_pattern_matching",
			Name:               "Pattern Matching Available",
			Description:        "instanceof can use pattern matching (Java 16+)",
			Category:           "migration",
			Language:           "java",
			Pattern:            `instanceof\s+(\w+)\s*\)|if\s*\([^)]*instanceof`,
			Keywords:           []string{"instanceof"},
			Severity:           "low",
			SuggestionTemplate: "Use pattern matching: if (obj instanceof Type var)",
		},
		{
			ID:                 "java_records",
			Name:               "Record Class Available",
			Description:        "Data class can be converted to record (Java 14+)",
			Category:           "migration",
			Language:           "java",
			Pattern:            `class\s+\w+\s*\{[^}]*private\s+final\s+\w+`,
			Keywords:           []string{"class", "private", "final", "getter"},
			Severity:           "low",
			SuggestionTemplate: "Consider converting to record class",
		},
		{
			ID:                 "java_sealed_class",
			Name:               "Sealed Class Available",
			Description:        "Class hierarchy can use sealed classes (Java 17)",
			Category:           "migration",
			Language:           "java",
			Pattern:            `abstract\s+class|interface\s+\w+.*extends`,
			Keywords:           []string{"abstract", "class", "interface", "extends"},
			Severity:           "low",
			SuggestionTemplate: "Consider using sealed classes for restricted inheritance",
		},
		{
			ID:                 "java_var_keyword",
			Name:               "Local Variable Type Inference",
			Description:        "Can use 'var' for local variables (Java 10+)",
			Category:           "migration",
			Language:           "java",
			Pattern:            `(List|Map|Set|String|Integer)<[^>]+>\s+\w+\s*=\s*new`,
			Keywords:           []string{"List", "Map", "new"},
			Severity:           "low",
			SuggestionTemplate: "Consider using 'var' for local variable declaration",
		},
	}
	
	db.patterns["java"] = append(db.patterns["java"], migrationPatterns...)
}

// addCodeQualityPatterns adds general code quality patterns
func (db *PatternDatabase) addCodeQualityPatterns() {
	qualityPatterns := []*ErrorPattern{
		{
			ID:                 "long_method",
			Name:               "Long Method",
			Description:        "Method is too long",
			Category:           "quality",
			Language:           "java",
			Pattern:            ``, // Will use line count
			Keywords:           []string{},
			Severity:           "medium",
			SuggestionTemplate: "Consider breaking down method '%s' (>50 lines)",
		},
		{
			ID:                 "deep_nesting",
			Name:               "Deep Nesting",
			Description:        "Code has deep nesting levels",
			Category:           "quality",
			Language:           "java",
			Pattern:            `(\{[^{}]*){5,}`, // 5+ levels of nesting
			Keywords:           []string{},
			Severity:           "medium",
			SuggestionTemplate: "Reduce nesting depth using early returns or extraction",
		},
		{
			ID:                 "empty_catch",
			Name:               "Empty Catch Block",
			Description:        "Catch block is empty",
			Category:           "quality",
			Language:           "java",
			Pattern:            `catch\s*\([^)]+\)\s*\{\s*\}`,
			Keywords:           []string{"catch", "empty"},
			Severity:           "high",
			SuggestionTemplate: "Handle or log the exception in catch block",
		},
		{
			ID:                 "todo_comment",
			Name:               "TODO Comment",
			Description:        "TODO comment found",
			Category:           "quality",
			Language:           "java",
			Pattern:            `//\s*(TODO|FIXME|HACK|XXX)`,
			Keywords:           []string{"TODO", "FIXME", "HACK"},
			Severity:           "low",
			SuggestionTemplate: "Address TODO: %s",
		},
		{
			ID:                 "magic_number",
			Name:               "Magic Number",
			Description:        "Hard-coded numeric literal",
			Category:           "quality",
			Language:           "java",
			Pattern:            `[^0-9\.]([2-9]\d{2,}|[1-9]\d{3,})[^0-9]`, // Numbers > 100
			Keywords:           []string{},
			Severity:           "low",
			SuggestionTemplate: "Extract magic number to named constant",
		},
		{
			ID:                 "unused_import",
			Name:               "Unused Import",
			Description:        "Import statement is not used",
			Category:           "quality",
			Language:           "java",
			Pattern:            `import\s+[^;]+;.*The import .+ is never used`,
			Keywords:           []string{"import", "unused", "never"},
			Severity:           "low",
			SuggestionTemplate: "Remove unused import '%s'",
		},
	}
	
	db.patterns["java"] = append(db.patterns["java"], qualityPatterns...)
}

// GetPatterns returns patterns for a specific language
func (db *PatternDatabase) GetPatterns(language string) []*ErrorPattern {
	return db.patterns[strings.ToLower(language)]
}

// GetAllPatterns returns all patterns in the database
func (db *PatternDatabase) GetAllPatterns() map[string][]*ErrorPattern {
	return db.patterns
}

// AddPattern adds a custom pattern to the database
func (db *PatternDatabase) AddPattern(pattern *ErrorPattern) {
	if pattern.Language == "" {
		pattern.Language = "generic"
	}
	db.patterns[pattern.Language] = append(db.patterns[pattern.Language], pattern)
}

// RemovePattern removes a pattern by ID
func (db *PatternDatabase) RemovePattern(language, patternID string) bool {
	patterns := db.patterns[language]
	for i, p := range patterns {
		if p.ID == patternID {
			db.patterns[language] = append(patterns[:i], patterns[i+1:]...)
			return true
		}
	}
	return false
}

// PatternMatcher handles pattern matching operations
type PatternMatcher struct {
	database *PatternDatabase
	cache    map[string]*regexp.Regexp
}

// NewPatternMatcher creates a new pattern matcher
func NewPatternMatcher(database *PatternDatabase) *PatternMatcher {
	if database == nil {
		database = NewPatternDatabase()
	}
	return &PatternMatcher{
		database: database,
		cache:    make(map[string]*regexp.Regexp),
	}
}

// MatchPatterns matches patterns against code and error context
func (m *PatternMatcher) MatchPatterns(code, errorContext, language string) []PatternMatch {
	var matches []PatternMatch
	patterns := m.database.GetPatterns(language)
	
	// Combine code and error context for matching
	fullContext := code + "\n" + errorContext
	lines := strings.Split(code, "\n")
	
	for _, pattern := range patterns {
		if pattern.Pattern == "" {
			// Special handling for patterns without regex
			if pattern.ID == "long_method" {
				matches = append(matches, m.checkLongMethods(lines, pattern)...)
			}
			continue
		}
		
		regex := m.getCompiledRegex(pattern.Pattern)
		if regex == nil {
			continue
		}
		
		// Check for matches
		if matchIndices := regex.FindAllStringSubmatchIndex(fullContext, -1); len(matchIndices) > 0 {
			for _, matchIndex := range matchIndices {
				patternMatch := m.createPatternMatch(pattern, fullContext, matchIndex, lines)
				if patternMatch != nil {
					matches = append(matches, *patternMatch)
				}
			}
		}
	}
	
	return matches
}

// getCompiledRegex returns a compiled regex, using cache
func (m *PatternMatcher) getCompiledRegex(pattern string) *regexp.Regexp {
	if regex, exists := m.cache[pattern]; exists {
		return regex
	}
	
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	
	m.cache[pattern] = regex
	return regex
}

// createPatternMatch creates a PatternMatch from regex match
func (m *PatternMatcher) createPatternMatch(pattern *ErrorPattern, text string, match []int, lines []string) *PatternMatch {
	if len(match) < 2 {
		return nil
	}
	
	start := match[0]
	end := match[1]
	
	// Find line and column
	line, column := m.findLineAndColumn(text, start)
	
	// Extract context
	contextStart := start - 50
	if contextStart < 0 {
		contextStart = 0
	}
	contextEnd := end + 50
	if contextEnd > len(text) {
		contextEnd = len(text)
	}
	context := text[contextStart:contextEnd]
	
	// Calculate confidence based on keyword matches
	confidence := m.calculateConfidence(text[start:end], pattern)
	
	// Format suggestion
	suggestion := m.formatSuggestion(pattern.SuggestionTemplate, text[start:end], line)
	
	return &PatternMatch{
		PatternID:   pattern.ID,
		Name:        pattern.Name,
		Description: pattern.Description,
		Category:    pattern.Category,
		Severity:    pattern.Severity,
		Confidence:  confidence,
		Location: Location{
			Line:   line,
			Column: column,
		},
		Suggestion: suggestion,
		Context:    context,
	}
}

// findLineAndColumn finds the line and column number for a position
func (m *PatternMatcher) findLineAndColumn(text string, position int) (line, column int) {
	line = 1
	column = 1
	
	for i := 0; i < position && i < len(text); i++ {
		if text[i] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	
	return line, column
}

// calculateConfidence calculates confidence score for a match
func (m *PatternMatcher) calculateConfidence(matchedText string, pattern *ErrorPattern) float64 {
	confidence := 0.5 // Base confidence
	
	// Check for keyword matches
	matchedLower := strings.ToLower(matchedText)
	keywordMatches := 0
	for _, keyword := range pattern.Keywords {
		if strings.Contains(matchedLower, strings.ToLower(keyword)) {
			keywordMatches++
		}
	}
	
	if len(pattern.Keywords) > 0 {
		confidence += 0.5 * float64(keywordMatches) / float64(len(pattern.Keywords))
	} else {
		confidence = 0.7 // Default confidence when no keywords
	}
	
	// Adjust based on pattern category
	switch pattern.Category {
	case "compilation":
		confidence += 0.1
	case "runtime":
		confidence += 0.15
	}
	
	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return confidence
}

// formatSuggestion formats the suggestion template
func (m *PatternMatcher) formatSuggestion(template, matchedText string, line int) string {
	suggestion := template
	
	// Simple replacements
	suggestion = strings.ReplaceAll(suggestion, "%d", fmt.Sprintf("%d", line))
	suggestion = strings.ReplaceAll(suggestion, "%s", matchedText)
	
	return suggestion
}

// checkLongMethods checks for long methods
func (m *PatternMatcher) checkLongMethods(lines []string, pattern *ErrorPattern) []PatternMatch {
	var matches []PatternMatch
	
	inMethod := false
	methodStart := 0
	methodName := ""
	braceCount := 0
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Simple method detection (can be improved)
		if !inMethod && (strings.Contains(line, "public ") || strings.Contains(line, "private ") || 
			strings.Contains(line, "protected ")) && strings.Contains(line, "(") {
			
			// Extract method name
			if idx := strings.Index(line, "("); idx > 0 {
				parts := strings.Fields(line[:idx])
				if len(parts) > 0 {
					methodName = parts[len(parts)-1]
				}
			}
			
			if strings.Contains(line, "{") {
				inMethod = true
				methodStart = i + 1
				braceCount = 1
			}
		}
		
		if inMethod {
			braceCount += strings.Count(trimmed, "{")
			braceCount -= strings.Count(trimmed, "}")
			
			if braceCount == 0 {
				methodLength := i - methodStart + 1
				if methodLength > 50 {
					matches = append(matches, PatternMatch{
						PatternID:   pattern.ID,
						Name:        pattern.Name,
						Description: fmt.Sprintf("Method '%s' is %d lines long", methodName, methodLength),
						Category:    pattern.Category,
						Severity:    pattern.Severity,
						Confidence:  0.9,
						Location: Location{
							Line:    methodStart,
							EndLine: i + 1,
						},
						Suggestion: fmt.Sprintf(pattern.SuggestionTemplate, methodName),
					})
				}
				inMethod = false
			}
		}
	}
	
	return matches
}