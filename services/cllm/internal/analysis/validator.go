package analysis

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Validator performs code validation
type Validator struct {
	syntaxRules map[string][]SyntaxRule
}

// SyntaxRule represents a syntax validation rule
type SyntaxRule struct {
	ID          string
	Name        string
	Description string
	Check       func(string) []ValidationError
}

// NewValidator creates a new code validator
func NewValidator() *Validator {
	v := &Validator{
		syntaxRules: make(map[string][]SyntaxRule),
	}
	v.loadDefaultRules()
	return v
}

// loadDefaultRules loads default validation rules
func (v *Validator) loadDefaultRules() {
	// Java validation rules
	v.syntaxRules["java"] = []SyntaxRule{
		{
			ID:          "balanced_braces",
			Name:        "Balanced Braces",
			Description: "Check for balanced braces",
			Check:       v.checkBalancedBraces,
		},
		{
			ID:          "balanced_parentheses",
			Name:        "Balanced Parentheses",
			Description: "Check for balanced parentheses",
			Check:       v.checkBalancedParentheses,
		},
		{
			ID:          "missing_semicolon",
			Name:        "Missing Semicolons",
			Description: "Check for potentially missing semicolons",
			Check:       v.checkMissingSemicolons,
		},
		{
			ID:          "unclosed_string",
			Name:        "Unclosed String Literals",
			Description: "Check for unclosed string literals",
			Check:       v.checkUnclosedStrings,
		},
		{
			ID:          "invalid_identifiers",
			Name:        "Invalid Identifiers",
			Description: "Check for invalid variable/method names",
			Check:       v.checkInvalidIdentifiers,
		},
	}
	
	// Python validation rules
	v.syntaxRules["python"] = []SyntaxRule{
		{
			ID:          "indentation",
			Name:        "Indentation Errors",
			Description: "Check for indentation issues",
			Check:       v.checkPythonIndentation,
		},
		{
			ID:          "unclosed_string",
			Name:        "Unclosed String Literals",
			Description: "Check for unclosed string literals",
			Check:       v.checkUnclosedStrings,
		},
		{
			ID:          "colon_missing",
			Name:        "Missing Colons",
			Description: "Check for missing colons after control statements",
			Check:       v.checkPythonColons,
		},
	}
	
	// JavaScript/TypeScript validation rules
	v.syntaxRules["javascript"] = []SyntaxRule{
		{
			ID:          "balanced_braces",
			Name:        "Balanced Braces",
			Description: "Check for balanced braces",
			Check:       v.checkBalancedBraces,
		},
		{
			ID:          "balanced_parentheses",
			Name:        "Balanced Parentheses",
			Description: "Check for balanced parentheses",
			Check:       v.checkBalancedParentheses,
		},
		{
			ID:          "unclosed_string",
			Name:        "Unclosed String Literals",
			Description: "Check for unclosed string literals",
			Check:       v.checkUnclosedStrings,
		},
	}
	v.syntaxRules["typescript"] = v.syntaxRules["javascript"]
	
	// Go validation rules
	v.syntaxRules["go"] = []SyntaxRule{
		{
			ID:          "balanced_braces",
			Name:        "Balanced Braces",
			Description: "Check for balanced braces",
			Check:       v.checkBalancedBraces,
		},
		{
			ID:          "unused_imports",
			Name:        "Unused Imports",
			Description: "Check for potentially unused imports",
			Check:       v.checkGoUnusedImports,
		},
	}
}

// ValidateCode validates code for syntax and other issues
func (v *Validator) ValidateCode(code, language string) *ValidationResult {
	result := &ValidationResult{
		Valid:       true,
		Errors:      []ValidationError{},
		Warnings:    []ValidationWarning{},
		Suggestions: []string{},
	}
	
	// Get rules for the language
	rules, exists := v.syntaxRules[strings.ToLower(language)]
	if !exists {
		// Use generic validation for unknown languages
		rules = v.getGenericRules()
	}
	
	// Run each validation rule
	for _, rule := range rules {
		errors := rule.Check(code)
		if len(errors) > 0 {
			result.Valid = false
			result.Errors = append(result.Errors, errors...)
		}
	}
	
	// Add warnings
	warnings := v.checkForWarnings(code, language)
	result.Warnings = append(result.Warnings, warnings...)
	
	// Add suggestions
	suggestions := v.generateSuggestions(code, language)
	result.Suggestions = append(result.Suggestions, suggestions...)
	
	return result
}

// checkBalancedBraces checks for balanced braces
func (v *Validator) checkBalancedBraces(code string) []ValidationError {
	var errors []ValidationError
	stack := []rune{}
	lineNum := 1
	colNum := 1
	
	for _, char := range code {
		if char == '\n' {
			lineNum++
			colNum = 1
		} else {
			colNum++
		}
		
		switch char {
		case '{':
			stack = append(stack, char)
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				errors = append(errors, ValidationError{
					Type:    "unmatched_brace",
					Message: "Unmatched closing brace",
					Location: Location{
						Line:   lineNum,
						Column: colNum,
					},
				})
			} else {
				stack = stack[:len(stack)-1]
			}
		}
	}
	
	if len(stack) > 0 {
		errors = append(errors, ValidationError{
			Type:    "unclosed_brace",
			Message: fmt.Sprintf("%d unclosed brace(s)", len(stack)),
			Location: Location{
				Line: lineNum,
			},
		})
	}
	
	return errors
}

// checkBalancedParentheses checks for balanced parentheses
func (v *Validator) checkBalancedParentheses(code string) []ValidationError {
	var errors []ValidationError
	stack := []rune{}
	lineNum := 1
	colNum := 1
	inString := false
	escapeNext := false
	
	for _, char := range code {
		if char == '\n' {
			lineNum++
			colNum = 1
		} else {
			colNum++
		}
		
		// Handle string literals
		if !escapeNext {
			if char == '"' || char == '\'' {
				inString = !inString
				continue
			}
			if char == '\\' {
				escapeNext = true
				continue
			}
		} else {
			escapeNext = false
			continue
		}
		
		if inString {
			continue
		}
		
		switch char {
		case '(':
			stack = append(stack, char)
		case ')':
			if len(stack) == 0 || stack[len(stack)-1] != '(' {
				errors = append(errors, ValidationError{
					Type:    "unmatched_parenthesis",
					Message: "Unmatched closing parenthesis",
					Location: Location{
						Line:   lineNum,
						Column: colNum,
					},
				})
			} else {
				stack = stack[:len(stack)-1]
			}
		}
	}
	
	if len(stack) > 0 {
		errors = append(errors, ValidationError{
			Type:    "unclosed_parenthesis",
			Message: fmt.Sprintf("%d unclosed parenthesis/parentheses", len(stack)),
			Location: Location{
				Line: lineNum,
			},
		})
	}
	
	return errors
}

// checkMissingSemicolons checks for potentially missing semicolons in Java
func (v *Validator) checkMissingSemicolons(code string) []ValidationError {
	var errors []ValidationError
	lines := strings.Split(code, "\n")
	
	// Patterns that typically need semicolons
	needsSemicolon := regexp.MustCompile(`^\s*(return|break|continue|throw|import|package)\s+`)
	declarationPattern := regexp.MustCompile(`^\s*(public|private|protected|static|final)?\s*(int|long|double|float|boolean|char|String|void|\w+)\s+\w+\s*=`)
	methodCallPattern := regexp.MustCompile(`^\s*\w+\.\w+\([^)]*\)\s*$`)
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Skip empty lines, comments, and lines that already end with semicolon
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || 
			strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") ||
			strings.HasSuffix(trimmed, ";") || strings.HasSuffix(trimmed, "{") ||
			strings.HasSuffix(trimmed, "}") {
			continue
		}
		
		// Check if line likely needs a semicolon
		if needsSemicolon.MatchString(trimmed) ||
			declarationPattern.MatchString(trimmed) ||
			methodCallPattern.MatchString(trimmed) {
			
			errors = append(errors, ValidationError{
				Type:       "missing_semicolon",
				Message:    "Statement may be missing a semicolon",
				Location:   Location{Line: i + 1},
				CanAutoFix: true,
			})
		}
	}
	
	return errors
}

// checkUnclosedStrings checks for unclosed string literals
func (v *Validator) checkUnclosedStrings(code string) []ValidationError {
	var errors []ValidationError
	lines := strings.Split(code, "\n")
	
	for i, line := range lines {
		// Count quotes, accounting for escaped quotes
		doubleQuotes := 0
		singleQuotes := 0
		escaped := false
		
		for _, char := range line {
			if escaped {
				escaped = false
				continue
			}
			
			if char == '\\' {
				escaped = true
				continue
			}
			
			if char == '"' {
				doubleQuotes++
			} else if char == '\'' {
				singleQuotes++
			}
		}
		
		// Check for odd number of quotes (unclosed)
		if doubleQuotes%2 != 0 {
			errors = append(errors, ValidationError{
				Type:    "unclosed_string",
				Message: "Unclosed double-quoted string",
				Location: Location{Line: i + 1},
			})
		}
		
		if singleQuotes%2 != 0 {
			errors = append(errors, ValidationError{
				Type:    "unclosed_string",
				Message: "Unclosed single-quoted string",
				Location: Location{Line: i + 1},
			})
		}
	}
	
	return errors
}

// checkInvalidIdentifiers checks for invalid variable/method names
func (v *Validator) checkInvalidIdentifiers(code string) []ValidationError {
	var errors []ValidationError
	lines := strings.Split(code, "\n")
	
	// Java keywords that cannot be used as identifiers
	javaKeywords := map[string]bool{
		"abstract": true, "assert": true, "boolean": true, "break": true,
		"byte": true, "case": true, "catch": true, "char": true,
		"class": true, "const": true, "continue": true, "default": true,
		"do": true, "double": true, "else": true, "enum": true,
		"extends": true, "final": true, "finally": true, "float": true,
		"for": true, "goto": true, "if": true, "implements": true,
		"import": true, "instanceof": true, "int": true, "interface": true,
		"long": true, "native": true, "new": true, "package": true,
		"private": true, "protected": true, "public": true, "return": true,
		"short": true, "static": true, "strictfp": true, "super": true,
		"switch": true, "synchronized": true, "this": true, "throw": true,
		"throws": true, "transient": true, "try": true, "void": true,
		"volatile": true, "while": true,
	}
	
	// Pattern to match variable/method declarations
	identifierPattern := regexp.MustCompile(`\b(\w+)\s+(\w+)\s*[=;(]`)
	
	for i, line := range lines {
		matches := identifierPattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 2 {
				identifier := match[2]
				
				// Check if identifier is a keyword
				if javaKeywords[identifier] {
					errors = append(errors, ValidationError{
						Type:    "invalid_identifier",
						Message: fmt.Sprintf("'%s' is a reserved keyword and cannot be used as an identifier", identifier),
						Location: Location{Line: i + 1},
					})
				}
				
				// Check if identifier starts with a digit
				if len(identifier) > 0 && unicode.IsDigit(rune(identifier[0])) {
					errors = append(errors, ValidationError{
						Type:    "invalid_identifier",
						Message: fmt.Sprintf("Identifier '%s' cannot start with a digit", identifier),
						Location: Location{Line: i + 1},
					})
				}
			}
		}
	}
	
	return errors
}

// checkPythonIndentation checks for Python indentation issues
func (v *Validator) checkPythonIndentation(code string) []ValidationError {
	var errors []ValidationError
	lines := strings.Split(code, "\n")
	
	indentStack := []int{0}
	
	for i, line := range lines {
		// Skip empty lines and comments
		if strings.TrimSpace(line) == "" || strings.TrimSpace(line)[0] == '#' {
			continue
		}
		
		// Calculate indentation
		indent := 0
		for _, char := range line {
			if char == ' ' {
				indent++
			} else if char == '\t' {
				indent += 4 // Treat tab as 4 spaces
			} else {
				break
			}
		}
		
		// Check if line should increase indentation
		trimmed := strings.TrimSpace(line)
		shouldIndent := strings.HasSuffix(trimmed, ":") &&
			(strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "elif ") ||
				strings.HasPrefix(trimmed, "else") || strings.HasPrefix(trimmed, "for ") ||
				strings.HasPrefix(trimmed, "while ") || strings.HasPrefix(trimmed, "def ") ||
				strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "try") ||
				strings.HasPrefix(trimmed, "except") || strings.HasPrefix(trimmed, "finally") ||
				strings.HasPrefix(trimmed, "with "))
		
		// Check indentation level
		if indent < indentStack[len(indentStack)-1] {
			// Dedent - should match a previous level
			found := false
			for _, level := range indentStack {
				if indent == level {
					found = true
					// Pop stack until we reach this level
					for indentStack[len(indentStack)-1] != indent {
						indentStack = indentStack[:len(indentStack)-1]
					}
					break
				}
			}
			
			if !found {
				errors = append(errors, ValidationError{
					Type:    "indentation_error",
					Message: "Inconsistent indentation",
					Location: Location{Line: i + 1},
				})
			}
		} else if indent > indentStack[len(indentStack)-1] {
			// Indent - should be after a colon
			if i > 0 {
				prevLine := strings.TrimSpace(lines[i-1])
				if !strings.HasSuffix(prevLine, ":") {
					errors = append(errors, ValidationError{
						Type:    "indentation_error",
						Message: "Unexpected indentation",
						Location: Location{Line: i + 1},
					})
				}
			}
			indentStack = append(indentStack, indent)
		}
		
		// Note: shouldIndent check left for future enhancement
		_ = shouldIndent
	}
	
	return errors
}

// checkPythonColons checks for missing colons in Python
func (v *Validator) checkPythonColons(code string) []ValidationError {
	var errors []ValidationError
	lines := strings.Split(code, "\n")
	
	// Patterns that need colons
	needsColon := []string{
		`^\s*if\s+.+[^:]$`,
		`^\s*elif\s+.+[^:]$`,
		`^\s*else[^:]$`,
		`^\s*for\s+.+\s+in\s+.+[^:]$`,
		`^\s*while\s+.+[^:]$`,
		`^\s*def\s+\w+\([^)]*\)[^:]$`,
		`^\s*class\s+\w+.*[^:]$`,
		`^\s*try[^:]$`,
		`^\s*except.*[^:]$`,
		`^\s*finally[^:]$`,
		`^\s*with\s+.+[^:]$`,
	}
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		
		// Check each pattern
		for _, pattern := range needsColon {
			if matched, _ := regexp.MatchString(pattern, line); matched {
				errors = append(errors, ValidationError{
					Type:       "missing_colon",
					Message:    "Statement may be missing a colon",
					Location:   Location{Line: i + 1},
					CanAutoFix: true,
				})
				break
			}
		}
	}
	
	return errors
}

// checkGoUnusedImports checks for potentially unused imports in Go
func (v *Validator) checkGoUnusedImports(code string) []ValidationError {
	var errors []ValidationError
	lines := strings.Split(code, "\n")
	
	// Extract imports
	imports := []string{}
	importAliases := make(map[string]string)
	inImportBlock := false
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		if strings.HasPrefix(trimmed, "import (") {
			inImportBlock = true
			continue
		}
		
		if inImportBlock {
			if trimmed == ")" {
				inImportBlock = false
				continue
			}
			
			// Extract import
			importLine := strings.Trim(trimmed, `"`)
			if importLine != "" {
				parts := strings.Fields(importLine)
				if len(parts) == 2 {
					// Aliased import
					importAliases[parts[0]] = parts[1]
					imports = append(imports, parts[0])
				} else if len(parts) == 1 {
					// Regular import
					lastPart := parts[0]
					if idx := strings.LastIndex(lastPart, "/"); idx >= 0 {
						lastPart = lastPart[idx+1:]
					}
					imports = append(imports, lastPart)
				}
			}
		} else if strings.HasPrefix(trimmed, "import ") {
			// Single import
			importMatch := regexp.MustCompile(`import\s+"([^"]+)"`).FindStringSubmatch(trimmed)
			if len(importMatch) > 1 {
				importPath := importMatch[1]
				lastPart := importPath
				if idx := strings.LastIndex(lastPart, "/"); idx >= 0 {
					lastPart = lastPart[idx+1:]
				}
				imports = append(imports, lastPart)
			}
		}
	}
	
	// Check if imports are used
	codeWithoutImports := strings.Join(lines, "\n")
	for _, imp := range imports {
		// Simple check: see if the import name appears in the code
		// This is a simplified check and may have false positives
		if !strings.Contains(codeWithoutImports, imp+".") && 
			!strings.Contains(codeWithoutImports, imp+"(") {
			errors = append(errors, ValidationError{
				Type:    "unused_import",
				Message: fmt.Sprintf("Import '%s' may be unused", imp),
				Location: Location{Line: 1}, // Approximate line (imports are at the top)
			})
		}
	}
	
	return errors
}

// checkForWarnings checks for code that might cause warnings
func (v *Validator) checkForWarnings(code, language string) []ValidationWarning {
	var warnings []ValidationWarning
	lines := strings.Split(code, "\n")
	
	// Check for TODO/FIXME comments
	for i, line := range lines {
		if strings.Contains(line, "TODO") || strings.Contains(line, "FIXME") ||
			strings.Contains(line, "HACK") || strings.Contains(line, "XXX") {
			warnings = append(warnings, ValidationWarning{
				Type:    "todo_comment",
				Message: "TODO/FIXME comment found",
				Location: Location{Line: i + 1},
			})
		}
	}
	
	// Check for console.log in production code (JavaScript)
	if language == "javascript" || language == "typescript" {
		for i, line := range lines {
			if strings.Contains(line, "console.log") {
				warnings = append(warnings, ValidationWarning{
					Type:    "console_log",
					Message: "console.log found - consider using proper logging",
					Location: Location{Line: i + 1},
				})
			}
		}
	}
	
	// Check for print statements in production code (Python)
	if language == "python" {
		for i, line := range lines {
			if strings.Contains(line, "print(") {
				warnings = append(warnings, ValidationWarning{
					Type:    "print_statement",
					Message: "print() found - consider using proper logging",
					Location: Location{Line: i + 1},
				})
			}
		}
	}
	
	// Check for fmt.Println in production code (Go)
	if language == "go" {
		for i, line := range lines {
			if strings.Contains(line, "fmt.Println") || strings.Contains(line, "fmt.Printf") {
				warnings = append(warnings, ValidationWarning{
					Type:    "fmt_print",
					Message: "fmt.Print* found - consider using proper logging",
					Location: Location{Line: i + 1},
				})
			}
		}
	}
	
	return warnings
}

// generateSuggestions generates improvement suggestions
func (v *Validator) generateSuggestions(code, language string) []string {
	var suggestions []string
	lines := strings.Split(code, "\n")
	
	// Check for very long lines
	for _, line := range lines {
		if len(line) > 120 {
			suggestions = append(suggestions, "Consider breaking long lines (>120 characters) for better readability")
			break
		}
	}
	
	// Check for deep nesting
	maxNesting := 0
	currentNesting := 0
	for _, line := range lines {
		currentNesting += strings.Count(line, "{")
		currentNesting -= strings.Count(line, "}")
		if currentNesting > maxNesting {
			maxNesting = currentNesting
		}
	}
	
	if maxNesting > 4 {
		suggestions = append(suggestions, "Consider refactoring deeply nested code (>4 levels) for better maintainability")
	}
	
	// Language-specific suggestions
	switch language {
	case "java":
		if !strings.Contains(code, "@Override") && strings.Contains(code, "extends") {
			suggestions = append(suggestions, "Consider using @Override annotation for overridden methods")
		}
		if strings.Contains(code, "System.out.println") {
			suggestions = append(suggestions, "Consider using a proper logging framework instead of System.out.println")
		}
		
	case "python":
		if !strings.Contains(code, "if __name__") && strings.Contains(code, "def main") {
			suggestions = append(suggestions, "Consider adding 'if __name__ == \"__main__\":' guard for script execution")
		}
		
	case "go":
		if !strings.Contains(code, "defer") && (strings.Contains(code, "Close()") || strings.Contains(code, "Unlock()")) {
			suggestions = append(suggestions, "Consider using 'defer' for cleanup operations")
		}
	}
	
	return suggestions
}

// getGenericRules returns generic validation rules for unknown languages
func (v *Validator) getGenericRules() []SyntaxRule {
	return []SyntaxRule{
		{
			ID:          "balanced_braces",
			Name:        "Balanced Braces",
			Description: "Check for balanced braces",
			Check:       v.checkBalancedBraces,
		},
		{
			ID:          "balanced_parentheses",
			Name:        "Balanced Parentheses",
			Description: "Check for balanced parentheses",
			Check:       v.checkBalancedParentheses,
		},
		{
			ID:          "unclosed_string",
			Name:        "Unclosed String Literals",
			Description: "Check for unclosed string literals",
			Check:       v.checkUnclosedStrings,
		},
	}
}