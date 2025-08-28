package providers

import (
	"context"
	"io"
	"time"
)

// Provider defines the interface for LLM providers
type Provider interface {
	// Name returns the provider name
	Name() string

	// IsAvailable checks if the provider is available and configured
	IsAvailable(ctx context.Context) bool

	// GetCapabilities returns the provider's capabilities
	GetCapabilities() Capabilities

	// Complete generates a completion for the given request
	Complete(ctx context.Context, request CompletionRequest) (*CompletionResponse, error)

	// CompleteStream generates a streaming completion for the given request
	CompleteStream(ctx context.Context, request CompletionRequest) (<-chan StreamChunk, error)

	// ListModels returns available models for this provider
	ListModels(ctx context.Context) ([]Model, error)

	// Close cleans up provider resources
	Close() error
}

// Capabilities describes what a provider can do
type Capabilities struct {
	SupportStreaming    bool
	SupportFunctionCall bool
	MaxContextLength    int
	MaxOutputTokens     int
	SupportedLanguages  []string
}

// CompletionRequest represents a request for LLM completion
type CompletionRequest struct {
	// Model to use for completion
	Model string

	// Messages in the conversation
	Messages []Message

	// System prompt (optional, provider may include in messages)
	SystemPrompt string

	// Temperature controls randomness (0-2, typically 0.7)
	Temperature float32

	// MaxTokens limits the response length
	MaxTokens int

	// TopP for nucleus sampling
	TopP float32

	// Stop sequences to halt generation
	StopSequences []string

	// Stream indicates if streaming is desired
	Stream bool

	// Timeout for the request
	Timeout time.Duration

	// Additional provider-specific options
	Options map[string]interface{}
}

// Message represents a conversation message
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // Message content
}

// CompletionResponse represents the response from an LLM
type CompletionResponse struct {
	// ID is a unique identifier for this completion
	ID string

	// Model used for the completion
	Model string

	// Content is the generated text
	Content string

	// FinishReason indicates why generation stopped
	FinishReason string

	// Usage statistics
	Usage Usage

	// Provider-specific metadata
	Metadata map[string]interface{}

	// Created timestamp
	Created time.Time
}

// StreamChunk represents a chunk in a streaming response
type StreamChunk struct {
	// Delta content since last chunk
	Delta string

	// FinishReason if this is the last chunk
	FinishReason string

	// Error if something went wrong
	Error error
}

// Usage tracks token usage for a completion
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Model represents an available LLM model
type Model struct {
	// ID is the model identifier
	ID string

	// Name is the human-readable name
	Name string

	// Description of the model
	Description string

	// ContextLength is the maximum context size
	ContextLength int

	// Provider-specific metadata
	Metadata map[string]interface{}
}

// ProviderConfig contains common configuration for providers
type ProviderConfig struct {
	// Type of provider (ollama, openai, etc.)
	Type string

	// BaseURL for the provider API
	BaseURL string

	// APIKey for authentication (if required)
	APIKey string

	// Model is the default model to use
	Model string

	// Timeout for requests
	Timeout time.Duration

	// MaxRetries for failed requests
	MaxRetries int

	// RetryDelay between attempts
	RetryDelay time.Duration

	// Custom options for the provider
	Options map[string]interface{}
}

// ProviderFactory creates providers based on configuration
type ProviderFactory interface {
	// CreateProvider creates a provider instance
	CreateProvider(config ProviderConfig) (Provider, error)

	// RegisterProvider registers a custom provider type
	RegisterProvider(providerType string, constructor ProviderConstructor)
}

// ProviderConstructor is a function that creates a provider instance
type ProviderConstructor func(config ProviderConfig) (Provider, error)

// StreamReader is a helper interface for reading streaming responses
type StreamReader interface {
	io.Reader
	io.Closer
}

// Error types for provider operations
type ProviderError struct {
	Provider string
	Type     string
	Message  string
	Cause    error
}

func (e *ProviderError) Error() string {
	if e.Cause != nil {
		return e.Provider + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Provider + ": " + e.Message
}

func (e *ProviderError) Unwrap() error {
	return e.Cause
}

// Common error types
const (
	ErrTypeConnection   = "connection"
	ErrTypeAuth         = "authentication"
	ErrTypeRateLimit    = "rate_limit"
	ErrTypeInvalidModel = "invalid_model"
	ErrTypeTimeout      = "timeout"
	ErrTypeInternal     = "internal"
)

// CodeTransformationRequest represents a request for code transformation
type CodeTransformationRequest struct {
	// Code to transform
	Code string

	// Language of the code
	Language string

	// Error context if this is error correction
	ErrorContext string

	// Transformation type (e.g., "java11to17", "fix_compilation")
	TransformationType string

	// Additional context about the codebase
	CodebaseContext map[string]string

	// Instructions for the transformation
	Instructions string
}

// CodeTransformationResponse represents the transformation result
type CodeTransformationResponse struct {
	// TransformedCode is the modified code
	TransformedCode string

	// Diff is the git-style diff of changes
	Diff string

	// Explanation of changes made
	Explanation string

	// Confidence score (0-1)
	Confidence float32

	// Suggestions for additional changes
	Suggestions []string
}