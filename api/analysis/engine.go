package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Engine is the core analysis engine implementation
type Engine struct {
	analyzers         map[string]LanguageAnalyzer
	fallbackAnalyzers map[string]LanguageAnalyzer // Fallback analyzers for hybrid mode
	config            AnalysisConfig
	cache             CacheManager
	logger            *logrus.Logger
	mu                sync.RWMutex
	dispatcher        *AnalysisDispatcher // Nomad dispatcher for distributed analysis
}

// NewEngine creates a new analysis engine
func NewEngine(logger *logrus.Logger) *Engine {
	return &Engine{
		analyzers:         make(map[string]LanguageAnalyzer),
		fallbackAnalyzers: make(map[string]LanguageAnalyzer),
		config:            DefaultConfig(),
		logger:            logger,
		cache:             NewInMemoryCache(),
	}
}

// NewEngineWithDispatcher creates a new analysis engine with Nomad dispatcher
func NewEngineWithDispatcher(logger *logrus.Logger, dispatcher *AnalysisDispatcher) *Engine {
	engine := NewEngine(logger)
	engine.dispatcher = dispatcher
	
	// Register Nomad-based analyzers
	engine.RegisterAnalyzer("python", NewNomadPylintAnalyzer(dispatcher))
	engine.RegisterAnalyzer("javascript", NewNomadESLintAnalyzer(dispatcher))
	engine.RegisterAnalyzer("go", NewNomadGolangCIAnalyzer(dispatcher))
	
	return engine
}

// DefaultConfig returns the default analysis configuration
func DefaultConfig() AnalysisConfig {
	return AnalysisConfig{
		Enabled:        true,
		FailOnCritical: true,
		ARFIntegration: true,
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

// RegisterAnalyzer registers a language analyzer
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

// RegisterAnalyzerWithFallback registers a primary analyzer with fallback support (for hybrid mode)
func (e *Engine) RegisterAnalyzerWithFallback(language string, analyzer LanguageAnalyzer) error {
	// Register as primary analyzer
	return e.RegisterAnalyzer(language, analyzer)
}

// RegisterFallbackAnalyzer registers a fallback analyzer for hybrid mode
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

// GetAnalyzer gets a language analyzer
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

// GetSupportedLanguages returns all supported languages
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

// ConfigureAnalysis configures the analysis engine
func (e *Engine) ConfigureAnalysis(config AnalysisConfig) error {
	if err := e.ValidateConfiguration(config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.config = config
	
	// Configure individual analyzers if language-specific config exists
	for lang, cfg := range config.Languages {
		if analyzer, ok := e.analyzers[lang]; ok {
			if err := analyzer.Configure(cfg); err != nil {
				return fmt.Errorf("failed to configure %s analyzer: %w", lang, err)
			}
		}
	}
	
	return nil
}

// GetConfiguration returns the current configuration
func (e *Engine) GetConfiguration() AnalysisConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	return e.config
}

// ValidateConfiguration validates an analysis configuration
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

// AnalyzeRepository analyzes a repository
func (e *Engine) AnalyzeRepository(ctx context.Context, repo Repository) (*AnalysisResult, error) {
	// Create codebase from repository
	codebase := Codebase{
		Repository: repo,
		RootPath:   fmt.Sprintf("/tmp/analysis/%s", repo.ID),
		Languages:  make(map[string]int),
		Metadata:   repo.Metadata,
	}
	
	// TODO: Clone repository and detect files
	// For now, we'll use a placeholder implementation
	
	return e.AnalyzeCodebase(ctx, codebase, e.config)
}

// AnalyzeCodebase analyzes a codebase with the given configuration
func (e *Engine) AnalyzeCodebase(ctx context.Context, codebase Codebase, config AnalysisConfig) (*AnalysisResult, error) {
	startTime := time.Now()
	
	// Generate cache key
	cacheKey := e.generateCacheKey(codebase, config)
	
	// Check cache if enabled
	if config.CacheEnabled && e.cache != nil {
		if cached, found := e.cache.Get(cacheKey); found {
			e.logger.WithField("cache_key", cacheKey).Debug("Cache hit")
			return cached, nil
		}
	}
	
	// Create result
	result := &AnalysisResult{
		ID:              uuid.New().String(),
		Repository:      codebase.Repository,
		Timestamp:       time.Now(),
		LanguageResults: make(map[string]*LanguageAnalysisResult),
		Issues:          []Issue{},
		ARFTriggers:     []ARFTrigger{},
		Success:         true,
	}
	
	// Detect languages in codebase
	languages := e.detectLanguages(codebase)
	
	// Create wait group for parallel analysis
	var wg sync.WaitGroup
	var mu sync.Mutex
	
	// Analyze each language in parallel
	for _, lang := range languages {
		analyzer, err := e.GetAnalyzer(lang)
		if err != nil {
			e.logger.WithField("language", lang).Warn("No analyzer available")
			continue
		}
		
		wg.Add(1)
		go func(language string, analyzer LanguageAnalyzer) {
			defer wg.Done()
			
			// Create timeout context for this analyzer
			analyzerCtx, cancel := context.WithTimeout(ctx, config.Timeout)
			defer cancel()
			
			var langResult *LanguageAnalysisResult
			// Perform analysis using the registered analyzer
			e.logger.WithField("language", language).Debug("Starting analysis")
			langResult, analysisErr := analyzer.Analyze(analyzerCtx, codebase)
			
			// Handle analysis errors
			if analysisErr != nil {
				e.logger.WithError(analysisErr).WithField("language", language).Error("Analysis failed")
				langResult = &LanguageAnalysisResult{
					Language: language,
					Analyzer: analyzer.GetAnalyzerInfo().Name,
					Success:  false,
					Error:    analysisErr.Error(),
				}
			}
			
			// Store result
			mu.Lock()
			result.LanguageResults[language] = langResult
			if langResult.Issues != nil {
				result.Issues = append(result.Issues, langResult.Issues...)
			}
			mu.Unlock()
		}(lang, analyzer)
	}
	
	// Wait for all analyzers to complete
	wg.Wait()
	
	// Calculate metrics
	result.Metrics = e.calculateMetrics(result, time.Since(startTime))
	
	// Calculate overall score
	result.OverallScore = e.calculateScore(result)
	
	// Generate ARF triggers if integration is enabled
	if config.ARFIntegration {
		result.ARFTriggers = e.generateARFTriggers(result)
	}
	
	// Sort issues by severity and file
	e.sortIssues(result.Issues)
	
	// Limit issues if configured
	if config.MaxIssues > 0 && len(result.Issues) > config.MaxIssues {
		result.Issues = result.Issues[:config.MaxIssues]
	}
	
	// Cache result if enabled
	if config.CacheEnabled && e.cache != nil {
		if err := e.cache.Set(cacheKey, result, config.CacheTTL); err != nil {
			e.logger.WithError(err).Warn("Failed to cache result")
		}
	}
	
	// Check if we should fail
	if config.FailOnCritical {
		for _, issue := range result.Issues {
			if issue.Severity == SeverityCritical {
				result.Success = false
				result.Error = "Critical issues found"
				break
			}
		}
	}
	
	return result, nil
}

// GetAnalysisResult retrieves a stored analysis result
func (e *Engine) GetAnalysisResult(id string) (*AnalysisResult, error) {
	// TODO: Implement persistent storage
	return nil, fmt.Errorf("not implemented")
}

// ListAnalysisResults lists analysis results for a repository
func (e *Engine) ListAnalysisResults(repo Repository, limit int) ([]*AnalysisResult, error) {
	// TODO: Implement persistent storage
	return nil, fmt.Errorf("not implemented")
}

// ClearCache clears the cache for a repository
func (e *Engine) ClearCache(repo Repository) error {
	if e.cache != nil {
		return e.cache.Clear()
	}
	return nil
}

// detectLanguages detects languages in the codebase
func (e *Engine) detectLanguages(codebase Codebase) []string {
	languages := make(map[string]bool)
	
	// Use provided languages if available
	if len(codebase.Languages) > 0 {
		for lang := range codebase.Languages {
			languages[strings.ToLower(lang)] = true
		}
	}
	
	// Detect from files
	for _, file := range codebase.Files {
		ext := filepath.Ext(file)
		switch ext {
		case ".java":
			languages["java"] = true
		case ".py":
			languages["python"] = true
		case ".go":
			languages["go"] = true
		case ".js", ".jsx", ".ts", ".tsx":
			languages["javascript"] = true
		case ".cs":
			languages["csharp"] = true
		case ".rs":
			languages["rust"] = true
		case ".cpp", ".cc", ".cxx", ".c", ".h", ".hpp":
			languages["cpp"] = true
		}
	}
	
	// Convert to slice
	result := make([]string, 0, len(languages))
	for lang := range languages {
		result = append(result, lang)
	}
	
	return result
}

// generateCacheKey generates a cache key for the analysis
func (e *Engine) generateCacheKey(codebase Codebase, config AnalysisConfig) string {
	h := sha256.New()
	h.Write([]byte(codebase.Repository.ID))
	h.Write([]byte(codebase.Repository.Commit))
	h.Write([]byte(fmt.Sprintf("%v", config)))
	return hex.EncodeToString(h.Sum(nil))
}

// calculateMetrics calculates analysis metrics
func (e *Engine) calculateMetrics(result *AnalysisResult, duration time.Duration) AnalysisMetrics {
	metrics := AnalysisMetrics{
		TotalIssues:      len(result.Issues),
		IssuesBySeverity: make(map[string]int),
		IssuesByCategory: make(map[string]int),
		AnalysisTime:     duration,
	}
	
	// Count issues by severity and category
	for _, issue := range result.Issues {
		metrics.IssuesBySeverity[string(issue.Severity)]++
		metrics.IssuesByCategory[string(issue.Category)]++
	}
	
	// Aggregate language metrics
	for _, langResult := range result.LanguageResults {
		if langResult.Metrics.TotalFiles > 0 {
			metrics.TotalFiles += langResult.Metrics.TotalFiles
			metrics.AnalyzedFiles += langResult.Metrics.AnalyzedFiles
			metrics.SkippedFiles += langResult.Metrics.SkippedFiles
			metrics.CacheHits += langResult.Metrics.CacheHits
			metrics.CacheMisses += langResult.Metrics.CacheMisses
		}
	}
	
	return metrics
}

// calculateScore calculates an overall score for the analysis
func (e *Engine) calculateScore(result *AnalysisResult) float64 {
	if len(result.Issues) == 0 {
		return 100.0
	}
	
	// Weight issues by severity
	weights := map[SeverityLevel]float64{
		SeverityCritical: 10.0,
		SeverityHigh:     5.0,
		SeverityMedium:   2.0,
		SeverityLow:      1.0,
		SeverityInfo:     0.5,
	}
	
	totalWeight := 0.0
	for _, issue := range result.Issues {
		totalWeight += weights[issue.Severity]
	}
	
	// Calculate score (100 - weighted issues, min 0)
	score := 100.0 - totalWeight
	if score < 0 {
		score = 0
	}
	
	return score
}

// generateARFTriggers generates ARF triggers from issues
func (e *Engine) generateARFTriggers(result *AnalysisResult) []ARFTrigger {
	triggers := []ARFTrigger{}
	
	for _, issue := range result.Issues {
		if !issue.ARFCompatible {
			continue
		}
		
		// Check if analyzer can provide ARF recipes
		for lang, langResult := range result.LanguageResults {
			analyzer, err := e.GetAnalyzer(lang)
			if err != nil {
				continue
			}
			
			// Check if this issue belongs to this language
			found := false
			for _, langIssue := range langResult.Issues {
				if langIssue.ID == issue.ID {
					found = true
					break
				}
			}
			
			if !found {
				continue
			}
			
			// Get ARF recipes for this issue
			recipes := analyzer.GetARFRecipes(issue)
			for _, recipe := range recipes {
				trigger := ARFTrigger{
					IssueID:     issue.ID,
					RecipeName:  recipe,
					Priority:    e.getPriority(issue.Severity),
					AutoApprove: issue.Severity == SeverityLow || issue.Severity == SeverityInfo,
				}
				triggers = append(triggers, trigger)
			}
		}
	}
	
	return triggers
}

// getPriority maps severity to priority
func (e *Engine) getPriority(severity SeverityLevel) int {
	switch severity {
	case SeverityCritical:
		return 1
	case SeverityHigh:
		return 2
	case SeverityMedium:
		return 3
	case SeverityLow:
		return 4
	case SeverityInfo:
		return 5
	default:
		return 10
	}
}

// sortIssues sorts issues by severity and file
func (e *Engine) sortIssues(issues []Issue) {
	sort.Slice(issues, func(i, j int) bool {
		// First by severity
		if issues[i].Severity != issues[j].Severity {
			return e.getPriority(issues[i].Severity) < e.getPriority(issues[j].Severity)
		}
		// Then by file
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		// Then by line
		return issues[i].Line < issues[j].Line
	})
}

