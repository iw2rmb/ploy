package analysis

import (
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Engine is the core analysis engine implementation.
type Engine struct {
	analyzers         map[string]LanguageAnalyzer
	fallbackAnalyzers map[string]LanguageAnalyzer
	config            AnalysisConfig
	cache             CacheManager
	logger            *logrus.Logger
	mu                sync.RWMutex
	dispatcher        *AnalysisDispatcher
}

// NewEngine creates a new analysis engine.
func NewEngine(logger *logrus.Logger) *Engine {
	return &Engine{
		analyzers:         make(map[string]LanguageAnalyzer),
		fallbackAnalyzers: make(map[string]LanguageAnalyzer),
		config:            DefaultConfig(),
		logger:            logger,
		cache:             NewInMemoryCache(),
	}
}

// NewEngineWithDispatcher creates a new analysis engine with Nomad dispatcher.
func NewEngineWithDispatcher(logger *logrus.Logger, dispatcher *AnalysisDispatcher) *Engine {
	engine := NewEngine(logger)
	engine.dispatcher = dispatcher

	if err := engine.RegisterAnalyzer("python", NewNomadPylintAnalyzer(dispatcher)); err != nil {
		logger.WithError(err).Warn("Failed to register python analyzer")
	}
	if err := engine.RegisterAnalyzer("javascript", NewNomadESLintAnalyzer(dispatcher)); err != nil {
		logger.WithError(err).Warn("Failed to register javascript analyzer")
	}
	if err := engine.RegisterAnalyzer("go", NewNomadGolangCIAnalyzer(dispatcher)); err != nil {
		logger.WithError(err).Warn("Failed to register go analyzer")
	}

	return engine
}

// DefaultConfig returns the default analysis configuration.
func DefaultConfig() AnalysisConfig {
	return AnalysisConfig{
		Enabled:        true,
		FailOnCritical: true,
		MaxIssues:      1000,
		Timeout:        30 * time.Minute,
		CacheEnabled:   true,
		CacheTTL:       1 * time.Hour,
		Languages:      make(map[string]interface{}),
		ExcludePatterns: []string{
			"**/node_modules/**",
			"**/vendor/**",
			"**/.git/**",
			"**/build/**",
			"**/dist/**",
			"**/target/**",
		},
	}
}

func (e *Engine) getFallbackAnalyzer(language string) LanguageAnalyzer {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.fallbackAnalyzers[strings.ToLower(language)]
}
