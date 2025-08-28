package providers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MockProvider is a mock provider for testing
type MockProvider struct {
	mu                 sync.RWMutex
	name               string
	available          bool
	capabilities       Capabilities
	models             []Model
	responses          map[string]string
	streamResponses    map[string][]string
	errors             map[string]error
	completionDelay    time.Duration
	streamChunkDelay   time.Duration
	callCount          map[string]int
	lastRequest        *CompletionRequest
}

// NewMockProvider creates a new mock provider instance
func NewMockProvider(config ProviderConfig) (*MockProvider, error) {
	provider := &MockProvider{
		name:      "mock",
		available: true,
		capabilities: Capabilities{
			SupportStreaming:    true,
			SupportFunctionCall: true,
			MaxContextLength:    8192,
			MaxOutputTokens:     2048,
			SupportedLanguages: []string{
				"java", "python", "javascript", "go",
			},
		},
		models: []Model{
			{
				ID:          "mock-model-1",
				Name:        "Mock Model 1",
				Description: "A mock model for testing",
				ContextLength: 4096,
			},
			{
				ID:          "mock-model-2",
				Name:        "Mock Model 2",
				Description: "Another mock model for testing",
				ContextLength: 8192,
			},
		},
		responses:        make(map[string]string),
		streamResponses:  make(map[string][]string),
		errors:           make(map[string]error),
		callCount:        make(map[string]int),
		completionDelay:  100 * time.Millisecond,
		streamChunkDelay: 10 * time.Millisecond,
	}

	// Set default responses
	provider.SetDefaultResponses()

	// Apply configuration if provided
	if options, ok := config.Options["mock_config"].(map[string]interface{}); ok {
		if available, ok := options["available"].(bool); ok {
			provider.available = available
		}
		if delay, ok := options["completion_delay"].(time.Duration); ok {
			provider.completionDelay = delay
		}
	}

	return provider, nil
}

// Name returns the provider name
func (p *MockProvider) Name() string {
	return p.name
}

// IsAvailable checks if the provider is available
func (p *MockProvider) IsAvailable(ctx context.Context) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.available
}

// GetCapabilities returns the provider's capabilities
func (p *MockProvider) GetCapabilities() Capabilities {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.capabilities
}

// Complete generates a completion for the given request
func (p *MockProvider) Complete(ctx context.Context, request CompletionRequest) (*CompletionResponse, error) {
	p.mu.Lock()
	p.lastRequest = &request
	p.callCount["Complete"]++
	p.mu.Unlock()

	// Simulate processing delay
	select {
	case <-time.After(p.completionDelay):
	case <-ctx.Done():
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeTimeout,
			Message:  "request cancelled",
			Cause:    ctx.Err(),
		}
	}

	// Check for configured error
	p.mu.RLock()
	if err, exists := p.errors["Complete"]; exists && err != nil {
		p.mu.RUnlock()
		return nil, err
	}
	p.mu.RUnlock()

	// Generate response based on request
	responseContent := p.generateResponse(request)

	return &CompletionResponse{
		ID:           fmt.Sprintf("mock-%d", time.Now().Unix()),
		Model:        request.Model,
		Content:      responseContent,
		FinishReason: "stop",
		Usage: Usage{
			PromptTokens:     len(request.Messages) * 10,
			CompletionTokens: len(responseContent) / 4,
			TotalTokens:      len(request.Messages)*10 + len(responseContent)/4,
		},
		Metadata: map[string]interface{}{
			"mock": true,
		},
		Created: time.Now(),
	}, nil
}

// CompleteStream generates a streaming completion
func (p *MockProvider) CompleteStream(ctx context.Context, request CompletionRequest) (<-chan StreamChunk, error) {
	p.mu.Lock()
	p.lastRequest = &request
	p.callCount["CompleteStream"]++
	p.mu.Unlock()

	// Check for configured error
	p.mu.RLock()
	if err, exists := p.errors["CompleteStream"]; exists && err != nil {
		p.mu.RUnlock()
		return nil, err
	}
	p.mu.RUnlock()

	chunks := make(chan StreamChunk, 10)

	go func() {
		defer close(chunks)

		// Get stream response chunks
		responseChunks := p.getStreamResponse(request)

		for i, chunk := range responseChunks {
			select {
			case <-time.After(p.streamChunkDelay):
				finishReason := ""
				if i == len(responseChunks)-1 {
					finishReason = "stop"
				}
				chunks <- StreamChunk{
					Delta:        chunk,
					FinishReason: finishReason,
				}
			case <-ctx.Done():
				chunks <- StreamChunk{
					Error: &ProviderError{
						Provider: p.Name(),
						Type:     ErrTypeTimeout,
						Message:  "stream cancelled",
						Cause:    ctx.Err(),
					},
				}
				return
			}
		}
	}()

	return chunks, nil
}

// ListModels returns available models
func (p *MockProvider) ListModels(ctx context.Context) ([]Model, error) {
	p.mu.Lock()
	p.callCount["ListModels"]++
	p.mu.Unlock()

	p.mu.RLock()
	defer p.mu.RUnlock()

	if err, exists := p.errors["ListModels"]; exists && err != nil {
		return nil, err
	}

	return p.models, nil
}

// Close cleans up provider resources
func (p *MockProvider) Close() error {
	// Nothing to clean up for mock provider
	return nil
}

// SetAvailable sets the availability status
func (p *MockProvider) SetAvailable(available bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.available = available
}

// SetResponse sets a custom response for a specific prompt
func (p *MockProvider) SetResponse(prompt string, response string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responses[prompt] = response
}

// SetStreamResponse sets custom stream response chunks
func (p *MockProvider) SetStreamResponse(prompt string, chunks []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.streamResponses[prompt] = chunks
}

// SetError sets an error to be returned for a specific method
func (p *MockProvider) SetError(method string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errors[method] = err
}

// GetCallCount returns the number of times a method was called
func (p *MockProvider) GetCallCount(method string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.callCount[method]
}

// GetLastRequest returns the last request received
func (p *MockProvider) GetLastRequest() *CompletionRequest {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastRequest
}

// Reset resets the mock provider state
func (p *MockProvider) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callCount = make(map[string]int)
	p.lastRequest = nil
	p.errors = make(map[string]error)
}

// SetDefaultResponses sets up default mock responses
func (p *MockProvider) SetDefaultResponses() {
	p.responses["default"] = "This is a mock response for testing purposes."
	p.responses["error"] = "Error: This is a simulated error response."
	p.responses["code"] = `public class HelloWorld {
    public static void main(String[] args) {
        System.out.println("Hello, World!");
    }
}`
	
	p.streamResponses["default"] = []string{
		"This ", "is ", "a ", "streaming ", "mock ", "response ", "for ", "testing.",
	}
}

// generateResponse generates a response based on the request
func (p *MockProvider) generateResponse(request CompletionRequest) string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Check if there's a specific response for the prompt
	if len(request.Messages) > 0 {
		lastMessage := request.Messages[len(request.Messages)-1]
		if response, exists := p.responses[lastMessage.Content]; exists {
			return response
		}
		
		// Check for keyword-based responses
		content := strings.ToLower(lastMessage.Content)
		if strings.Contains(content, "error") {
			return p.responses["error"]
		}
		if strings.Contains(content, "code") || strings.Contains(content, "java") {
			return p.responses["code"]
		}
	}

	// Return default response
	return p.responses["default"]
}

// getStreamResponse gets stream response chunks
func (p *MockProvider) getStreamResponse(request CompletionRequest) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Check if there's a specific stream response
	if len(request.Messages) > 0 {
		lastMessage := request.Messages[len(request.Messages)-1]
		if chunks, exists := p.streamResponses[lastMessage.Content]; exists {
			return chunks
		}
	}

	// Return default stream response
	return p.streamResponses["default"]
}

// MockProviderBuilder provides a fluent interface for building mock providers
type MockProviderBuilder struct {
	provider *MockProvider
}

// NewMockProviderBuilder creates a new mock provider builder
func NewMockProviderBuilder() *MockProviderBuilder {
	provider, _ := NewMockProvider(ProviderConfig{})
	return &MockProviderBuilder{provider: provider}
}

// WithAvailability sets the availability status
func (b *MockProviderBuilder) WithAvailability(available bool) *MockProviderBuilder {
	b.provider.SetAvailable(available)
	return b
}

// WithResponse adds a custom response
func (b *MockProviderBuilder) WithResponse(prompt, response string) *MockProviderBuilder {
	b.provider.SetResponse(prompt, response)
	return b
}

// WithError sets an error for a method
func (b *MockProviderBuilder) WithError(method string, err error) *MockProviderBuilder {
	b.provider.SetError(method, err)
	return b
}

// WithDelay sets the completion delay
func (b *MockProviderBuilder) WithDelay(delay time.Duration) *MockProviderBuilder {
	b.provider.completionDelay = delay
	return b
}

// Build returns the configured mock provider
func (b *MockProviderBuilder) Build() *MockProvider {
	return b.provider
}