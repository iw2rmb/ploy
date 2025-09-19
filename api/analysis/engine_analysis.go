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
)

// AnalyzeRepository analyzes a repository.
func (e *Engine) AnalyzeRepository(ctx context.Context, repo Repository) (*AnalysisResult, error) {
	codebase := Codebase{
		Repository: repo,
		RootPath:   fmt.Sprintf("/tmp/analysis/%s", repo.ID),
		Languages:  make(map[string]int),
		Metadata:   repo.Metadata,
	}
	// TODO: Clone repository and detect files
	return e.AnalyzeCodebase(ctx, codebase, e.config)
}

// AnalyzeCodebase analyzes a codebase with the given configuration.
func (e *Engine) AnalyzeCodebase(ctx context.Context, codebase Codebase, config AnalysisConfig) (*AnalysisResult, error) {
	startTime := time.Now()
	cacheKey := e.generateCacheKey(codebase, config)

	if config.CacheEnabled && e.cache != nil {
		if cached, found := e.cache.Get(cacheKey); found {
			e.logger.WithField("cache_key", cacheKey).Debug("Cache hit")
			return cached, nil
		}
	}

	result := &AnalysisResult{
		ID:              uuid.New().String(),
		Repository:      codebase.Repository,
		Timestamp:       time.Now(),
		LanguageResults: make(map[string]*LanguageAnalysisResult),
		Issues:          []Issue{},
		Success:         true,
	}

	languages := e.detectLanguages(codebase)

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, lang := range languages {
		analyzer, err := e.GetAnalyzer(lang)
		if err != nil {
			e.logger.WithField("language", lang).Warn("No analyzer available")
			continue
		}

		wg.Add(1)
		go func(language string, analyzer LanguageAnalyzer) {
			defer wg.Done()

			analyzerCtx, cancel := context.WithTimeout(ctx, config.Timeout)
			defer cancel()

			e.logger.WithField("language", language).Debug("Starting analysis")
			langResult, analysisErr := analyzer.Analyze(analyzerCtx, codebase)
			if analysisErr != nil {
				e.logger.WithError(analysisErr).WithField("language", language).Error("Analysis failed")
				langResult = e.handleAnalyzerFailure(analyzerCtx, codebase, language, analyzer, analysisErr)
			}

			mu.Lock()
			result.LanguageResults[language] = langResult
			if langResult.Issues != nil {
				result.Issues = append(result.Issues, langResult.Issues...)
			}
			mu.Unlock()
		}(lang, analyzer)
	}

	wg.Wait()

	result.Metrics = e.calculateMetrics(result, time.Since(startTime))
	result.OverallScore = e.calculateScore(result)
	e.sortIssues(result.Issues)

	if config.MaxIssues > 0 && len(result.Issues) > config.MaxIssues {
		result.Issues = result.Issues[:config.MaxIssues]
	}

	if config.CacheEnabled && e.cache != nil {
		if err := e.cache.Set(cacheKey, result, config.CacheTTL); err != nil {
			e.logger.WithError(err).Warn("Failed to cache result")
		}
	}

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

// GetAnalysisResult retrieves a stored analysis result.
func (e *Engine) GetAnalysisResult(id string) (*AnalysisResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// ListAnalysisResults lists analysis results for a repository.
func (e *Engine) ListAnalysisResults(repo Repository, limit int) ([]*AnalysisResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// ClearCache clears the cache for a repository.
func (e *Engine) ClearCache(repo Repository) error {
	if e.cache != nil {
		return e.cache.Clear()
	}
	return nil
}

func (e *Engine) detectLanguages(codebase Codebase) []string {
	languages := make(map[string]bool)

	if len(codebase.Languages) > 0 {
		for lang := range codebase.Languages {
			languages[strings.ToLower(lang)] = true
		}
	}

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

	result := make([]string, 0, len(languages))
	for lang := range languages {
		result = append(result, lang)
	}
	return result
}

func (e *Engine) generateCacheKey(codebase Codebase, config AnalysisConfig) string {
	h := sha256.New()
	_, _ = h.Write([]byte(codebase.Repository.ID))
	_, _ = h.Write([]byte(codebase.Repository.Commit))
	_, _ = fmt.Fprintf(h, "%v", config)
	return hex.EncodeToString(h.Sum(nil))
}

func (e *Engine) calculateMetrics(result *AnalysisResult, duration time.Duration) AnalysisMetrics {
	metrics := AnalysisMetrics{
		TotalIssues:      len(result.Issues),
		IssuesBySeverity: make(map[string]int),
		IssuesByCategory: make(map[string]int),
		AnalysisTime:     duration,
	}

	for _, issue := range result.Issues {
		metrics.IssuesBySeverity[string(issue.Severity)]++
		metrics.IssuesByCategory[string(issue.Category)]++
	}

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

func (e *Engine) calculateScore(result *AnalysisResult) float64 {
	if len(result.Issues) == 0 {
		return 100.0
	}

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

	score := 100.0 - totalWeight
	if score < 0 {
		score = 0
	}
	return score
}

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

func (e *Engine) sortIssues(issues []Issue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return e.getPriority(issues[i].Severity) < e.getPriority(issues[j].Severity)
		}
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		return issues[i].Line < issues[j].Line
	})
}

func (e *Engine) handleAnalyzerFailure(ctx context.Context, codebase Codebase, language string, analyzer LanguageAnalyzer, primaryErr error) *LanguageAnalysisResult {
	fallback := e.getFallbackAnalyzer(language)
	if fallback == nil {
		return &LanguageAnalysisResult{
			Language: language,
			Analyzer: analyzer.GetAnalyzerInfo().Name,
			Success:  false,
			Error:    primaryErr.Error(),
		}
	}

	fallbackInfo := fallback.GetAnalyzerInfo()
	e.logger.WithError(primaryErr).
		WithField("language", language).
		WithField("fallback", fallbackInfo.Name).
		Warn("Primary analyzer failed, attempting fallback")

	fallbackResult, fallbackErr := fallback.Analyze(ctx, codebase)
	if fallbackErr != nil {
		e.logger.WithError(fallbackErr).
			WithField("language", language).
			WithField("fallback", fallbackInfo.Name).
			Error("Fallback analysis failed")
		return &LanguageAnalysisResult{
			Language: language,
			Analyzer: fallbackInfo.Name,
			Success:  false,
			Error:    fmt.Sprintf("fallback analyzer failed: %v (primary: %v)", fallbackErr, primaryErr),
		}
	}

	if fallbackResult == nil {
		fallbackResult = &LanguageAnalysisResult{
			Language: language,
			Analyzer: fallbackInfo.Name,
			Success:  true,
		}
	} else {
		if fallbackResult.Analyzer == "" {
			fallbackResult.Analyzer = fallbackInfo.Name
		}
		if fallbackResult.Language == "" {
			fallbackResult.Language = language
		}
		fallbackResult.Success = true
	}

	return fallbackResult
}
