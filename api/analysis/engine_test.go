package analysis

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

type mockAnalyzer struct {
	info         AnalyzerInfo
	result       *LanguageAnalysisResult
	err          error
	validateErr  error
	configureErr error
	autoFix      bool

	mu    sync.Mutex
	calls int
}

func newMockAnalyzer(name, language string) *mockAnalyzer {
	return &mockAnalyzer{
		info: AnalyzerInfo{
			Name:     name,
			Language: language,
		},
		result: &LanguageAnalysisResult{
			Language: language,
			Analyzer: name,
			Success:  true,
		},
	}
}

func (m *mockAnalyzer) Analyze(ctx context.Context, codebase Codebase) (*LanguageAnalysisResult, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	return m.result, nil
}

func (m *mockAnalyzer) GetSupportedFileTypes() []string {
	return []string{"*"}
}

func (m *mockAnalyzer) GetAnalyzerInfo() AnalyzerInfo {
	return m.info
}

func (m *mockAnalyzer) ValidateConfiguration(config interface{}) error {
	return m.validateErr
}

func (m *mockAnalyzer) Configure(config interface{}) error {
	return m.configureErr
}

func (m *mockAnalyzer) GenerateFixSuggestions(issue Issue) ([]FixSuggestion, error) {
	return nil, nil
}

func (m *mockAnalyzer) CanAutoFix(issue Issue) bool {
	return m.autoFix
}

func (m *mockAnalyzer) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func newTestEngine() *Engine {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return NewEngine(logger)
}

func TestEngineRegisterAnalyzerRejectsNil(t *testing.T) {
	engine := newTestEngine()

	if err := engine.RegisterAnalyzer("go", nil); err == nil {
		t.Fatalf("expected error when registering nil analyzer")
	}
}

func TestEngineRegisterAnalyzerWithFallbackRegistersPrimary(t *testing.T) {
	engine := newTestEngine()
	mock := newMockAnalyzer("primary", "go")

	if err := engine.RegisterAnalyzerWithFallback("Go", mock); err != nil {
		t.Fatalf("RegisterAnalyzerWithFallback: %v", err)
	}

	got, err := engine.GetAnalyzer("go")
	if err != nil {
		t.Fatalf("GetAnalyzer: %v", err)
	}
	if got != mock {
		t.Fatalf("expected registered analyzer to match mock")
	}
}

func TestEngineRegisterFallbackAnalyzerRejectsNil(t *testing.T) {
	engine := newTestEngine()

	if err := engine.RegisterFallbackAnalyzer("python", nil); err == nil {
		t.Fatalf("expected error when registering nil fallback analyzer")
	}
}

func TestEngineGetSupportedLanguagesSorted(t *testing.T) {
	engine := newTestEngine()

	if err := engine.RegisterAnalyzer("python", newMockAnalyzer("py", "python")); err != nil {
		t.Fatalf("register python: %v", err)
	}
	if err := engine.RegisterAnalyzer("go", newMockAnalyzer("go", "go")); err != nil {
		t.Fatalf("register go: %v", err)
	}

	langs := engine.GetSupportedLanguages()
	want := []string{"go", "python"}
	if len(langs) != len(want) {
		t.Fatalf("supported languages len = %d, want %d", len(langs), len(want))
	}
	for i, lang := range want {
		if langs[i] != lang {
			t.Fatalf("langs[%d] = %s, want %s", i, langs[i], lang)
		}
	}
}

func TestEngineGetAnalyzerMissing(t *testing.T) {
	engine := newTestEngine()

	if _, err := engine.GetAnalyzer("ruby"); err == nil {
		t.Fatalf("expected error when analyzer missing")
	}
}

func TestEngineAnalyzeCodebaseUsesCache(t *testing.T) {
	engine := newTestEngine()
	analyzer := newMockAnalyzer("go-analyzer", "go")

	if err := engine.RegisterAnalyzer("go", analyzer); err != nil {
		t.Fatalf("register go: %v", err)
	}

	codebase := Codebase{
		Repository: Repository{ID: "repo1", Commit: "abcdef"},
		Languages:  map[string]int{"go": 1},
	}

	config := DefaultConfig()

	ctx := context.Background()
	first, err := engine.AnalyzeCodebase(ctx, codebase, config)
	if err != nil {
		t.Fatalf("AnalyzeCodebase first call: %v", err)
	}
	second, err := engine.AnalyzeCodebase(ctx, codebase, config)
	if err != nil {
		t.Fatalf("AnalyzeCodebase second call: %v", err)
	}

	if analyzer.Calls() != 1 {
		t.Fatalf("expected analyzer called once, got %d", analyzer.Calls())
	}

	if first != second {
		t.Fatalf("expected cached result on second call")
	}
}

func TestEngineGetConfigurationReturnsSnapshot(t *testing.T) {
	engine := newTestEngine()
	config := DefaultConfig()
	config.MaxIssues = 42
	config.CacheEnabled = false

	if err := engine.ConfigureAnalysis(config); err != nil {
		t.Fatalf("ConfigureAnalysis: %v", err)
	}

	got := engine.GetConfiguration()
	if got.MaxIssues != 42 || got.CacheEnabled {
		t.Fatalf("unexpected configuration snapshot: %#v", got)
	}
}

func TestEngineClearCacheEvictsResults(t *testing.T) {
	engine := newTestEngine()
	analyzer := newMockAnalyzer("go-analyzer", "go")

	if err := engine.RegisterAnalyzer("go", analyzer); err != nil {
		t.Fatalf("register go: %v", err)
	}

	codebase := Codebase{
		Repository: Repository{ID: "repo2", Commit: "abcdef"},
		Languages:  map[string]int{"go": 1},
	}
	config := DefaultConfig()

	if _, err := engine.AnalyzeCodebase(context.Background(), codebase, config); err != nil {
		t.Fatalf("first analysis: %v", err)
	}
	if err := engine.ClearCache(codebase.Repository); err != nil {
		t.Fatalf("clear cache: %v", err)
	}
	if _, err := engine.AnalyzeCodebase(context.Background(), codebase, config); err != nil {
		t.Fatalf("second analysis after clear: %v", err)
	}

	if analyzer.Calls() != 2 {
		t.Fatalf("expected analyzer called twice after cache clear, got %d", analyzer.Calls())
	}
}

func TestEngineAnalyzeCodebaseHandlesAnalyzerError(t *testing.T) {
	engine := newTestEngine()
	failing := newMockAnalyzer("failure", "go")
	failing.err = errors.New("boom")

	if err := engine.RegisterAnalyzer("go", failing); err != nil {
		t.Fatalf("register go: %v", err)
	}

	codebase := Codebase{
		Repository: Repository{ID: "repo3", Commit: "abcdef"},
		Languages:  map[string]int{"go": 1},
	}

	config := DefaultConfig()

	result, err := engine.AnalyzeCodebase(context.Background(), codebase, config)
	if err != nil {
		t.Fatalf("AnalyzeCodebase: %v", err)
	}

	langResult := result.LanguageResults["go"]
	if langResult == nil {
		t.Fatalf("expected language result for go")
	}
	if langResult.Success {
		t.Fatalf("expected failure result for go analyzer")
	}
	if langResult.Error == "" {
		t.Fatalf("expected error message for go analyzer failure")
	}
	if langResult.Analyzer != failing.info.Name {
		t.Fatalf("expected analyzer name %s, got %s", failing.info.Name, langResult.Analyzer)
	}
}

func TestEngineAnalyzeCodebaseFallbackFailure(t *testing.T) {
	engine := newTestEngine()
	primary := newMockAnalyzer("primary", "go")
	primary.err = errors.New("primary exploded")
	fallback := newMockAnalyzer("fallback", "go")
	fallback.err = errors.New("fallback exploded")

	if err := engine.RegisterAnalyzer("go", primary); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	if err := engine.RegisterFallbackAnalyzer("go", fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	codebase := Codebase{
		Repository: Repository{ID: "repo-fallback", Commit: "abcdef"},
		Languages:  map[string]int{"go": 1},
	}

	result, err := engine.AnalyzeCodebase(context.Background(), codebase, DefaultConfig())
	if err != nil {
		t.Fatalf("AnalyzeCodebase: %v", err)
	}

	langResult := result.LanguageResults["go"]
	if langResult == nil {
		t.Fatalf("expected language result")
	}
	if langResult.Success {
		t.Fatalf("expected fallback failure to mark result unsuccessful")
	}
	if !strings.Contains(langResult.Error, "fallback analyzer failed") {
		t.Fatalf("expected fallback failure message, got %q", langResult.Error)
	}
	if langResult.Analyzer != "fallback" {
		t.Fatalf("expected fallback analyzer label, got %s", langResult.Analyzer)
	}
}

func TestEngineAnalyzeCodebaseFallsBackOnFailure(t *testing.T) {
	engine := newTestEngine()
	failing := newMockAnalyzer("primary", "go")
	failing.err = errors.New("primary failed")

	fallback := newMockAnalyzer("fallback", "go")
	fallback.result = &LanguageAnalysisResult{
		Language: "go",
		Analyzer: "fallback",
		Issues: []Issue{{
			ID:       "issue-1",
			Severity: SeverityHigh,
			Category: CategoryBug,
			Message:  "from fallback",
			File:     "main.go",
			Line:     10,
		}},
		Success: true,
	}

	if err := engine.RegisterAnalyzer("go", failing); err != nil {
		t.Fatalf("register primary: %v", err)
	}
	if err := engine.RegisterFallbackAnalyzer("go", fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	codebase := Codebase{
		Repository: Repository{ID: "repo4", Commit: "abcdef"},
		Languages:  map[string]int{"go": 1},
	}
	config := DefaultConfig()

	result, err := engine.AnalyzeCodebase(context.Background(), codebase, config)
	if err != nil {
		t.Fatalf("AnalyzeCodebase: %v", err)
	}

	langResult := result.LanguageResults["go"]
	if langResult == nil {
		t.Fatalf("expected language result for go")
	}
	if !langResult.Success {
		t.Fatalf("expected fallback analysis to succeed")
	}
	if langResult.Analyzer != "fallback" {
		t.Fatalf("expected fallback analyzer to be used, got %s", langResult.Analyzer)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected issues from fallback to bubble up: %d", len(result.Issues))
	}
}

func TestEngineFailOnCriticalIssueMarksResultFailed(t *testing.T) {
	engine := newTestEngine()
	critical := newMockAnalyzer("critical", "go")
	critical.result = &LanguageAnalysisResult{
		Language: "go",
		Analyzer: "critical",
		Issues: []Issue{{
			ID:       "critical-1",
			Severity: SeverityCritical,
			Category: CategorySecurity,
			Message:  "critical issue",
			File:     "main.go",
			Line:     5,
		}},
		Success: true,
		Metrics: AnalysisMetrics{TotalFiles: 1, AnalyzedFiles: 1},
	}

	if err := engine.RegisterAnalyzer("go", critical); err != nil {
		t.Fatalf("register critical: %v", err)
	}

	codebase := Codebase{
		Repository: Repository{ID: "repo5", Commit: "abcdef"},
		Languages:  map[string]int{"go": 1},
	}

	config := DefaultConfig()

	result, err := engine.AnalyzeCodebase(context.Background(), codebase, config)
	if err != nil {
		t.Fatalf("AnalyzeCodebase: %v", err)
	}

	if result.Success {
		t.Fatalf("expected overall analysis to fail due to critical issue")
	}
	if result.Error != "Critical issues found" {
		t.Fatalf("unexpected error message: %s", result.Error)
	}
	if result.OverallScore >= 100 {
		t.Fatalf("expected overall score to drop when issues found, got %f", result.OverallScore)
	}
}

func TestEngineAnalyzeRepositoryDelegatesToAnalyzeCodebase(t *testing.T) {
	engine := newTestEngine()
	engine.config.MaxIssues = 5

	result, err := engine.AnalyzeRepository(context.Background(), Repository{ID: "repo-x", Commit: "123"})
	if err != nil {
		t.Fatalf("AnalyzeRepository: %v", err)
	}
	if result.Repository.ID != "repo-x" {
		t.Fatalf("expected repository id propagated")
	}
	if !result.Success {
		t.Fatalf("expected success when no analyzers run")
	}
}

func TestEngineConfigureAnalysisValidation(t *testing.T) {
	engine := newTestEngine()

	config := DefaultConfig()
	config.Timeout = 0

	if err := engine.ConfigureAnalysis(config); err == nil {
		t.Fatalf("expected invalid configuration error")
	}
}

func TestEngineValidateConfigurationErrors(t *testing.T) {
	engine := newTestEngine()

	if err := engine.ValidateConfiguration(AnalysisConfig{Timeout: -1}); err == nil {
		t.Fatalf("expected timeout validation error")
	}
	if err := engine.ValidateConfiguration(AnalysisConfig{Timeout: time.Second, CacheTTL: -time.Second}); err == nil {
		t.Fatalf("expected cache ttl validation error")
	}
	if err := engine.ValidateConfiguration(AnalysisConfig{Timeout: time.Second, MaxIssues: -1}); err == nil {
		t.Fatalf("expected max issues validation error")
	}
}

func TestEngineDetectLanguagesFromFilesAndMetadata(t *testing.T) {
	engine := newTestEngine()
	codebase := Codebase{
		Languages: map[string]int{"GO": 2},
		Files: []string{
			"main.go",
			"service/service.py",
			"ui/app.tsx",
			"native/addon.cc",
		},
	}

	languages := engine.detectLanguages(codebase)
	want := map[string]bool{
		"go":         true,
		"python":     true,
		"javascript": true,
		"cpp":        true,
	}
	if len(languages) != len(want) {
		t.Fatalf("languages len = %d, want %d", len(languages), len(want))
	}
	for _, lang := range languages {
		if !want[lang] {
			t.Fatalf("unexpected language detected: %s", lang)
		}
	}
}

func TestEngineCalculateScoreWeightsSeverities(t *testing.T) {
	engine := newTestEngine()
	result := &AnalysisResult{
		Issues: []Issue{
			{Severity: SeverityCritical},
			{Severity: SeverityHigh},
			{Severity: SeverityMedium},
			{Severity: SeverityLow},
			{Severity: SeverityInfo},
		},
	}

	score := engine.calculateScore(result)
	if score != 100.0-18.5 {
		t.Fatalf("unexpected score: %f", score)
	}
}

func TestEngineGetPriorityDefaultCase(t *testing.T) {
	engine := newTestEngine()
	if p := engine.getPriority("unknown"); p != 10 {
		t.Fatalf("unexpected priority for unknown severity: %d", p)
	}
}

func TestEngineCalculateMetricsAggregatesLanguageMetrics(t *testing.T) {
	engine := newTestEngine()
	result := &AnalysisResult{
		Issues: []Issue{
			{Severity: SeverityCritical, Category: CategorySecurity},
			{Severity: SeverityLow, Category: CategoryMaintenance},
		},
		LanguageResults: map[string]*LanguageAnalysisResult{
			"go": {
				Metrics: AnalysisMetrics{TotalFiles: 3, AnalyzedFiles: 2, SkippedFiles: 1, CacheHits: 1, CacheMisses: 0},
			},
			"python": {
				Metrics: AnalysisMetrics{TotalFiles: 5, AnalyzedFiles: 4, SkippedFiles: 1, CacheHits: 0, CacheMisses: 1},
			},
		},
	}

	metrics := engine.calculateMetrics(result, 2*time.Minute)
	if metrics.TotalIssues != 2 {
		t.Fatalf("TotalIssues = %d, want 2", metrics.TotalIssues)
	}
	if metrics.IssuesBySeverity[string(SeverityCritical)] != 1 {
		t.Fatalf("critical count incorrect")
	}
	if metrics.IssuesByCategory[string(CategoryMaintenance)] != 1 {
		t.Fatalf("maintenance category count incorrect")
	}
	if metrics.TotalFiles != 8 || metrics.AnalyzedFiles != 6 || metrics.SkippedFiles != 2 {
		t.Fatalf("aggregated file metrics incorrect: %#v", metrics)
	}
	if metrics.CacheHits != 1 || metrics.CacheMisses != 1 {
		t.Fatalf("cache metrics incorrect: %#v", metrics)
	}
	if metrics.AnalysisTime <= 0 {
		t.Fatalf("expected analysis time to be captured")
	}
}

func TestEngineSortIssuesOrdersBySeverityFileAndLine(t *testing.T) {
	engine := newTestEngine()
	issues := []Issue{
		{Severity: SeverityLow, File: "b.go", Line: 20},
		{Severity: SeverityCritical, File: "c.go", Line: 5},
		{Severity: SeverityHigh, File: "a.go", Line: 10},
		{Severity: SeverityHigh, File: "a.go", Line: 5},
	}

	engine.sortIssues(issues)
	if issues[0].Severity != SeverityCritical {
		t.Fatalf("expected critical first")
	}
	if issues[1].Line != 5 || issues[1].File != "a.go" {
		t.Fatalf("expected high severity sorted by file/line")
	}
	if issues[2].Line != 10 {
		t.Fatalf("expected second high severity line ordering")
	}
}

func TestEngineGetAnalysisResultNotImplemented(t *testing.T) {
	engine := newTestEngine()
	if _, err := engine.GetAnalysisResult("id"); err == nil {
		t.Fatalf("expected not implemented error")
	}
}

func TestEngineListAnalysisResultsNotImplemented(t *testing.T) {
	engine := newTestEngine()
	if _, err := engine.ListAnalysisResults(Repository{ID: "repo"}, 1); err == nil {
		t.Fatalf("expected not implemented error")
	}
}
