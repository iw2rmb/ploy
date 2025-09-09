package arf

import (
	"context"
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// GoRecipe represents Go specific transformation recipes
type GoRecipe struct {
	*models.Recipe
	GoModUpdates map[string]string `json:"go_mod_updates"`
	GofmtOptions []string          `json:"gofmt_options"`
	StaticCheck  []string          `json:"staticcheck_rules"`
}

// extractGoSymbols extracts symbols from Go AST nodes
func extractGoSymbols(node *ASTNode) []Symbol {
	var symbols []Symbol

	// Look for function and type declarations
	if node.Type == "function_declaration" {
		symbols = append(symbols, Symbol{
			Name:       extractNameFromNode(node),
			Type:       "function",
			Visibility: extractGoVisibility(node),
		})
	} else if node.Type == "type_declaration" {
		symbols = append(symbols, Symbol{
			Name:       extractNameFromNode(node),
			Type:       "type",
			Visibility: extractGoVisibility(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		symbols = append(symbols, extractGoSymbols(child)...)
	}

	return symbols
}

// extractGoImports extracts import statements from Go AST nodes
func extractGoImports(node *ASTNode) []Import {
	var imports []Import

	if node.Type == "import_declaration" {
		imports = append(imports, Import{
			Module: extractImportModuleFromNode(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		imports = append(imports, extractGoImports(child)...)
	}

	return imports
}

// generateGoTransformation creates Go-specific transformations
func generateGoTransformation(ctx context.Context, ast *UniversalAST, recipe *models.Recipe) (*Transformation, error) {
	transformation := &Transformation{
		ID:       fmt.Sprintf("go-transform-%d", time.Now().Unix()),
		Type:     TransformationTypeRefactor, // Default type
		Language: "go",
		Changes:  []CodeChange{},
		Metadata: make(map[string]interface{}),
	}

	return transformation, nil
}

// extractGoVisibility determines visibility for Go symbols based on capitalization
func extractGoVisibility(node *ASTNode) string {
	name := extractNameFromNode(node)
	if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
		return "public"
	}
	return "package"
}
