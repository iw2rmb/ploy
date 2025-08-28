package providers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestMockProvider tests the mock provider functionality
func TestMockProvider(t *testing.T) {
	t.Run("Creation", func(t *testing.T) {
		config := ProviderConfig{
			Type: "mock",
		}
		provider, err := NewMockProvider(config)
		if err != nil {
			t.Fatalf("Failed to create mock provider: %v", err)
		}
		if provider.Name() != "mock" {
			t.Errorf("Expected provider name 'mock', got %s", provider.Name())
		}
	})

	t.Run("IsAvailable", func(t *testing.T) {
		provider := NewMockProviderBuilder().Build()
		ctx := context.Background()
		
		if !provider.IsAvailable(ctx) {
			t.Error("Expected provider to be available by default")
		}
		
		provider.SetAvailable(false)
		if provider.IsAvailable(ctx) {
			t.Error("Expected provider to be unavailable after setting")
		}
	})

	t.Run("GetCapabilities", func(t *testing.T) {
		provider := NewMockProviderBuilder().Build()
		caps := provider.GetCapabilities()
		
		if !caps.SupportStreaming {
			t.Error("Expected streaming support")
		}
		if caps.MaxContextLength != 8192 {
			t.Errorf("Expected max context length 8192, got %d", caps.MaxContextLength)
		}
		if len(caps.SupportedLanguages) == 0 {
			t.Error("Expected supported languages")
		}
	})

	t.Run("Complete", func(t *testing.T) {
		provider := NewMockProviderBuilder().
			WithResponse("test prompt", "test response").
			Build()
		
		ctx := context.Background()
		request := CompletionRequest{
			Messages: []Message{
				{Role: "user", Content: "test prompt"},
			},
			Temperature: 0.7,
			MaxTokens:   100,
		}
		
		response, err := provider.Complete(ctx, request)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		
		if response.Content != "test response" {
			t.Errorf("Expected 'test response', got %s", response.Content)
		}
		if response.FinishReason != "stop" {
			t.Errorf("Expected finish reason 'stop', got %s", response.FinishReason)
		}
		if provider.GetCallCount("Complete") != 1 {
			t.Errorf("Expected call count 1, got %d", provider.GetCallCount("Complete"))
		}
	})

	t.Run("CompleteWithError", func(t *testing.T) {
		expectedErr := &ProviderError{
			Provider: "mock",
			Type:     ErrTypeInternal,
			Message:  "test error",
		}
		
		provider := NewMockProviderBuilder().
			WithError("Complete", expectedErr).
			Build()
		
		ctx := context.Background()
		request := CompletionRequest{
			Messages: []Message{
				{Role: "user", Content: "test"},
			},
		}
		
		_, err := provider.Complete(ctx, request)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		
		var providerErr *ProviderError
		if !errors.As(err, &providerErr) {
			t.Fatalf("Expected ProviderError, got %T", err)
		}
		if providerErr.Message != expectedErr.Message {
			t.Errorf("Expected error message '%s', got '%s'", expectedErr.Message, providerErr.Message)
		}
	})

	t.Run("CompleteStream", func(t *testing.T) {
		provider := NewMockProviderBuilder().
			WithDelay(5 * time.Millisecond).
			Build()
		
		provider.SetStreamResponse("test", []string{"Hello", " ", "World"})
		
		ctx := context.Background()
		request := CompletionRequest{
			Messages: []Message{
				{Role: "user", Content: "test"},
			},
			Stream: true,
		}
		
		chunks, err := provider.CompleteStream(ctx, request)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		
		var result strings.Builder
		chunkCount := 0
		for chunk := range chunks {
			if chunk.Error != nil {
				t.Fatalf("Unexpected chunk error: %v", chunk.Error)
			}
			result.WriteString(chunk.Delta)
			chunkCount++
			
			if chunk.FinishReason == "stop" {
				break
			}
		}
		
		if result.String() != "Hello World" {
			t.Errorf("Expected 'Hello World', got '%s'", result.String())
		}
		if chunkCount != 3 {
			t.Errorf("Expected 3 chunks, got %d", chunkCount)
		}
	})

	t.Run("ListModels", func(t *testing.T) {
		provider := NewMockProviderBuilder().Build()
		
		ctx := context.Background()
		models, err := provider.ListModels(ctx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		
		if len(models) != 2 {
			t.Errorf("Expected 2 models, got %d", len(models))
		}
		
		if models[0].ID != "mock-model-1" {
			t.Errorf("Expected first model ID 'mock-model-1', got %s", models[0].ID)
		}
	})

	t.Run("LastRequest", func(t *testing.T) {
		provider := NewMockProviderBuilder().Build()
		
		ctx := context.Background()
		request := CompletionRequest{
			Model:       "test-model",
			Temperature: 0.5,
			MaxTokens:   200,
			Messages: []Message{
				{Role: "user", Content: "test message"},
			},
		}
		
		_, _ = provider.Complete(ctx, request)
		
		lastReq := provider.GetLastRequest()
		if lastReq == nil {
			t.Fatal("Expected last request to be stored")
		}
		if lastReq.Model != "test-model" {
			t.Errorf("Expected model 'test-model', got %s", lastReq.Model)
		}
		if lastReq.Temperature != 0.5 {
			t.Errorf("Expected temperature 0.5, got %f", lastReq.Temperature)
		}
	})

	t.Run("Reset", func(t *testing.T) {
		provider := NewMockProviderBuilder().Build()
		
		ctx := context.Background()
		request := CompletionRequest{
			Messages: []Message{{Role: "user", Content: "test"}},
		}
		
		_, _ = provider.Complete(ctx, request)
		if provider.GetCallCount("Complete") != 1 {
			t.Error("Expected call count to be 1 before reset")
		}
		
		provider.Reset()
		if provider.GetCallCount("Complete") != 0 {
			t.Error("Expected call count to be 0 after reset")
		}
		if provider.GetLastRequest() != nil {
			t.Error("Expected last request to be nil after reset")
		}
	})
}

// TestFactory tests the provider factory
func TestFactory(t *testing.T) {
	t.Run("CreateProvider", func(t *testing.T) {
		factory := NewFactory()
		
		// Test mock provider creation
		config := ProviderConfig{
			Type: "mock",
		}
		provider, err := factory.CreateProvider(config)
		if err != nil {
			t.Fatalf("Failed to create provider: %v", err)
		}
		if provider.Name() != "mock" {
			t.Errorf("Expected provider name 'mock', got %s", provider.Name())
		}
		
		// Test unknown provider type
		config.Type = "unknown"
		_, err = factory.CreateProvider(config)
		if err == nil {
			t.Error("Expected error for unknown provider type")
		}
	})

	t.Run("RegisterProvider", func(t *testing.T) {
		factory := NewFactory()
		
		// Register custom provider
		customCalled := false
		factory.RegisterProvider("custom", func(config ProviderConfig) (Provider, error) {
			customCalled = true
			return NewMockProviderBuilder().Build(), nil
		})
		
		config := ProviderConfig{
			Type: "custom",
		}
		provider, err := factory.CreateProvider(config)
		if err != nil {
			t.Fatalf("Failed to create custom provider: %v", err)
		}
		if !customCalled {
			t.Error("Custom constructor not called")
		}
		if provider == nil {
			t.Error("Expected provider, got nil")
		}
	})

	t.Run("GetProviderTypes", func(t *testing.T) {
		factory := NewFactory()
		types := factory.GetProviderTypes()
		
		// Check for built-in providers
		hasOllama := false
		hasOpenAI := false
		hasMock := false
		
		for _, t := range types {
			switch t {
			case "ollama":
				hasOllama = true
			case "openai":
				hasOpenAI = true
			case "mock":
				hasMock = true
			}
		}
		
		if !hasOllama {
			t.Error("Expected 'ollama' in provider types")
		}
		if !hasOpenAI {
			t.Error("Expected 'openai' in provider types")
		}
		if !hasMock {
			t.Error("Expected 'mock' in provider types")
		}
	})

	t.Run("DefaultFactory", func(t *testing.T) {
		// Test convenience functions that use default factory
		config := ProviderConfig{
			Type: "mock",
		}
		
		provider, err := CreateFromConfig(config)
		if err != nil {
			t.Fatalf("Failed to create provider with default factory: %v", err)
		}
		if provider.Name() != "mock" {
			t.Errorf("Expected provider name 'mock', got %s", provider.Name())
		}
		
		types := GetAvailableProviders()
		if len(types) < 3 {
			t.Error("Expected at least 3 provider types")
		}
	})
}

// TestProviderManager tests the provider manager
func TestProviderManager(t *testing.T) {
	t.Run("Creation", func(t *testing.T) {
		configs := []ProviderConfig{
			{Type: "mock"},
		}
		
		manager, err := NewProviderManager(configs)
		if err != nil {
			t.Fatalf("Failed to create provider manager: %v", err)
		}
		
		primary := manager.GetPrimary()
		if primary == nil {
			t.Error("Expected primary provider, got nil")
		}
		if primary.Name() != "mock" {
			t.Errorf("Expected primary provider 'mock', got %s", primary.Name())
		}
	})

	t.Run("NoConfigs", func(t *testing.T) {
		_, err := NewProviderManager([]ProviderConfig{})
		if err == nil {
			t.Error("Expected error for empty configs")
		}
	})

	t.Run("GetProvider", func(t *testing.T) {
		configs := []ProviderConfig{
			{Type: "mock"},
		}
		
		manager, _ := NewProviderManager(configs)
		
		provider, err := manager.GetProvider("mock")
		if err != nil {
			t.Fatalf("Failed to get provider: %v", err)
		}
		if provider.Name() != "mock" {
			t.Errorf("Expected provider 'mock', got %s", provider.Name())
		}
		
		_, err = manager.GetProvider("nonexistent")
		if err == nil {
			t.Error("Expected error for nonexistent provider")
		}
	})

	t.Run("SetPrimary", func(t *testing.T) {
		// Create two mock providers with different names
		mock1, _ := NewMockProvider(ProviderConfig{Type: "mock"})
		mock1.name = "mock1"
		mock2, _ := NewMockProvider(ProviderConfig{Type: "mock"})
		mock2.name = "mock2"
		
		manager := &ProviderManager{
			providers: []Provider{mock1, mock2},
			primary:   mock1,
		}
		
		err := manager.SetPrimary("mock2")
		if err != nil {
			t.Fatalf("Failed to set primary: %v", err)
		}
		
		primary := manager.GetPrimary()
		if primary.Name() != "mock2" {
			t.Errorf("Expected primary 'mock2', got %s", primary.Name())
		}
		
		err = manager.SetPrimary("nonexistent")
		if err == nil {
			t.Error("Expected error for nonexistent provider")
		}
	})

	t.Run("GetAvailableProvider", func(t *testing.T) {
		mock1 := NewMockProviderBuilder().WithAvailability(false).Build()
		mock1.name = "mock1"
		mock2 := NewMockProviderBuilder().WithAvailability(true).Build()
		mock2.name = "mock2"
		
		manager := &ProviderManager{
			providers: []Provider{mock1, mock2},
			primary:   mock1,
		}
		
		ctx := context.Background()
		available, err := manager.GetAvailableProvider(ctx)
		if err != nil {
			t.Fatalf("Failed to get available provider: %v", err)
		}
		if available.Name() != "mock2" {
			t.Errorf("Expected available provider 'mock2', got %s", available.Name())
		}
		
		// Make all unavailable
		mock2.SetAvailable(false)
		_, err = manager.GetAvailableProvider(ctx)
		if err == nil {
			t.Error("Expected error when no providers available")
		}
	})

	t.Run("Close", func(t *testing.T) {
		configs := []ProviderConfig{
			{Type: "mock"},
		}
		
		manager, _ := NewProviderManager(configs)
		err := manager.Close()
		if err != nil {
			t.Errorf("Unexpected error closing manager: %v", err)
		}
	})
}

// TestProviderError tests the provider error type
func TestProviderError(t *testing.T) {
	t.Run("ErrorFormatting", func(t *testing.T) {
		err := &ProviderError{
			Provider: "test",
			Type:     ErrTypeConnection,
			Message:  "connection failed",
		}
		
		expected := "test: connection failed"
		if err.Error() != expected {
			t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
		}
		
		cause := errors.New("underlying error")
		err.Cause = cause
		expected = "test: connection failed: underlying error"
		if err.Error() != expected {
			t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
		}
	})

	t.Run("ErrorUnwrap", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := &ProviderError{
			Provider: "test",
			Type:     ErrTypeInternal,
			Message:  "internal error",
			Cause:    cause,
		}
		
		unwrapped := err.Unwrap()
		if unwrapped != cause {
			t.Error("Unwrap did not return the correct cause")
		}
		
		// Test with errors.Is
		if !errors.Is(err, cause) {
			t.Error("errors.Is should find the underlying cause")
		}
	})
}

// TestCodeTransformation tests code transformation data structures
func TestCodeTransformation(t *testing.T) {
	t.Run("RequestCreation", func(t *testing.T) {
		req := CodeTransformationRequest{
			Code:               "public class Test {}",
			Language:           "java",
			TransformationType: "java11to17",
			Instructions:       "Update to Java 17",
		}
		
		if req.Language != "java" {
			t.Errorf("Expected language 'java', got %s", req.Language)
		}
		if req.TransformationType != "java11to17" {
			t.Errorf("Expected transformation type 'java11to17', got %s", req.TransformationType)
		}
	})

	t.Run("ResponseCreation", func(t *testing.T) {
		resp := CodeTransformationResponse{
			TransformedCode: "public sealed class Test {}",
			Diff:            "- public class Test {}\n+ public sealed class Test {}",
			Explanation:     "Added sealed modifier",
			Confidence:      0.95,
			Suggestions:     []string{"Consider adding permitted subclasses"},
		}
		
		if resp.Confidence != 0.95 {
			t.Errorf("Expected confidence 0.95, got %f", resp.Confidence)
		}
		if len(resp.Suggestions) != 1 {
			t.Errorf("Expected 1 suggestion, got %d", len(resp.Suggestions))
		}
	})
}