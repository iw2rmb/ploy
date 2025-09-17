package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
)

type stubEngine struct {
	analyzeResult      *AnalysisResult
	analyzeErr         error
	analyzeCodebaseRes *AnalysisResult
	analyzeCodebaseErr error

	getResult     *AnalysisResult
	getResultErr  error
	listResults   []*AnalysisResult
	listErr       error
	configureErr  error
	validateErr   error
	languages     []string
	config        AnalysisConfig
	analyzers     map[string]LanguageAnalyzer
	clearCacheErr error
	clearCalls    int

	registerCalls map[string]LanguageAnalyzer
}

func newStubEngine() *stubEngine {
	return &stubEngine{
		config:        DefaultConfig(),
		analyzers:     make(map[string]LanguageAnalyzer),
		registerCalls: make(map[string]LanguageAnalyzer),
	}
}

func (s *stubEngine) AnalyzeRepository(ctx context.Context, repo Repository) (*AnalysisResult, error) {
	if s.analyzeResult != nil || s.analyzeErr != nil {
		return s.analyzeResult, s.analyzeErr
	}
	return &AnalysisResult{ID: "default"}, nil
}

func (s *stubEngine) AnalyzeCodebase(ctx context.Context, codebase Codebase, config AnalysisConfig) (*AnalysisResult, error) {
	if s.analyzeCodebaseRes != nil || s.analyzeCodebaseErr != nil {
		return s.analyzeCodebaseRes, s.analyzeCodebaseErr
	}
	return &AnalysisResult{ID: "codebase"}, nil
}

func (s *stubEngine) RegisterAnalyzer(language string, analyzer LanguageAnalyzer) error {
	if analyzer == nil {
		return errors.New("nil analyzer")
	}
	language = strings.ToLower(language)
	s.registerCalls[language] = analyzer
	s.analyzers[language] = analyzer
	return nil
}

func (s *stubEngine) GetAnalyzer(language string) (LanguageAnalyzer, error) {
	analyzer, ok := s.analyzers[strings.ToLower(language)]
	if !ok {
		return nil, errors.New("missing analyzer")
	}
	return analyzer, nil
}

func (s *stubEngine) GetSupportedLanguages() []string {
	return s.languages
}

func (s *stubEngine) ConfigureAnalysis(config AnalysisConfig) error {
	if s.configureErr != nil {
		return s.configureErr
	}
	s.config = config
	return nil
}

func (s *stubEngine) GetConfiguration() AnalysisConfig {
	return s.config
}

func (s *stubEngine) ValidateConfiguration(config AnalysisConfig) error {
	return s.validateErr
}

func (s *stubEngine) GetAnalysisResult(id string) (*AnalysisResult, error) {
	if s.getResult != nil || s.getResultErr != nil {
		return s.getResult, s.getResultErr
	}
	return nil, errors.New("not found")
}

func (s *stubEngine) ListAnalysisResults(repo Repository, limit int) ([]*AnalysisResult, error) {
	if s.listResults != nil || s.listErr != nil {
		return s.listResults, s.listErr
	}
	return []*AnalysisResult{}, nil
}

func (s *stubEngine) ClearCache(repo Repository) error {
	s.clearCalls++
	return s.clearCacheErr
}

func setupTestHandler(engine AnalysisEngine) (*fiber.App, *Handler) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	app := fiber.New()
	handler := NewHandler(engine, logger)
	handler.RegisterRoutes(app)
	return app, handler
}

func parseJSONResponse(t *testing.T, res *http.Response) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return body
}

func TestHandlerAnalyzeRepositorySuccess(t *testing.T) {
	engine := newStubEngine()
	engine.analyzeResult = &AnalysisResult{ID: "analysis-1"}

	app, _ := setupTestHandler(engine)
	reqBody := AnalysisRequest{Repository: Repository{ID: "repo", Name: "Repo"}}
	payload, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/analysis/analyze", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body := parseJSONResponse(t, res)
	if body["id"] != "analysis-1" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestHandlerAnalyzeRepositoryValidation(t *testing.T) {
	engine := newStubEngine()
	app, _ := setupTestHandler(engine)

	req := httptest.NewRequest("POST", "/v1/analysis/analyze", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if res.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestHandlerAnalyzeRepositoryEngineError(t *testing.T) {
	engine := newStubEngine()
	engine.analyzeErr = errors.New("boom")

	app, _ := setupTestHandler(engine)
	payload, _ := json.Marshal(AnalysisRequest{Repository: Repository{ID: "repo"}})
	req := httptest.NewRequest("POST", "/v1/analysis/analyze", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if res.StatusCode != 500 {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
}

func TestHandlerGetAnalysisResult(t *testing.T) {
	engine := newStubEngine()
	engine.getResult = &AnalysisResult{ID: "res-1"}

	app, _ := setupTestHandler(engine)
	req := httptest.NewRequest("GET", "/v1/analysis/results/res-1", nil)

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body := parseJSONResponse(t, res)
	if body["id"] != "res-1" {
		t.Fatalf("unexpected response: %#v", body)
	}

	engine.getResult = nil
	engine.getResultErr = errors.New("missing")
	req = httptest.NewRequest("GET", "/v1/analysis/results/missing", nil)
	resMissing, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resMissing != nil && resMissing.Body != nil {
			_ = resMissing.Body.Close()
		}
	}()
	if resMissing.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resMissing.StatusCode)
	}
}

func TestHandlerListAnalysisResults(t *testing.T) {
	engine := newStubEngine()
	engine.listResults = []*AnalysisResult{{ID: "r1"}, {ID: "r2"}}

	app, _ := setupTestHandler(engine)

	req := httptest.NewRequest("GET", "/v1/analysis/results", nil)
	resMissingRepo, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resMissingRepo != nil && resMissingRepo.Body != nil {
			_ = resMissingRepo.Body.Close()
		}
	}()
	if resMissingRepo.StatusCode != 400 {
		t.Fatalf("status = %d, want 400 when repository_id missing", resMissingRepo.StatusCode)
	}

	req = httptest.NewRequest("GET", "/v1/analysis/results?repository_id=repo&limit=5", nil)
	resValid, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resValid != nil && resValid.Body != nil {
			_ = resValid.Body.Close()
		}
	}()
	if resValid.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resValid.StatusCode)
	}
	body := parseJSONResponse(t, resValid)
	if count, ok := body["count"].(float64); !ok || int(count) != 2 {
		t.Fatalf("expected count 2, got %#v", body["count"])
	}
}

func TestHandlerUpdateConfiguration(t *testing.T) {
	engine := newStubEngine()
	app, _ := setupTestHandler(engine)

	// invalid body
	req := httptest.NewRequest("PUT", "/v1/analysis/config", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	resInvalidBody, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resInvalidBody != nil && resInvalidBody.Body != nil {
			_ = resInvalidBody.Body.Close()
		}
	}()
	if resInvalidBody.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resInvalidBody.StatusCode)
	}

	// validation error
	engine.configureErr = errors.New("invalid config")
	cfg := DefaultConfig()
	cfg.Timeout = 5 * time.Minute
	payload, _ := json.Marshal(cfg)
	req = httptest.NewRequest("PUT", "/v1/analysis/config", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resValidationErr, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resValidationErr != nil && resValidationErr.Body != nil {
			_ = resValidationErr.Body.Close()
		}
	}()
	if resValidationErr.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resValidationErr.StatusCode)
	}

	engine.configureErr = nil
	req = httptest.NewRequest("PUT", "/v1/analysis/config", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resSuccess, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resSuccess != nil && resSuccess.Body != nil {
			_ = resSuccess.Body.Close()
		}
	}()
	if resSuccess.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resSuccess.StatusCode)
	}
}

func TestHandlerGetConfiguration(t *testing.T) {
	engine := newStubEngine()
	engine.config.MaxIssues = 77
	app, _ := setupTestHandler(engine)

	req := httptest.NewRequest("GET", "/v1/analysis/config", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body := parseJSONResponse(t, res)
	if issues, ok := body["max_issues"].(float64); !ok || int(issues) != 77 {
		t.Fatalf("unexpected config response: %#v", body)
	}
}

func TestHandlerValidateConfiguration(t *testing.T) {
	engine := newStubEngine()
	app, _ := setupTestHandler(engine)

	payload, _ := json.Marshal(DefaultConfig())
	req := httptest.NewRequest("POST", "/v1/analysis/config/validate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resValid, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resValid != nil && resValid.Body != nil {
			_ = resValid.Body.Close()
		}
	}()
	if resValid.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resValid.StatusCode)
	}
	body := parseJSONResponse(t, resValid)
	if valid, ok := body["valid"].(bool); !ok || !valid {
		t.Fatalf("expected valid true, got %#v", body)
	}

	engine.validateErr = errors.New("invalid")
	req = httptest.NewRequest("POST", "/v1/analysis/config/validate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resInvalid, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resInvalid != nil && resInvalid.Body != nil {
			_ = resInvalid.Body.Close()
		}
	}()
	if resInvalid.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resInvalid.StatusCode)
	}
	body = parseJSONResponse(t, resInvalid)
	if valid, ok := body["valid"].(bool); !ok || valid {
		t.Fatalf("expected valid false, got %#v", body)
	}
}

func TestHandlerGetSupportedLanguages(t *testing.T) {
	engine := newStubEngine()
	engine.languages = []string{"go", "python"}

	app, _ := setupTestHandler(engine)
	req := httptest.NewRequest("GET", "/v1/analysis/languages", nil)

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body := parseJSONResponse(t, res)
	if count, ok := body["count"].(float64); !ok || int(count) != 2 {
		t.Fatalf("expected count 2, got %#v", body["count"])
	}
}

func TestHandlerGetAnalyzerInfo(t *testing.T) {
	engine := newStubEngine()
	analyzer := newMockAnalyzer("go-an", "go")
	engine.analyzers["go"] = analyzer
	app, _ := setupTestHandler(engine)

	req := httptest.NewRequest("GET", "/v1/analysis/languages/go/info", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body := parseJSONResponse(t, res)
	if body["name"] != "go-an" {
		t.Fatalf("unexpected analyzer info: %#v", body)
	}

	req = httptest.NewRequest("GET", "/v1/analysis/languages/ruby/info", nil)
	resMissing, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resMissing != nil && resMissing.Body != nil {
			_ = resMissing.Body.Close()
		}
	}()
	if resMissing.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resMissing.StatusCode)
	}
}

func TestHandlerFixAndCacheEndpoints(t *testing.T) {
	engine := newStubEngine()
	app, _ := setupTestHandler(engine)

	req := httptest.NewRequest("GET", "/v1/analysis/issues/123/fixes", nil)
	resList, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resList != nil && resList.Body != nil {
			_ = resList.Body.Close()
		}
	}()
	if resList.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resList.StatusCode)
	}
	body := parseJSONResponse(t, resList)
	if body["issue_id"] != "123" {
		t.Fatalf("unexpected fix response: %#v", body)
	}

	req = httptest.NewRequest("POST", "/v1/analysis/issues/123/fix", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	resInvalid, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resInvalid != nil && resInvalid.Body != nil {
			_ = resInvalid.Body.Close()
		}
	}()
	if resInvalid.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resInvalid.StatusCode)
	}

	payload := []byte(`{"fix_index":0}`)
	req = httptest.NewRequest("POST", "/v1/analysis/issues/123/fix", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resApply, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resApply != nil && resApply.Body != nil {
			_ = resApply.Body.Close()
		}
	}()
	if resApply.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resApply.StatusCode)
	}

	req = httptest.NewRequest("DELETE", "/v1/analysis/cache?repository_id=repo", nil)
	resCache, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resCache != nil && resCache.Body != nil {
			_ = resCache.Body.Close()
		}
	}()
	if resCache.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resCache.StatusCode)
	}
	if engine.clearCalls != 1 {
		t.Fatalf("expected clear cache to be invoked once, got %d", engine.clearCalls)
	}

	req = httptest.NewRequest("GET", "/v1/analysis/cache/metrics", nil)
	resMetrics, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if resMetrics != nil && resMetrics.Body != nil {
			_ = resMetrics.Body.Close()
		}
	}()
	if resMetrics.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resMetrics.StatusCode)
	}
	body = parseJSONResponse(t, resMetrics)
	if body["hits"].(float64) != 0 {
		t.Fatalf("expected zero hits")
	}
}

func TestHandlerHealthCheck(t *testing.T) {
	engine := newStubEngine()
	engine.languages = []string{"go"}

	app, _ := setupTestHandler(engine)
	req := httptest.NewRequest("GET", "/v1/analysis/health", nil)

	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	body := parseJSONResponse(t, res)
	if body["status"] != "healthy" {
		t.Fatalf("unexpected health payload: %#v", body)
	}
}

func TestHandlerClearCacheError(t *testing.T) {
	engine := newStubEngine()
	engine.clearCacheErr = errors.New("boom")
	app, _ := setupTestHandler(engine)

	req := httptest.NewRequest("DELETE", "/v1/analysis/cache?repository_id=repo", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
	}()
	if res.StatusCode != 500 {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
}
