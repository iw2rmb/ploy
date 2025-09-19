package analysis

import "fmt"

// ConfigureAnalysis configures the analysis engine.
func (e *Engine) ConfigureAnalysis(config AnalysisConfig) error {
	if err := e.ValidateConfiguration(config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.config = config

	for lang, cfg := range config.Languages {
		if analyzer, ok := e.analyzers[lang]; ok {
			if err := analyzer.Configure(cfg); err != nil {
				return fmt.Errorf("failed to configure %s analyzer: %w", lang, err)
			}
		}
	}

	return nil
}

// GetConfiguration returns the current configuration.
func (e *Engine) GetConfiguration() AnalysisConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config
}

// ValidateConfiguration validates an analysis configuration.
func (e *Engine) ValidateConfiguration(config AnalysisConfig) error {
	if config.MaxIssues < 0 {
		return fmt.Errorf("max_issues must be non-negative")
	}

	if config.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if config.CacheTTL < 0 {
		return fmt.Errorf("cache_ttl must be non-negative")
	}

	return nil
}
