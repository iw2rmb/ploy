package arf

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/services/cllm/internal/providers"
	"github.com/iw2rmb/ploy/services/cllm/internal/sandbox"
)

// MockLLMProvider implements providers.Provider for testing
type MockLLMProvider struct {
	CompleteFunc func(ctx context.Context, request providers.CompletionRequest) (*providers.CompletionResponse, error)
	NameFunc     func() string
}

func (m *MockLLMProvider) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock"
}

func (m *MockLLMProvider) IsAvailable(ctx context.Context) bool {
	return true
}

func (m *MockLLMProvider) GetCapabilities() providers.Capabilities {
	return providers.Capabilities{
		SupportStreaming:    false,
		SupportFunctionCall: false,
		MaxContextLength:    4096,
		MaxOutputTokens:     2048,
		SupportedLanguages:  []string{"java"},
	}
}

func (m *MockLLMProvider) Complete(ctx context.Context, request providers.CompletionRequest) (*providers.CompletionResponse, error) {
	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, request)
	}
	return &providers.CompletionResponse{
		ID:           "test-completion",
		Model:        "mock-model",
		Content:      "Mock analysis: Fix the compilation error by updating imports.",
		FinishReason: "stop",
		Created:      time.Now(),
	}, nil
}

func (m *MockLLMProvider) CompleteStream(ctx context.Context, request providers.CompletionRequest) (<-chan providers.StreamChunk, error) {
	return nil, nil
}

func (m *MockLLMProvider) ListModels(ctx context.Context) ([]providers.Model, error) {
	return []providers.Model{
		{
			ID:            "mock-model",
			Name:          "Mock Model",
			Description:   "Mock model for testing",
			ContextLength: 4096,
		},
	}, nil
}

func (m *MockLLMProvider) Close() error {
	return nil
}

// Test helper functions

func setupTestHandler() *Handler {
	mockProvider := &MockLLMProvider{}
	mockSandbox := &sandbox.Manager{} // Use the real Manager struct for tests
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	
	handler := NewHandler(mockProvider, mockSandbox, logger)
	return handler
}

func createTestApp(handler *Handler) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})
	
	app.Post("/v1/arf/analyze", handler.AnalyzeErrors)
	app.Get("/v1/arf/health", handler.Health)
	
	return app
}

func createValidARFRequest() ARFAnalysisRequest {
	return ARFAnalysisRequest{
		ProjectID: "test-project",
		Errors: []ErrorDetails{
			{
				Message:  "cannot find symbol: class ArrayList",
				File:     "src/main/java/com/example/Test.java",
				Line:     10,
				Type:     "compilation",
				Severity: "error",
				Context:  "import statement missing",
			},
		},
		CodeContext: CodeContext{
			Language:         "java",
			FrameworkVersion: "17",
			BuildTool:        "maven",
			Dependencies: []Dependency{
				{
					GroupID:    "org.springframework",
					ArtifactID: "spring-core",
					Version:    "5.3.0",
				},
			},
			SourceFiles: []SourceFile{
				{
					Path:      "src/main/java/com/example/Test.java",
					Content:   "public class Test { ArrayList<String> list; }",
					LineCount: 1,
				},
			},
		},
		TransformGoal:  "Fix compilation errors",
		AttemptNumber:  1,
		History:        []AttemptInfo{},
	}
}

// Tests

func TestHandler_AnalyzeErrors_Success(t *testing.T) {
	handler := setupTestHandler()
	app := createTestApp(handler)

	req := createValidARFRequest()
	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/v1/arf/analyze", bytes.NewReader(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var response ARFAnalysisResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response.Status)
	assert.NotEmpty(t, response.Analysis)
	assert.Greater(t, response.Confidence, 0.0)
	assert.NotEmpty(t, response.Metadata.RequestID)
	assert.Equal(t, "mock", response.Metadata.LLMProvider)
}

func TestHandler_AnalyzeErrors_InvalidRequest(t *testing.T) {
	handler := setupTestHandler()
	app := createTestApp(handler)

	// Empty request body
	httpReq := httptest.NewRequest("POST", "/v1/arf/analyze", bytes.NewReader([]byte("{}")))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var errorResp ARFErrorResponse
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)

	assert.Equal(t, "error", errorResp.Status)
	assert.Contains(t, errorResp.Message, "validation failed")
}

func TestHandler_AnalyzeErrors_MissingRequiredFields(t *testing.T) {
	handler := setupTestHandler()
	app := createTestApp(handler)

	tests := []struct {
		name        string
		modify      func(*ARFAnalysisRequest)
		expectedErr string
	}{
		{
			name: "missing project_id",
			modify: func(req *ARFAnalysisRequest) {
				req.ProjectID = ""
			},
			expectedErr: "project_id is required",
		},
		{
			name: "missing errors",
			modify: func(req *ARFAnalysisRequest) {
				req.Errors = nil
			},
			expectedErr: "at least one error is required",
		},
		{
			name: "missing transform_goal",
			modify: func(req *ARFAnalysisRequest) {
				req.TransformGoal = ""
			},
			expectedErr: "transform_goal is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createValidARFRequest()
			tt.modify(&req)

			reqBody, err := json.Marshal(req)
			require.NoError(t, err)

			httpReq := httptest.NewRequest("POST", "/v1/arf/analyze", bytes.NewReader(reqBody))
			httpReq.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(httpReq)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, 400, resp.StatusCode)

			var errorResp ARFErrorResponse
			err = json.NewDecoder(resp.Body).Decode(&errorResp)
			require.NoError(t, err)

			assert.Contains(t, errorResp.Errors[0].Details, tt.expectedErr)
		})
	}
}

func TestHandler_AnalyzeErrors_TooManyErrors(t *testing.T) {
	handler := setupTestHandler()
	app := createTestApp(handler)

	req := createValidARFRequest()
	// Add too many errors
	for i := 0; i < 51; i++ {
		req.Errors = append(req.Errors, ErrorDetails{
			Message: "test error",
			File:    "test.java",
			Line:    1,
			Type:    "compilation",
		})
	}

	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/v1/arf/analyze", bytes.NewReader(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	var errorResp ARFErrorResponse
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)

	assert.Contains(t, errorResp.Errors[0].Details, "too many errors")
}

func TestHandler_Health(t *testing.T) {
	handler := setupTestHandler()
	app := createTestApp(handler)

	httpReq := httptest.NewRequest("GET", "/v1/arf/health", nil)

	resp, err := app.Test(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var health map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)

	assert.Equal(t, "healthy", health["status"])
	assert.Equal(t, "arf-analyzer", health["service"])
}

func TestARFAnalysisOptions_Headers(t *testing.T) {
	handler := setupTestHandler()
	app := createTestApp(handler)

	req := createValidARFRequest()
	reqBody, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq := httptest.NewRequest("POST", "/v1/arf/analyze", bytes.NewReader(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Preferred-Model", "custom-model")
	httpReq.Header.Set("X-Debug-Mode", "true")

	resp, err := app.Test(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	
	// Should have request ID header
	assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))
}

func TestPatternMatcher_Integration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	matcher := NewPatternMatcher(logger)

	errors := []ErrorDetails{
		{
			Message:  "cannot find symbol: javax.servlet.http.HttpServlet",
			File:     "Test.java",
			Line:     1,
			Type:     "compilation",
			Severity: "error",
		},
	}

	context := CodeContext{
		Language: "java",
	}

	matches, err := matcher.FindPatterns(errors, context)
	require.NoError(t, err)

	assert.Greater(t, len(matches), 0)
	
	// Should detect javax to jakarta migration pattern
	found := false
	for _, match := range matches {
		if match.PatternID == "javax_to_jakarta" {
			found = true
			assert.Greater(t, match.Confidence, 0.9)
			break
		}
	}
	assert.True(t, found, "Should detect javax to jakarta migration pattern")
}

func BenchmarkHandler_AnalyzeErrors(b *testing.B) {
	handler := setupTestHandler()
	app := createTestApp(handler)

	req := createValidARFRequest()
	reqBody, _ := json.Marshal(req)

	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		httpReq := httptest.NewRequest("POST", "/v1/arf/analyze", bytes.NewReader(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(httpReq)
		if err != nil {
			b.Fatal(err)
		}
		resp.Body.Close()
	}
}