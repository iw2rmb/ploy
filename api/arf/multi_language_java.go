package arf

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// extractJavaSymbols extracts symbols from Java AST nodes
func extractJavaSymbols(node *ASTNode) []Symbol {
	var symbols []Symbol

	// Look for class, method, and field declarations
	if node.Type == "class_declaration" {
		symbols = append(symbols, Symbol{
			Name:       extractNameFromNode(node),
			Type:       "class",
			Visibility: extractJavaVisibility(node),
		})
	} else if node.Type == "method_declaration" {
		symbols = append(symbols, Symbol{
			Name:       extractNameFromNode(node),
			Type:       "method",
			Visibility: extractJavaVisibility(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		symbols = append(symbols, extractJavaSymbols(child)...)
	}

	return symbols
}

// extractJavaImports extracts import statements from Java AST nodes
func extractJavaImports(node *ASTNode) []Import {
	var imports []Import

	if node.Type == "import_declaration" {
		imports = append(imports, Import{
			Module: extractImportModuleFromNode(node),
		})
	}

	// Recursively process children
	for _, child := range node.Children {
		imports = append(imports, extractJavaImports(child)...)
	}

	return imports
}

// generateJavaTransformation creates Java-specific transformations
func generateJavaTransformation(ctx context.Context, ast *UniversalAST, recipe *models.Recipe) (*Transformation, error) {
	// Generate Java-specific transformations based on recipe
	transformation := &Transformation{
		ID:       fmt.Sprintf("java-transform-%d", time.Now().Unix()),
		Type:     TransformationTypeRefactor, // Default type
		Language: "java",
		Changes:  []CodeChange{},
		Metadata: make(map[string]interface{}),
	}

	// Example: Remove unused imports transformation
	if recipe.ID == "cleanup.unused-imports" {
		changes := generateJavaUnusedImportsCleanup(ast)
		transformation.Changes = changes
	}

	return transformation, nil
}

// generateJavaUnusedImportsCleanup generates code changes to remove unused Java imports
func generateJavaUnusedImportsCleanup(ast *UniversalAST) []CodeChange {
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

// extractJavaVisibility extracts visibility modifiers for Java symbols
func extractJavaVisibility(node *ASTNode) string {
	// Look for visibility modifiers in children
	for _, child := range node.Children {
		if child.Type == "modifiers" || child.Type == "visibility_modifier" {
			return child.Text
		}
	}
	return "package" // Default for Java
}

// Helper methods for extracting information from AST nodes

func extractNameFromNode(node *ASTNode) string {
	// Look for identifier in children or text content
	for _, child := range node.Children {
		if child.Type == "identifier" {
			return child.Text
		}
	}
	return node.Text
}

func extractImportModuleFromNode(node *ASTNode) string {
	// Extract module name from import statement
	for _, child := range node.Children {
		if child.Type == "scoped_identifier" || child.Type == "identifier" || child.Type == "string" {
			return child.Text
		}
	}
	return ""
}
