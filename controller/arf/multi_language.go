package arf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// MultiLanguageEngine defines the interface for universal AST parsing and transformation
type MultiLanguageEngine interface {
	ParseAST(ctx context.Context, code string, language string) (*UniversalAST, error)
	GenerateTransformation(ctx context.Context, ast *UniversalAST, recipe Recipe) (*Transformation, error)
	ValidateLanguageSupport(language string) (bool, error)
	GetLanguageCapabilities(language string) (*LanguageCapabilities, error)
}

// UniversalAST represents a parsed Abstract Syntax Tree for any supported language
type UniversalAST struct {
	Language    string                 `json:"language"`
	Parser      string                 `json:"parser"`
	RootNode    *ASTNode               `json:"root_node"`
	Symbols     []Symbol               `json:"symbols"`
	Imports     []Import               `json:"imports"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// ASTNode represents a node in the Abstract Syntax Tree
type ASTNode struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	StartByte    int                    `json:"start_byte"`
	EndByte      int                    `json:"end_byte"`
	StartPoint   Point                  `json:"start_point"`
	EndPoint     Point                  `json:"end_point"`
	Children     []*ASTNode             `json:"children"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// Point represents a position in source code
type Point struct {
	Row    int `json:"row"`
	Column int `json:"column"`
}

// Symbol represents a symbol (function, class, variable) in the AST
type Symbol struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Visibility string `json:"visibility"`
	StartPoint Point  `json:"start_point"`
	EndPoint   Point  `json:"end_point"`
}

// Import represents an import/include statement
type Import struct {
	Module     string `json:"module"`
	Alias      string `json:"alias"`
	Items      []string `json:"items"`
	StartPoint Point  `json:"start_point"`
	EndPoint   Point  `json:"end_point"`
}

// LanguageCapabilities describes what transformations are available for a language
type LanguageCapabilities struct {
	Language        string              `json:"language"`
	Parsers         []string            `json:"parsers"`
	Transformations []TransformationType `json:"transformations"`
	Frameworks      []string            `json:"frameworks"`
	LaneSupport     []string            `json:"lane_support"`
}

// TransformationType defines types of transformations available
type TransformationType string

const (
	TransformationTypeCleanup     TransformationType = "cleanup"
	TransformationTypeModernize   TransformationType = "modernize" 
	TransformationTypeMigration   TransformationType = "migration"
	TransformationTypeSecurity    TransformationType = "security"
	TransformationTypeRefactor    TransformationType = "refactor"
	TransformationTypeOptimize    TransformationType = "optimize"
	TransformationTypeWASM        TransformationType = "wasm"
)

// Transformation represents a code transformation operation
type Transformation struct {
	ID          string                 `json:"id"`
	Type        TransformationType     `json:"type"`
	Language    string                 `json:"language"`
	Changes     []CodeChange           `json:"changes"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// CodeChange represents a specific change to be made to code
type CodeChange struct {
	Type        string `json:"type"`
	StartByte   int    `json:"start_byte"`
	EndByte     int    `json:"end_byte"`
	OldText     string `json:"old_text"`
	NewText     string `json:"new_text"`
	Explanation string `json:"explanation"`
}

// Language-specific recipe types

// NodeJSRecipe represents Node.js specific transformation recipes
type NodeJSRecipe struct {
	Recipe
	PackageUpdates  map[string]string      `json:"package_updates"`
	ESLintRules     map[string]interface{} `json:"eslint_rules"`
	TypeScript      bool                   `json:"typescript"`
}

// PythonRecipe represents Python specific transformation recipes  
type PythonRecipe struct {
	Recipe
	PipUpdates      map[string]string      `json:"pip_updates"`
	PyUpgrade       string                 `json:"pyupgrade_target"`
	BlackConfig     map[string]interface{} `json:"black_config"`
}

// GoRecipe represents Go specific transformation recipes
type GoRecipe struct {
	Recipe
	GoModUpdates    map[string]string `json:"go_mod_updates"`
	GofmtOptions    []string          `json:"gofmt_options"`
	StaticCheck     []string          `json:"staticcheck_rules"`
}

// WASMRecipe represents WebAssembly specific transformation recipes
type WASMRecipe struct {
	Recipe
	OptimizationLevel   int          `json:"optimization_level"`
	TargetFeatures      []string     `json:"target_features"`
	PolyfillsRequired   []string     `json:"polyfills_required"`
	MemoryConfiguration MemoryConfig `json:"memory_config"`
}

// MemoryConfig defines WASM memory configuration
type MemoryConfig struct {
	InitialPages int  `json:"initial_pages"`
	MaximumPages int  `json:"maximum_pages"`
	Shared       bool `json:"shared"`
}

// TreeSitterMultiLanguageEngine implements multi-language parsing using tree-sitter
type TreeSitterMultiLanguageEngine struct {
	treeSitterPath string
	parserDir      string
	capabilities   map[string]*LanguageCapabilities
}

// NewTreeSitterMultiLanguageEngine creates a new multi-language engine
func NewTreeSitterMultiLanguageEngine() (*TreeSitterMultiLanguageEngine, error) {
	treeSitterPath := os.Getenv("ARF_TREE_SITTER_PATH")
	if treeSitterPath == "" {
		treeSitterPath = "/usr/local/bin/tree-sitter"
	}

	parserDir := os.Getenv("TREE_SITTER_PARSER_DIR")
	if parserDir == "" {
		parserDir = "/usr/local/lib/node_modules"
	}

	// Verify tree-sitter is available
	if _, err := os.Stat(treeSitterPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("tree-sitter not found at %s", treeSitterPath)
	}

	engine := &TreeSitterMultiLanguageEngine{
		treeSitterPath: treeSitterPath,
		parserDir:      parserDir,
		capabilities:   make(map[string]*LanguageCapabilities),
	}

	// Initialize language capabilities
	if err := engine.initializeCapabilities(); err != nil {
		return nil, fmt.Errorf("failed to initialize language capabilities: %w", err)
	}

	return engine, nil
}

// ParseAST parses source code into a universal AST using tree-sitter
func (e *TreeSitterMultiLanguageEngine) ParseAST(ctx context.Context, code string, language string) (*UniversalAST, error) {
	// Validate language support
	if supported, err := e.ValidateLanguageSupport(language); err != nil {
		return nil, err
	} else if !supported {
		return nil, fmt.Errorf("language %s is not supported", language)
	}

	// Create temporary file with source code
	tmpFile, err := e.createTempSourceFile(code, language)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary source file: %w", err)
	}
	defer os.Remove(tmpFile)

	// Parse using tree-sitter
	parserName := e.getTreeSitterParserName(language)
	cmd := exec.CommandContext(ctx, e.treeSitterPath, "parse", tmpFile, "--scope", parserName)
	cmd.Env = append(os.Environ(), fmt.Sprintf("NODE_PATH=%s", e.parserDir))

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parsing failed: %w", err)
	}

	// Parse tree-sitter output into UniversalAST
	ast, err := e.parseTreeSitterOutput(string(output), language)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tree-sitter output: %w", err)
	}

	// Extract symbols and imports
	symbols, err := e.extractSymbols(ast, language)
	if err != nil {
		return nil, fmt.Errorf("failed to extract symbols: %w", err)
	}

	imports, err := e.extractImports(ast, language)
	if err != nil {
		return nil, fmt.Errorf("failed to extract imports: %w", err)
	}

	ast.Symbols = symbols
	ast.Imports = imports
	ast.Metadata = map[string]interface{}{
		"original_code": code,
		"file_size":     len(code),
		"parsed_at":     time.Now(),
	}

	return ast, nil
}

// GenerateTransformation creates a transformation based on AST analysis and recipe
func (e *TreeSitterMultiLanguageEngine) GenerateTransformation(ctx context.Context, ast *UniversalAST, recipe Recipe) (*Transformation, error) {
	switch ast.Language {
	case "java":
		return e.generateJavaTransformation(ctx, ast, recipe)
	case "javascript", "typescript":
		return e.generateJavaScriptTransformation(ctx, ast, recipe)
	case "python":
		return e.generatePythonTransformation(ctx, ast, recipe)
	case "go":
		return e.generateGoTransformation(ctx, ast, recipe)
	case "rust":
		return e.generateRustTransformation(ctx, ast, recipe)
	default:
		return nil, fmt.Errorf("transformation generation not implemented for language: %s", ast.Language)
	}
}

// ValidateLanguageSupport checks if a language is supported
func (e *TreeSitterMultiLanguageEngine) ValidateLanguageSupport(language string) (bool, error) {
	_, exists := e.capabilities[language]
	return exists, nil
}

// GetLanguageCapabilities returns capabilities for a specific language
func (e *TreeSitterMultiLanguageEngine) GetLanguageCapabilities(language string) (*LanguageCapabilities, error) {
	caps, exists := e.capabilities[language]
	if !exists {
		return nil, fmt.Errorf("language %s is not supported", language)
	}
	return caps, nil
}

// Helper methods

func (e *TreeSitterMultiLanguageEngine) initializeCapabilities() error {
	// Define capabilities for each supported language
	languages := map[string]*LanguageCapabilities{
		"java": {
			Language: "java",
			Parsers:  []string{"tree-sitter-java"},
			Transformations: []TransformationType{
				TransformationTypeCleanup,
				TransformationTypeModernize,
				TransformationTypeMigration,
				TransformationTypeSecurity,
				TransformationTypeRefactor,
			},
			Frameworks: []string{"spring", "spring-boot", "junit", "maven", "gradle"},
			LaneSupport: []string{"C"}, // Lane C (OSv)
		},
		"javascript": {
			Language: "javascript",
			Parsers:  []string{"tree-sitter-javascript"},
			Transformations: []TransformationType{
				TransformationTypeCleanup,
				TransformationTypeModernize,
				TransformationTypeSecurity,
				TransformationTypeOptimize,
				TransformationTypeWASM,
			},
			Frameworks: []string{"react", "vue", "angular", "node", "express"},
			LaneSupport: []string{"B", "E", "G"}, // Lane B (Unikraft), E (OCI), G (WASM)
		},
		"typescript": {
			Language: "typescript",
			Parsers:  []string{"tree-sitter-typescript"},
			Transformations: []TransformationType{
				TransformationTypeCleanup,
				TransformationTypeModernize,
				TransformationTypeSecurity,
				TransformationTypeOptimize,
				TransformationTypeWASM,
			},
			Frameworks: []string{"react", "vue", "angular", "node", "express", "nest"},
			LaneSupport: []string{"B", "E", "G"}, // Lane B (Unikraft), E (OCI), G (WASM)
		},
		"python": {
			Language: "python",
			Parsers:  []string{"tree-sitter-python"},
			Transformations: []TransformationType{
				TransformationTypeCleanup,
				TransformationTypeModernize,
				TransformationTypeSecurity,
				TransformationTypeRefactor,
			},
			Frameworks: []string{"django", "flask", "fastapi", "pytest", "pip"},
			LaneSupport: []string{"C", "E"}, // Lane C (OSv), E (OCI)
		},
		"go": {
			Language: "go",
			Parsers:  []string{"tree-sitter-go"},
			Transformations: []TransformationType{
				TransformationTypeCleanup,
				TransformationTypeModernize,
				TransformationTypeSecurity,
				TransformationTypeOptimize,
				TransformationTypeWASM,
			},
			Frameworks: []string{"gin", "echo", "gorilla", "gorm", "go-mod"},
			LaneSupport: []string{"A", "E", "G"}, // Lane A (Unikraft), E (OCI), G (WASM)
		},
		"rust": {
			Language: "rust",
			Parsers:  []string{"tree-sitter-rust"},
			Transformations: []TransformationType{
				TransformationTypeCleanup,
				TransformationTypeModernize,
				TransformationTypeSecurity,
				TransformationTypeOptimize,
				TransformationTypeWASM,
			},
			Frameworks: []string{"tokio", "serde", "cargo", "wasm-bindgen"},
			LaneSupport: []string{"A", "E", "G"}, // Lane A (Unikraft), E (OCI), G (WASM)
		},
	}

	// Verify parsers are available
	for lang, caps := range languages {
		if available := e.checkParserAvailability(caps.Parsers); available {
			e.capabilities[lang] = caps
		}
	}

	if len(e.capabilities) == 0 {
		return fmt.Errorf("no language parsers are available")
	}

	return nil
}

func (e *TreeSitterMultiLanguageEngine) checkParserAvailability(parsers []string) bool {
	for _, parser := range parsers {
		parserPath := filepath.Join(e.parserDir, parser)
		if _, err := os.Stat(parserPath); err == nil {
			return true
		}
	}
	return false
}

func (e *TreeSitterMultiLanguageEngine) createTempSourceFile(code string, language string) (string, error) {
	ext := e.getFileExtension(language)
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("arf_parse_*.%s", ext))
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(code); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func (e *TreeSitterMultiLanguageEngine) getFileExtension(language string) string {
	extensions := map[string]string{
		"java":       "java",
		"javascript": "js",
		"typescript": "ts",
		"python":     "py",
		"go":         "go",
		"rust":       "rs",
		"c":          "c",
		"cpp":        "cpp",
	}

	if ext, exists := extensions[language]; exists {
		return ext
	}
	return "txt"
}

func (e *TreeSitterMultiLanguageEngine) getTreeSitterParserName(language string) string {
	parserNames := map[string]string{
		"java":       "source.java",
		"javascript": "source.js",
		"typescript": "source.ts",
		"python":     "source.python",
		"go":         "source.go",
		"rust":       "source.rust",
		"c":          "source.c",
		"cpp":        "source.cpp",
	}

	if name, exists := parserNames[language]; exists {
		return name
	}
	return "source." + language
}

func (e *TreeSitterMultiLanguageEngine) parseTreeSitterOutput(output string, language string) (*UniversalAST, error) {
	// Parse tree-sitter's S-expression output into AST nodes
	// This is a simplified parser - real implementation would be more robust
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty tree-sitter output")
	}

	ast := &UniversalAST{
		Language: language,
		Parser:   "tree-sitter",
	}

	// Parse the root node from the first line
	rootNode, err := e.parseSExpressionLine(lines[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse root node: %w", err)
	}

	ast.RootNode = rootNode
	return ast, nil
}

func (e *TreeSitterMultiLanguageEngine) parseSExpressionLine(line string) (*ASTNode, error) {
	// Simplified S-expression parser for tree-sitter output
	// Real implementation would use a proper parser
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "(") {
		return nil, fmt.Errorf("invalid S-expression: %s", line)
	}

	// Extract node type (first token after opening paren)
	parts := strings.Fields(line[1:])
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty node")
	}

	nodeType := parts[0]
	text := ""
	if len(parts) > 1 {
		// Join remaining parts as text content
		text = strings.Join(parts[1:], " ")
		// Remove trailing closing paren
		text = strings.TrimSuffix(text, ")")
	}

	return &ASTNode{
		Type:     nodeType,
		Text:     text,
		Children: []*ASTNode{}, // Would be populated by recursive parsing
		Metadata: make(map[string]interface{}),
	}, nil
}

func (e *TreeSitterMultiLanguageEngine) extractSymbols(ast *UniversalAST, language string) ([]Symbol, error) {
	var symbols []Symbol

	// Language-specific symbol extraction
	switch language {
	case "java":
		symbols = e.extractJavaSymbols(ast.RootNode)
	case "javascript", "typescript":
		symbols = e.extractJavaScriptSymbols(ast.RootNode)
	case "python":
		symbols = e.extractPythonSymbols(ast.RootNode)
	case "go":
		symbols = e.extractGoSymbols(ast.RootNode)
	case "rust":
		symbols = e.extractRustSymbols(ast.RootNode)
	}

	return symbols, nil
}

func (e *TreeSitterMultiLanguageEngine) extractImports(ast *UniversalAST, language string) ([]Import, error) {
	var imports []Import

	// Language-specific import extraction
	switch language {
	case "java":
		imports = e.extractJavaImports(ast.RootNode)
	case "javascript", "typescript":
		imports = e.extractJavaScriptImports(ast.RootNode)
	case "python":
		imports = e.extractPythonImports(ast.RootNode)
	case "go":
		imports = e.extractGoImports(ast.RootNode)
	case "rust":
		imports = e.extractRustImports(ast.RootNode)
	}

	return imports, nil
}

// Language-specific symbol extraction methods

func (e *TreeSitterMultiLanguageEngine) extractJavaSymbols(node *ASTNode) []Symbol {
	var symbols []Symbol
	
	// Look for class, method, and field declarations
	if node.Type == "class_declaration" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "class",
			Visibility: e.extractVisibilityFromNode(node),
		})
	} else if node.Type == "method_declaration" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "method",
			Visibility: e.extractVisibilityFromNode(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		symbols = append(symbols, e.extractJavaSymbols(child)...)
	}

	return symbols
}

func (e *TreeSitterMultiLanguageEngine) extractJavaScriptSymbols(node *ASTNode) []Symbol {
	var symbols []Symbol

	// Look for function, class, and variable declarations
	if node.Type == "function_declaration" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "function",
			Visibility: "public", // JavaScript defaults
		})
	} else if node.Type == "class_declaration" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "class",
			Visibility: "public",
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		symbols = append(symbols, e.extractJavaScriptSymbols(child)...)
	}

	return symbols
}

func (e *TreeSitterMultiLanguageEngine) extractPythonSymbols(node *ASTNode) []Symbol {
	var symbols []Symbol

	// Look for function and class definitions
	if node.Type == "function_definition" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "function",
			Visibility: e.extractPythonVisibility(node),
		})
	} else if node.Type == "class_definition" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "class",
			Visibility: "public", // Python default
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		symbols = append(symbols, e.extractPythonSymbols(child)...)
	}

	return symbols
}

func (e *TreeSitterMultiLanguageEngine) extractGoSymbols(node *ASTNode) []Symbol {
	var symbols []Symbol

	// Look for function, type, and variable declarations
	if node.Type == "function_declaration" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "function",
			Visibility: e.extractGoVisibility(node),
		})
	} else if node.Type == "type_declaration" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "type",
			Visibility: e.extractGoVisibility(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		symbols = append(symbols, e.extractGoSymbols(child)...)
	}

	return symbols
}

func (e *TreeSitterMultiLanguageEngine) extractRustSymbols(node *ASTNode) []Symbol {
	var symbols []Symbol

	// Look for function, struct, enum, and trait declarations
	if node.Type == "function_item" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "function",
			Visibility: e.extractRustVisibility(node),
		})
	} else if node.Type == "struct_item" {
		symbols = append(symbols, Symbol{
			Name: e.extractNameFromNode(node),
			Type: "struct",
			Visibility: e.extractRustVisibility(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		symbols = append(symbols, e.extractRustSymbols(child)...)
	}

	return symbols
}

// Language-specific import extraction methods

func (e *TreeSitterMultiLanguageEngine) extractJavaImports(node *ASTNode) []Import {
	var imports []Import

	if node.Type == "import_declaration" {
		imports = append(imports, Import{
			Module: e.extractImportModuleFromNode(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		imports = append(imports, e.extractJavaImports(child)...)
	}

	return imports
}

func (e *TreeSitterMultiLanguageEngine) extractJavaScriptImports(node *ASTNode) []Import {
	var imports []Import

	if node.Type == "import_statement" {
		imports = append(imports, Import{
			Module: e.extractImportModuleFromNode(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		imports = append(imports, e.extractJavaScriptImports(child)...)
	}

	return imports
}

func (e *TreeSitterMultiLanguageEngine) extractPythonImports(node *ASTNode) []Import {
	var imports []Import

	if node.Type == "import_statement" || node.Type == "import_from_statement" {
		imports = append(imports, Import{
			Module: e.extractImportModuleFromNode(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		imports = append(imports, e.extractPythonImports(child)...)
	}

	return imports
}

func (e *TreeSitterMultiLanguageEngine) extractGoImports(node *ASTNode) []Import {
	var imports []Import

	if node.Type == "import_declaration" {
		imports = append(imports, Import{
			Module: e.extractImportModuleFromNode(node),
		})
	}

	// Recursively process children  
	for _, child := range node.Children {
		imports = append(imports, e.extractGoImports(child)...)
	}

	return imports
}

func (e *TreeSitterMultiLanguageEngine) extractRustImports(node *ASTNode) []Import {
	var imports []Import

	if node.Type == "use_declaration" {
		imports = append(imports, Import{
			Module: e.extractImportModuleFromNode(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		imports = append(imports, e.extractRustImports(child)...)
	}

	return imports
}

// Helper methods for extracting information from AST nodes

func (e *TreeSitterMultiLanguageEngine) extractNameFromNode(node *ASTNode) string {
	// Look for identifier in children or text content
	for _, child := range node.Children {
		if child.Type == "identifier" {
			return child.Text
		}
	}
	return node.Text
}

func (e *TreeSitterMultiLanguageEngine) extractVisibilityFromNode(node *ASTNode) string {
	// Look for visibility modifiers in children
	for _, child := range node.Children {
		if child.Type == "modifiers" || child.Type == "visibility_modifier" {
			return child.Text
		}
	}
	return "package" // Default for Java
}

func (e *TreeSitterMultiLanguageEngine) extractPythonVisibility(node *ASTNode) string {
	name := e.extractNameFromNode(node)
	if strings.HasPrefix(name, "__") {
		return "private"
	} else if strings.HasPrefix(name, "_") {
		return "protected"
	}
	return "public"
}

func (e *TreeSitterMultiLanguageEngine) extractGoVisibility(node *ASTNode) string {
	name := e.extractNameFromNode(node)
	if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
		return "public"
	}
	return "package"
}

func (e *TreeSitterMultiLanguageEngine) extractRustVisibility(node *ASTNode) string {
	// Look for pub keyword
	for _, child := range node.Children {
		if child.Type == "visibility_modifier" && child.Text == "pub" {
			return "public"
		}
	}
	return "private"
}

func (e *TreeSitterMultiLanguageEngine) extractImportModuleFromNode(node *ASTNode) string {
	// Extract module name from import statement
	for _, child := range node.Children {
		if child.Type == "scoped_identifier" || child.Type == "identifier" || child.Type == "string" {
			return child.Text
		}
	}
	return ""
}

// Language-specific transformation generators

func (e *TreeSitterMultiLanguageEngine) generateJavaTransformation(ctx context.Context, ast *UniversalAST, recipe Recipe) (*Transformation, error) {
	// Generate Java-specific transformations based on recipe
	transformation := &Transformation{
		ID:       fmt.Sprintf("java-transform-%d", time.Now().Unix()),
		Type:     TransformationType(recipe.Category),
		Language: "java",
		Changes:  []CodeChange{},
		Metadata: make(map[string]interface{}),
	}

	// Example: Remove unused imports transformation
	if recipe.ID == "cleanup.unused-imports" {
		changes := e.generateJavaUnusedImportsCleanup(ast)
		transformation.Changes = changes
	}

	return transformation, nil
}

func (e *TreeSitterMultiLanguageEngine) generateJavaScriptTransformation(ctx context.Context, ast *UniversalAST, recipe Recipe) (*Transformation, error) {
	transformation := &Transformation{
		ID:       fmt.Sprintf("js-transform-%d", time.Now().Unix()),
		Type:     TransformationType(recipe.Category),
		Language: ast.Language,
		Changes:  []CodeChange{},
		Metadata: make(map[string]interface{}),
	}

	return transformation, nil
}

func (e *TreeSitterMultiLanguageEngine) generatePythonTransformation(ctx context.Context, ast *UniversalAST, recipe Recipe) (*Transformation, error) {
	transformation := &Transformation{
		ID:       fmt.Sprintf("py-transform-%d", time.Now().Unix()),
		Type:     TransformationType(recipe.Category),
		Language: "python",
		Changes:  []CodeChange{},
		Metadata: make(map[string]interface{}),
	}

	return transformation, nil
}

func (e *TreeSitterMultiLanguageEngine) generateGoTransformation(ctx context.Context, ast *UniversalAST, recipe Recipe) (*Transformation, error) {
	transformation := &Transformation{
		ID:       fmt.Sprintf("go-transform-%d", time.Now().Unix()),
		Type:     TransformationType(recipe.Category),
		Language: "go",
		Changes:  []CodeChange{},
		Metadata: make(map[string]interface{}),
	}

	return transformation, nil
}

func (e *TreeSitterMultiLanguageEngine) generateRustTransformation(ctx context.Context, ast *UniversalAST, recipe Recipe) (*Transformation, error) {
	transformation := &Transformation{
		ID:       fmt.Sprintf("rust-transform-%d", time.Now().Unix()),
		Type:     TransformationType(recipe.Category),
		Language: "rust",
		Changes:  []CodeChange{},
		Metadata: make(map[string]interface{}),
	}

	return transformation, nil
}

func (e *TreeSitterMultiLanguageEngine) generateJavaUnusedImportsCleanup(ast *UniversalAST) []CodeChange {
	var changes []CodeChange

	// Analyze imports vs symbol usage to identify unused imports
	usedModules := make(map[string]bool)

	// Check which imported modules are actually used in symbols
	for _, symbol := range ast.Symbols {
		for _, imp := range ast.Imports {
			if strings.Contains(symbol.Name, imp.Module) {
				usedModules[imp.Module] = true
			}
		}
	}

	// Generate removal changes for unused imports
	for _, imp := range ast.Imports {
		if !usedModules[imp.Module] {
			changes = append(changes, CodeChange{
				Type:        "remove",
				StartByte:   imp.StartPoint.Row,
				EndByte:     imp.EndPoint.Row,
				OldText:     fmt.Sprintf("import %s;", imp.Module),
				NewText:     "",
				Explanation: fmt.Sprintf("Remove unused import: %s", imp.Module),
			})
		}
	}

	return changes
}

// MockMultiLanguageEngine implements MultiLanguageEngine interface for testing when Tree-sitter parsers are not available
type MockMultiLanguageEngine struct{}

// NewMockMultiLanguageEngine creates a new mock multi-language engine
func NewMockMultiLanguageEngine() (*MockMultiLanguageEngine, error) {
	return &MockMultiLanguageEngine{}, nil
}

// ParseAST creates a minimal mock AST for testing
func (m *MockMultiLanguageEngine) ParseAST(ctx context.Context, code string, language string) (*UniversalAST, error) {
	return &UniversalAST{
		Language: language,
		Parser:   "mock-parser",
		RootNode: &ASTNode{
			Type:  "program",
			Text:  "mock-program",
		},
		Symbols:  []Symbol{},
		Imports:  []Import{},
		Metadata: map[string]interface{}{
			"mock": true,
			"length": len(code),
		},
	}, nil
}

// GenerateTransformation creates a mock transformation
func (m *MockMultiLanguageEngine) GenerateTransformation(ctx context.Context, ast *UniversalAST, recipe Recipe) (*Transformation, error) {
	return &Transformation{
		ID:       "mock-transformation",
		Type:     TransformationTypeMigration,
		Language: ast.Language,
		Changes:  []CodeChange{},
		Metadata: map[string]interface{}{
			"mock":   true,
			"recipe": recipe,
		},
	}, nil
}

// ValidateLanguageSupport returns true for basic languages
func (m *MockMultiLanguageEngine) ValidateLanguageSupport(language string) (bool, error) {
	supportedLanguages := []string{"java", "javascript", "python", "go", "rust"}
	for _, lang := range supportedLanguages {
		if language == lang {
			return true, nil
		}
	}
	return false, nil
}

// GetLanguageCapabilities returns mock capabilities
func (m *MockMultiLanguageEngine) GetLanguageCapabilities(language string) (*LanguageCapabilities, error) {
	if supported, _ := m.ValidateLanguageSupport(language); !supported {
		return nil, fmt.Errorf("language %s not supported", language)
	}
	
	return &LanguageCapabilities{
		Language: language,
		Parsers:  []string{"mock-parser"},
		Transformations: []TransformationType{
			TransformationTypeCleanup,
			TransformationTypeModernize,
			TransformationTypeMigration,
		},
		Frameworks:  []string{"mock-framework"},
		LaneSupport: []string{"C"},
	}, nil
}