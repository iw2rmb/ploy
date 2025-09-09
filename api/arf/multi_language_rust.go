package arf

import (
	"context"
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// extractRustSymbols extracts symbols from Rust AST nodes
func extractRustSymbols(node *ASTNode) []Symbol {
	var symbols []Symbol

	// Look for function and struct declarations
	if node.Type == "function_item" {
		symbols = append(symbols, Symbol{
			Name:       extractNameFromNode(node),
			Type:       "function",
			Visibility: extractRustVisibility(node),
		})
	} else if node.Type == "struct_item" {
		symbols = append(symbols, Symbol{
			Name:       extractNameFromNode(node),
			Type:       "struct",
			Visibility: extractRustVisibility(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		symbols = append(symbols, extractRustSymbols(child)...)
	}

	return symbols
}

// extractRustImports extracts import statements from Rust AST nodes
func extractRustImports(node *ASTNode) []Import {
	var imports []Import

	if node.Type == "use_declaration" {
		imports = append(imports, Import{
			Module: extractImportModuleFromNode(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		imports = append(imports, extractRustImports(child)...)
	}

	return imports
}

// generateRustTransformation creates Rust-specific transformations
func generateRustTransformation(ctx context.Context, ast *UniversalAST, recipe *models.Recipe) (*Transformation, error) {
	transformation := &Transformation{
		ID:       fmt.Sprintf("rust-transform-%d", time.Now().Unix()),
		Type:     TransformationTypeRefactor, // Default type
		Language: "rust",
		Changes:  []CodeChange{},
		Metadata: make(map[string]interface{}),
	}

	return transformation, nil
}

// extractRustVisibility determines visibility for Rust symbols
func extractRustVisibility(node *ASTNode) string {
	// Look for pub keyword
	for _, child := range node.Children {
		if child.Type == "visibility_modifier" && child.Text == "pub" {
			return "public"
		}
	}
	return "private"
}
