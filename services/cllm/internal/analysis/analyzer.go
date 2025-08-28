package analysis

import (
	"regexp"
	"strings"
	"time"
)

// Analyzer performs code analysis
type Analyzer struct {
	patternMatcher *PatternMatcher
	contextBuilder *ContextBuilder
	validator      *Validator
}

// NewAnalyzer creates a new code analyzer
func NewAnalyzer() *Analyzer {
	patternDatabase := NewPatternDatabase()
	return &Analyzer{
		patternMatcher: NewPatternMatcher(patternDatabase),
		contextBuilder: NewContextBuilder(),
		validator:      NewValidator(),
	}
}

// Analyze performs comprehensive code analysis
func (a *Analyzer) Analyze(request AnalysisRequest) (*AnalysisResult, error) {
	startTime := time.Now()
	
	// Initialize result
	result := &AnalysisResult{
		Issues:   []Issue{},
		Patterns: []PatternMatch{},
	}
	
	// Extract code structure
	if request.Options.IncludePatterns || request.Options.IncludeMetrics {
		structure := a.extractStructure(request.Code, request.Language)
		result.Structure = structure
	}
	
	// Perform pattern matching
	if request.Options.IncludePatterns {
		patterns := a.patternMatcher.MatchPatterns(
			request.Code,
			request.ErrorContext,
			request.Language,
		)
		result.Patterns = patterns
		
		// Convert high-severity patterns to issues
		for _, pattern := range patterns {
			if pattern.Severity == "critical" || pattern.Severity == "high" {
				result.Issues = append(result.Issues, Issue{
					Type:     "pattern_match",
					Message:  pattern.Description,
					Severity: pattern.Severity,
					Location: pattern.Location,
					Rule:     pattern.PatternID,
				})
			}
		}
	}
	
	// Calculate metrics
	if request.Options.IncludeMetrics {
		metrics := a.calculateMetrics(request.Code, result.Structure)
		result.Metrics = metrics
	}
	
	// Build LLM context
	contextConfig := ContextConfig{
		MaxTokens:      4000,
		IncludeImports: true,
		IncludeComments: false,
		FocusOnErrors:  request.ErrorContext != "",
		ContextRadius:  5,
	}
	
	context := a.contextBuilder.BuildContext(
		request.Code,
		request.ErrorContext,
		result.Structure,
		result.Patterns,
		contextConfig,
	)
	result.Context = context
	
	// Validate code
	validation := a.validator.ValidateCode(request.Code, request.Language)
	for _, err := range validation.Errors {
		result.Issues = append(result.Issues, Issue{
			Type:     "validation_error",
			Message:  err.Message,
			Severity: "high",
			Location: err.Location,
		})
	}
	
	result.ProcessingTime = time.Since(startTime)
	return result, nil
}

// extractStructure extracts the structure of the code
func (a *Analyzer) extractStructure(code, language string) *CodeStructure {
	switch strings.ToLower(language) {
	case "java":
		return a.extractJavaStructure(code)
	case "python":
		return a.extractPythonStructure(code)
	case "javascript", "typescript":
		return a.extractJavaScriptStructure(code)
	case "go":
		return a.extractGoStructure(code)
	default:
		return a.extractGenericStructure(code)
	}
}

// extractJavaStructure extracts structure from Java code
func (a *Analyzer) extractJavaStructure(code string) *CodeStructure {
	structure := &CodeStructure{
		Imports: []Import{},
		Classes: []ClassInfo{},
		Methods: []MethodInfo{},
		Fields:  []FieldInfo{},
	}
	
	lines := strings.Split(code, "\n")
	
	// Extract package
	packageRegex := regexp.MustCompile(`^\s*package\s+([\w.]+)\s*;`)
	for i, line := range lines {
		if match := packageRegex.FindStringSubmatch(line); len(match) > 1 {
			structure.Package = match[1]
			break
		}
		// Stop looking after first non-comment, non-empty line that's not a package
		if i > 10 && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "//") {
			break
		}
	}
	
	// Extract imports
	importRegex := regexp.MustCompile(`^\s*import\s+(static\s+)?([\w.]+(?:\.\*)?)\s*;`)
	for i, line := range lines {
		if match := importRegex.FindStringSubmatch(line); len(match) > 2 {
			imp := Import{
				Path:     match[2],
				IsStatic: match[1] != "",
				Line:     i + 1,
			}
			structure.Imports = append(structure.Imports, imp)
		}
	}
	
	// Extract classes
	classRegex := regexp.MustCompile(`^\s*(public\s+|private\s+|protected\s+)?(abstract\s+|final\s+)?(class|interface|enum)\s+(\w+)(\s+extends\s+(\w+))?(\s+implements\s+([\w,\s]+))?`)
	
	for i, line := range lines {
		if match := classRegex.FindStringSubmatch(line); len(match) > 4 {
			class := ClassInfo{
				Name:      match[4],
				Type:      match[3],
				Modifiers: []string{},
				StartLine: i + 1,
			}
			
			// Add modifiers
			if match[1] != "" {
				class.Modifiers = append(class.Modifiers, strings.TrimSpace(match[1]))
			}
			if match[2] != "" {
				class.Modifiers = append(class.Modifiers, strings.TrimSpace(match[2]))
			}
			
			// Extends
			if len(match) > 6 && match[6] != "" {
				class.Extends = match[6]
			}
			
			// Implements
			if len(match) > 8 && match[8] != "" {
				implements := strings.Split(match[8], ",")
				for _, impl := range implements {
					class.Implements = append(class.Implements, strings.TrimSpace(impl))
				}
			}
			
			// Find end of class (simplified)
			class.EndLine = a.findBlockEnd(lines, i)
			
			// Extract methods and fields within class
			if class.EndLine > i && class.EndLine <= len(lines) {
				class.Methods, class.Fields = a.extractClassMembers(lines[i:class.EndLine], i+1)
			}
			
			structure.Classes = append(structure.Classes, class)
		}
	}
	
	return structure
}

// extractClassMembers extracts methods and fields from class body
func (a *Analyzer) extractClassMembers(lines []string, startLine int) ([]MethodInfo, []FieldInfo) {
	var methods []MethodInfo
	var fields []FieldInfo
	
	// Simplified method regex
	methodRegex := regexp.MustCompile(`^\s*(public\s+|private\s+|protected\s+)?(static\s+)?(final\s+)?(\w+(?:<[^>]+>)?)\s+(\w+)\s*\([^)]*\)`)
	
	// Simplified field regex
	// Field regex requires at least one modifier or explicit type declaration
	fieldRegex := regexp.MustCompile(`^\s*(public\s+|private\s+|protected\s+)(static\s+)?(final\s+)?(\w+(?:<[^>]+>)?)\s+(\w+)\s*(=\s*[^;]+)?\s*;`)
	
	for i, line := range lines {
		// Check for method
		if match := methodRegex.FindStringSubmatch(line); len(match) > 5 {
			method := MethodInfo{
				Name:       match[5],
				ReturnType: match[4],
				Modifiers:  []string{},
				StartLine:  startLine + i,
				Parameters: a.extractParameters(line),
			}
			
			// Add modifiers
			if match[1] != "" {
				method.Modifiers = append(method.Modifiers, strings.TrimSpace(match[1]))
			}
			if match[2] != "" {
				method.Modifiers = append(method.Modifiers, "static")
			}
			if match[3] != "" {
				method.Modifiers = append(method.Modifiers, "final")
			}
			
			// Find method end
			method.EndLine = startLine + a.findBlockEnd(lines, i)
			
			// Calculate basic complexity
			method.Complexity = a.calculateCyclomaticComplexity(lines[i:method.EndLine-startLine])
			
			methods = append(methods, method)
		} else if match := fieldRegex.FindStringSubmatch(line); len(match) > 5 {
			// Check for field
			field := FieldInfo{
				Name:      match[5],
				Type:      match[4],
				Modifiers: []string{},
				Line:      startLine + i,
			}
			
			// Add modifiers
			if match[1] != "" {
				field.Modifiers = append(field.Modifiers, strings.TrimSpace(match[1]))
			}
			if match[2] != "" {
				field.Modifiers = append(field.Modifiers, "static")
			}
			if match[3] != "" {
				field.Modifiers = append(field.Modifiers, "final")
			}
			
			// Initial value
			if len(match) > 6 && match[6] != "" {
				field.InitialValue = strings.TrimSpace(match[6][1:]) // Remove '='
			}
			
			fields = append(fields, field)
		}
	}
	
	return methods, fields
}

// extractParameters extracts method parameters from a method signature
func (a *Analyzer) extractParameters(methodSignature string) []ParameterInfo {
	var parameters []ParameterInfo
	
	// Find parameters between parentheses
	start := strings.Index(methodSignature, "(")
	end := strings.LastIndex(methodSignature, ")")
	
	if start < 0 || end < 0 || end <= start {
		return parameters
	}
	
	paramString := methodSignature[start+1 : end]
	if strings.TrimSpace(paramString) == "" {
		return parameters
	}
	
	// Split by comma (simplified - doesn't handle generics perfectly)
	params := strings.Split(paramString, ",")
	
	for _, param := range params {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}
		
		// Check for varargs
		isVarargs := strings.Contains(param, "...")
		param = strings.ReplaceAll(param, "...", "")
		
		// Split type and name
		parts := strings.Fields(param)
		if len(parts) >= 2 {
			paramInfo := ParameterInfo{
				Type:      strings.Join(parts[:len(parts)-1], " "),
				Name:      parts[len(parts)-1],
				IsVarargs: isVarargs,
			}
			parameters = append(parameters, paramInfo)
		}
	}
	
	return parameters
}

// findBlockEnd finds the end of a code block
func (a *Analyzer) findBlockEnd(lines []string, startIndex int) int {
	braceCount := 0
	started := false
	
	for i := startIndex; i < len(lines); i++ {
		line := lines[i]
		
		// Count braces
		for _, char := range line {
			if char == '{' {
				braceCount++
				started = true
			} else if char == '}' {
				braceCount--
				if started && braceCount == 0 {
					return i + 1
				}
			}
		}
	}
	
	return len(lines)
}

// calculateCyclomaticComplexity calculates the cyclomatic complexity of code
func (a *Analyzer) calculateCyclomaticComplexity(lines []string) int {
	complexity := 1 // Base complexity
	
	// Keywords that increase complexity
	complexityKeywords := []string{
		"if ", "else if", "case ", "for ", "while ", "catch ", 
		"&&", "||", "?", "switch ",
	}
	
	code := strings.Join(lines, "\n")
	
	for _, keyword := range complexityKeywords {
		complexity += strings.Count(code, keyword)
	}
	
	return complexity
}

// extractPythonStructure extracts structure from Python code
func (a *Analyzer) extractPythonStructure(code string) *CodeStructure {
	structure := &CodeStructure{
		Imports: []Import{},
		Classes: []ClassInfo{},
		Methods: []MethodInfo{},
		Fields:  []FieldInfo{},
	}
	
	lines := strings.Split(code, "\n")
	
	// Extract imports
	importRegex := regexp.MustCompile(`^\s*(from\s+([\w.]+)\s+)?import\s+([\w.,\s*]+)(\s+as\s+(\w+))?`)
	
	for i, line := range lines {
		if match := importRegex.FindStringSubmatch(line); len(match) > 3 {
			imp := Import{
				Path: match[3],
				Line: i + 1,
			}
			if match[2] != "" {
				imp.Path = match[2] + "." + match[3]
			}
			if len(match) > 5 && match[5] != "" {
				imp.Alias = match[5]
			}
			structure.Imports = append(structure.Imports, imp)
		}
	}
	
	// Extract classes and functions
	classRegex := regexp.MustCompile(`^class\s+(\w+)(\([^)]*\))?:`)
	funcRegex := regexp.MustCompile(`^def\s+(\w+)\s*\([^)]*\):`)
	
	for i, line := range lines {
		if match := classRegex.FindStringSubmatch(line); len(match) > 1 {
			class := ClassInfo{
				Name:      match[1],
				Type:      "class",
				StartLine: i + 1,
				EndLine:   a.findPythonBlockEnd(lines, i),
			}
			structure.Classes = append(structure.Classes, class)
		} else if match := funcRegex.FindStringSubmatch(line); len(match) > 1 {
			// Check if it's a top-level function (no indentation)
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				method := MethodInfo{
					Name:      match[1],
					StartLine: i + 1,
					EndLine:   a.findPythonBlockEnd(lines, i),
				}
				structure.Methods = append(structure.Methods, method)
			}
		}
	}
	
	return structure
}

// findPythonBlockEnd finds the end of a Python code block
func (a *Analyzer) findPythonBlockEnd(lines []string, startIndex int) int {
	if startIndex >= len(lines)-1 {
		return len(lines)
	}
	
	// Get indentation level of the block start
	startLine := lines[startIndex]
	baseIndent := len(startLine) - len(strings.TrimLeft(startLine, " \t"))
	
	// Find where indentation returns to same or less level
	for i := startIndex + 1; i < len(lines); i++ {
		line := lines[i]
		
		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}
		
		currentIndent := len(line) - len(strings.TrimLeft(line, " \t"))
		if currentIndent <= baseIndent {
			return i
		}
	}
	
	return len(lines)
}

// extractJavaScriptStructure extracts structure from JavaScript/TypeScript code
func (a *Analyzer) extractJavaScriptStructure(code string) *CodeStructure {
	structure := &CodeStructure{
		Imports: []Import{},
		Classes: []ClassInfo{},
		Methods: []MethodInfo{},
		Fields:  []FieldInfo{},
	}
	
	lines := strings.Split(code, "\n")
	
	// Extract imports
	importRegex := regexp.MustCompile(`^\s*import\s+(.+)\s+from\s+['"]([^'"]+)['"]`)
	
	for i, line := range lines {
		if match := importRegex.FindStringSubmatch(line); len(match) > 2 {
			imp := Import{
				Path: match[2],
				Line: i + 1,
			}
			structure.Imports = append(structure.Imports, imp)
		}
	}
	
	// Extract classes and functions
	classRegex := regexp.MustCompile(`^\s*(export\s+)?(default\s+)?class\s+(\w+)`)
	funcRegex := regexp.MustCompile(`^\s*(export\s+)?(async\s+)?function\s+(\w+)`)
	arrowFuncRegex := regexp.MustCompile(`^\s*(export\s+)?const\s+(\w+)\s*=\s*(async\s+)?\([^)]*\)\s*=>`)
	
	for i, line := range lines {
		if match := classRegex.FindStringSubmatch(line); len(match) > 3 {
			class := ClassInfo{
				Name:      match[3],
				Type:      "class",
				StartLine: i + 1,
				EndLine:   a.findBlockEnd(lines, i),
			}
			if match[1] != "" {
				class.Modifiers = append(class.Modifiers, "export")
			}
			structure.Classes = append(structure.Classes, class)
		} else if match := funcRegex.FindStringSubmatch(line); len(match) > 3 {
			method := MethodInfo{
				Name:      match[3],
				StartLine: i + 1,
				EndLine:   a.findBlockEnd(lines, i),
			}
			if match[2] != "" {
				method.Modifiers = append(method.Modifiers, "async")
			}
			structure.Methods = append(structure.Methods, method)
		} else if match := arrowFuncRegex.FindStringSubmatch(line); len(match) > 2 {
			method := MethodInfo{
				Name:      match[2],
				StartLine: i + 1,
				EndLine:   a.findBlockEnd(lines, i),
			}
			if match[3] != "" {
				method.Modifiers = append(method.Modifiers, "async")
			}
			structure.Methods = append(structure.Methods, method)
		}
	}
	
	return structure
}

// extractGoStructure extracts structure from Go code
func (a *Analyzer) extractGoStructure(code string) *CodeStructure {
	structure := &CodeStructure{
		Imports: []Import{},
		Classes: []ClassInfo{}, // Will use for structs
		Methods: []MethodInfo{},
		Fields:  []FieldInfo{},
	}
	
	lines := strings.Split(code, "\n")
	
	// Extract package
	packageRegex := regexp.MustCompile(`^\s*package\s+(\w+)`)
	for _, line := range lines {
		if match := packageRegex.FindStringSubmatch(line); len(match) > 1 {
			structure.Package = match[1]
			break
		}
	}
	
	// Extract imports
	importRegex := regexp.MustCompile(`^\s*import\s+"([^"]+)"`)
	importBlockRegex := regexp.MustCompile(`^\s*import\s+\(`)
	inImportBlock := false
	
	for i, line := range lines {
		if importBlockRegex.MatchString(line) {
			inImportBlock = true
			continue
		}
		
		if inImportBlock {
			if strings.Contains(line, ")") {
				inImportBlock = false
				continue
			}
			
			// Extract import from block
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && trimmed != "(" && trimmed != ")" {
				// Remove quotes
				path := strings.Trim(trimmed, `"`)
				imp := Import{
					Path: path,
					Line: i + 1,
				}
				structure.Imports = append(structure.Imports, imp)
			}
		} else if match := importRegex.FindStringSubmatch(line); len(match) > 1 {
			imp := Import{
				Path: match[1],
				Line: i + 1,
			}
			structure.Imports = append(structure.Imports, imp)
		}
	}
	
	// Extract structs (as classes)
	structRegex := regexp.MustCompile(`^\s*type\s+(\w+)\s+struct\s*{`)
	
	for i, line := range lines {
		if match := structRegex.FindStringSubmatch(line); len(match) > 1 {
			class := ClassInfo{
				Name:      match[1],
				Type:      "struct",
				StartLine: i + 1,
				EndLine:   a.findBlockEnd(lines, i),
			}
			structure.Classes = append(structure.Classes, class)
		}
	}
	
	// Extract functions
	funcRegex := regexp.MustCompile(`^\s*func\s+(\(\s*\w+\s+[^)]+\)\s+)?(\w+)\s*\([^)]*\)`)
	
	for i, line := range lines {
		if match := funcRegex.FindStringSubmatch(line); len(match) > 2 {
			method := MethodInfo{
				Name:      match[2],
				StartLine: i + 1,
				EndLine:   a.findBlockEnd(lines, i),
			}
			structure.Methods = append(structure.Methods, method)
		}
	}
	
	return structure
}

// extractGenericStructure extracts basic structure from any code
func (a *Analyzer) extractGenericStructure(code string) *CodeStructure {
	structure := &CodeStructure{
		Imports: []Import{},
		Classes: []ClassInfo{},
		Methods: []MethodInfo{},
		Fields:  []FieldInfo{},
	}
	
	// Basic analysis for unknown languages
	lines := strings.Split(code, "\n")
	
	// Look for import-like statements
	for i, line := range lines {
		if strings.Contains(line, "import") || strings.Contains(line, "require") ||
			strings.Contains(line, "include") || strings.Contains(line, "using") {
			structure.Imports = append(structure.Imports, Import{
				Path: strings.TrimSpace(line),
				Line: i + 1,
			})
		}
	}
	
	return structure
}

// calculateMetrics calculates code quality metrics
func (a *Analyzer) calculateMetrics(code string, structure *CodeStructure) *CodeMetrics {
	lines := strings.Split(code, "\n")
	
	metrics := &CodeMetrics{
		TotalLines:   len(lines),
		ClassCount:   len(structure.Classes),
		MethodCount:  len(structure.Methods),
		LinesOfCode:  0,
		CommentLines: 0,
	}
	
	// Calculate lines of code and comments
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		if trimmed == "" {
			continue
		}
		
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			metrics.CommentLines++
		} else {
			metrics.LinesOfCode++
		}
	}
	
	// Calculate average complexity
	totalComplexity := 0
	maxComplexity := 0
	methodCount := 0
	
	for _, class := range structure.Classes {
		for _, method := range class.Methods {
			if method.Complexity > 0 {
				totalComplexity += method.Complexity
				methodCount++
				if method.Complexity > maxComplexity {
					maxComplexity = method.Complexity
				}
			}
		}
	}
	
	for _, method := range structure.Methods {
		if method.Complexity > 0 {
			totalComplexity += method.Complexity
			methodCount++
			if method.Complexity > maxComplexity {
				maxComplexity = method.Complexity
			}
		}
	}
	
	if methodCount > 0 {
		metrics.AverageComplexity = float64(totalComplexity) / float64(methodCount)
	}
	metrics.MaxComplexity = maxComplexity
	
	// Simple duplication detection (consecutive similar lines)
	duplicateLines := 0
	for i := 1; i < len(lines); i++ {
		if lines[i] == lines[i-1] && strings.TrimSpace(lines[i]) != "" {
			duplicateLines++
		}
	}
	
	if metrics.LinesOfCode > 0 {
		metrics.DuplicationRatio = float64(duplicateLines) / float64(metrics.LinesOfCode)
	}
	
	return metrics
}