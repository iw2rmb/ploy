package analysis

import (
	"fmt"
	"sort"
	"strings"
)

// RegisterAnalyzer registers a language analyzer.
func (e *Engine) RegisterAnalyzer(language string, analyzer LanguageAnalyzer) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if analyzer == nil {
		return fmt.Errorf("analyzer cannot be nil")
	}

	language = strings.ToLower(language)
	e.analyzers[language] = analyzer
	e.logger.WithField("language", language).Info("Registered analyzer")
	return nil
}

// RegisterAnalyzerWithFallback registers a primary analyzer with fallback support (for hybrid mode).
func (e *Engine) RegisterAnalyzerWithFallback(language string, analyzer LanguageAnalyzer) error {
	return e.RegisterAnalyzer(language, analyzer)
}

// RegisterFallbackAnalyzer registers a fallback analyzer for hybrid mode.
func (e *Engine) RegisterFallbackAnalyzer(language string, analyzer LanguageAnalyzer) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if analyzer == nil {
		return fmt.Errorf("analyzer cannot be nil")
	}

	language = strings.ToLower(language)
	e.fallbackAnalyzers[language] = analyzer
	e.logger.WithField("language", language).Info("Registered fallback analyzer")
	return nil
}

// GetAnalyzer gets a language analyzer.
func (e *Engine) GetAnalyzer(language string) (LanguageAnalyzer, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	language = strings.ToLower(language)
	analyzer, ok := e.analyzers[language]
	if !ok {
		return nil, fmt.Errorf("no analyzer registered for language: %s", language)
	}
	return analyzer, nil
}

// GetSupportedLanguages returns all supported languages.
func (e *Engine) GetSupportedLanguages() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	languages := make([]string, 0, len(e.analyzers))
	for lang := range e.analyzers {
		languages = append(languages, lang)
	}
	sort.Strings(languages)
	return languages
}
