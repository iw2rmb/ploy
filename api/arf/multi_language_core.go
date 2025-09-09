package arf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// MultiLanguageEngine defines the interface for universal AST parsing and transformation
type MultiLanguageEngine interface {
	ParseAST(ctx context.Context, code string, language string) (*UniversalAST, error)
	GenerateTransformation(ctx context.Context, ast *UniversalAST, recipe *models.Recipe) (*Transformation, error)
	ValidateLanguageSupport(language string) (bool, error)
	GetLanguageCapabilities(language string) (*LanguageCapabilities, error)
}

// UniversalAST represents a parsed Abstract Syntax Tree for any supported language
type UniversalAST struct {
	Language string                 `json:"language"`
	Parser   string                 `json:"parser"`
	RootNode *ASTNode               `json:"root_node"`
	Symbols  []Symbol               `json:"symbols"`
	Imports  []Import               `json:"imports"`
	Metadata map[string]interface{} `json:"metadata"`
}

// ASTNode represents a node in the Abstract Syntax Tree
type ASTNode struct {
	Type       string                 `json:"type"`
	Text       string                 `json:"text"`
	StartByte  int                    `json:"start_byte"`
	EndByte    int                    `json:"end_byte"`
	StartPoint Point                  `json:"start_point"`
	EndPoint   Point                  `json:"end_point"`
	Children   []*ASTNode             `json:"children"`
	Metadata   map[string]interface{} `json:"metadata"`
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
	Module     string   `json:"module"`
	Alias      string   `json:"alias"`
	Items      []string `json:"items"`
	StartPoint Point    `json:"start_point"`
	EndPoint   Point    `json:"end_point"`
}

// LanguageCapabilities describes what transformations are available for a language
type LanguageCapabilities struct {
	Language        string               `json:"language"`
	Parsers         []string             `json:"parsers"`
	Transformations []TransformationType `json:"transformations"`
	Frameworks      []string             `json:"frameworks"`
	LaneSupport     []string             `json:"lane_support"`
}

// TransformationType defines types of transformations available
type TransformationType string

const (
	TransformationTypeCleanup   TransformationType = "cleanup"
	TransformationTypeModernize TransformationType = "modernize"
	TransformationTypeMigration TransformationType = "migration"
	TransformationTypeSecurity  TransformationType = "security"
	TransformationTypeRefactor  TransformationType = "refactor"
	TransformationTypeOptimize  TransformationType = "optimize"
	TransformationTypeWASM      TransformationType = "wasm"
)

// Transformation represents a code transformation operation
type Transformation struct {
	ID       string                 `json:"id"`
	Type     TransformationType     `json:"type"`
	Language string                 `json:"language"`
	Changes  []CodeChange           `json:"changes"`
	Metadata map[string]interface{} `json:"metadata"`
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
func (e *TreeSitterMultiLanguageEngine) GenerateTransformation(ctx context.Context, ast *UniversalAST, recipe *models.Recipe) (*Transformation, error) {
	switch ast.Language {
	case "java":
		return generateJavaTransformation(ctx, ast, recipe)
	case "javascript", "typescript":
		return generateJavaScriptTransformation(ctx, ast, recipe)
	case "python":
		return generatePythonTransformation(ctx, ast, recipe)
	case "go":
		return generateGoTransformation(ctx, ast, recipe)
	case "rust":
		return generateRustTransformation(ctx, ast, recipe)
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

// Core helper methods

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
			Frameworks:  []string{"spring", "spring-boot", "junit", "maven", "gradle"},
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
			Frameworks:  []string{"react", "vue", "angular", "node", "express"},
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
			Frameworks:  []string{"react", "vue", "angular", "node", "express", "nest"},
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
			Frameworks:  []string{"django", "flask", "fastapi", "pytest", "pip"},
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
			Frameworks:  []string{"gin", "echo", "gorilla", "gorm", "go-mod"},
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
			Frameworks:  []string{"tokio", "serde", "cargo", "wasm-bindgen"},
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

// Dispatcher methods for symbols and imports

func (e *TreeSitterMultiLanguageEngine) extractSymbols(ast *UniversalAST, language string) ([]Symbol, error) {
	switch language {
	case "java":
		return extractJavaSymbols(ast.RootNode), nil
	case "javascript", "typescript":
		return extractJavaScriptSymbols(ast.RootNode), nil
	case "python":
		return extractPythonSymbols(ast.RootNode), nil
	case "go":
		return extractGoSymbols(ast.RootNode), nil
	case "rust":
		return extractRustSymbols(ast.RootNode), nil
	default:
		return []Symbol{}, nil
	}
}

func (e *TreeSitterMultiLanguageEngine) extractImports(ast *UniversalAST, language string) ([]Import, error) {
	switch language {
	case "java":
		return extractJavaImports(ast.RootNode), nil
	case "javascript", "typescript":
		return extractJavaScriptImports(ast.RootNode), nil
	case "python":
		return extractPythonImports(ast.RootNode), nil
	case "go":
		return extractGoImports(ast.RootNode), nil
	case "rust":
		return extractRustImports(ast.RootNode), nil
	default:
		return []Import{}, nil
	}
}
