package arf

import (
	"context"
	"fmt"

	"github.com/iw2rmb/ploy/api/arf/models"
)

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
			Type: "program",
			Text: "mock-program",
		},
		Symbols: []Symbol{},
		Imports: []Import{},
		Metadata: map[string]interface{}{
			"mock":   true,
			"length": len(code),
		},
	}, nil
}

// GenerateTransformation creates a mock transformation
func (m *MockMultiLanguageEngine) GenerateTransformation(ctx context.Context, ast *UniversalAST, recipe *models.Recipe) (*Transformation, error) {
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
