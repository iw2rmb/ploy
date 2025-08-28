package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAIProvider implements the Provider interface for OpenAI
type OpenAIProvider struct {
	apiKey      string
	baseURL     string
	model       string
	httpClient  *http.Client
	timeout     time.Duration
	maxRetries  int
	retryDelay  time.Duration
}

// NewOpenAIProvider creates a new OpenAI provider instance
func NewOpenAIProvider(config ProviderConfig) (*OpenAIProvider, error) {
	apiKey := config.APIKey
	if apiKey == "" {
		// Try to get from environment variable
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	
	if apiKey == "" {
		return nil, &ProviderError{
			Provider: "openai",
			Type:     ErrTypeAuth,
			Message:  "OpenAI API key not provided",
		}
	}

	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com"
	}
	
	if config.Model == "" {
		config.Model = "gpt-4-turbo-preview"
	}
	
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	}

	return &OpenAIProvider{
		apiKey:     apiKey,
		baseURL:    strings.TrimSuffix(config.BaseURL, "/"),
		model:      config.Model,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		timeout:    config.Timeout,
		maxRetries: config.MaxRetries,
		retryDelay: config.RetryDelay,
	}, nil
}

// Name returns the provider name
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// IsAvailable checks if OpenAI API is accessible
func (p *OpenAIProvider) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetCapabilities returns OpenAI's capabilities
func (p *OpenAIProvider) GetCapabilities() Capabilities {
	maxContext := 8192
	if strings.Contains(p.model, "gpt-4") {
		maxContext = 128000
	}
	
	return Capabilities{
		SupportStreaming:    true,
		SupportFunctionCall: true,
		MaxContextLength:    maxContext,
		MaxOutputTokens:     4096,
		SupportedLanguages: []string{
			"java", "python", "javascript", "typescript", 
			"go", "rust", "c", "cpp", "csharp", "ruby",
			"php", "swift", "kotlin", "scala", "r",
		},
	}
}

// Complete generates a completion for the given request
func (p *OpenAIProvider) Complete(ctx context.Context, request CompletionRequest) (*CompletionResponse, error) {
	// Convert messages to OpenAI format
	openAIMessages := p.convertMessages(request)
	
	// Create OpenAI API request
	openAIReq := openAIChatRequest{
		Model:       p.selectModel(request.Model),
		Messages:    openAIMessages,
		Temperature: request.Temperature,
		MaxTokens:   request.MaxTokens,
		TopP:        request.TopP,
		Stop:        request.StopSequences,
		Stream:      false,
	}

	// Execute with retries
	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(p.retryDelay * time.Duration(attempt))
		}

		resp, err := p.executeRequest(ctx, openAIReq)
		if err != nil {
			lastErr = err
			// Check if error is retryable
			if perr, ok := err.(*ProviderError); ok {
				if perr.Type == ErrTypeRateLimit || perr.Type == ErrTypeTimeout {
					continue
				}
			}
			return nil, err
		}
		
		return resp, nil
	}

	return nil, lastErr
}

// CompleteStream generates a streaming completion
func (p *OpenAIProvider) CompleteStream(ctx context.Context, request CompletionRequest) (<-chan StreamChunk, error) {
	// Convert messages to OpenAI format
	openAIMessages := p.convertMessages(request)
	
	// Create OpenAI API request
	openAIReq := openAIChatRequest{
		Model:       p.selectModel(request.Model),
		Messages:    openAIMessages,
		Temperature: request.Temperature,
		MaxTokens:   request.MaxTokens,
		TopP:        request.TopP,
		Stop:        request.StopSequences,
		Stream:      true,
	}

	// Marshal request
	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to marshal request",
			Cause:    err,
		}
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to create request",
			Cause:    err,
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Execute request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeConnection,
			Message:  "failed to connect to OpenAI",
			Cause:    err,
		}
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, p.parseErrorResponse(resp.StatusCode, body)
	}

	// Create channel for streaming
	chunks := make(chan StreamChunk, 10)

	// Start goroutine to read SSE stream
	go func() {
		defer close(chunks)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				chunks <- StreamChunk{
					Error: &ProviderError{
						Provider: p.Name(),
						Type:     ErrTypeInternal,
						Message:  "failed to read stream",
						Cause:    err,
					},
				}
				return
			}

			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var streamResp openAIStreamResponse
			if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
				continue // Skip malformed chunks
			}

			if len(streamResp.Choices) > 0 {
				choice := streamResp.Choices[0]
				chunks <- StreamChunk{
					Delta:        choice.Delta.Content,
					FinishReason: choice.FinishReason,
				}

				if choice.FinishReason != "" {
					break
				}
			}
		}
	}()

	return chunks, nil
}

// ListModels returns available OpenAI models
func (p *OpenAIProvider) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to create request",
			Cause:    err,
		}
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeConnection,
			Message:  "failed to connect to OpenAI",
			Cause:    err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, p.parseErrorResponse(resp.StatusCode, body)
	}

	var modelsResp openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to decode response",
			Cause:    err,
		}
	}

	models := make([]Model, 0)
	for _, m := range modelsResp.Data {
		// Filter for chat models
		if strings.Contains(m.ID, "gpt") {
			models = append(models, Model{
				ID:          m.ID,
				Name:        m.ID,
				Description: fmt.Sprintf("Created by %s", m.OwnedBy),
				Metadata: map[string]interface{}{
					"owned_by": m.OwnedBy,
					"created":  m.Created,
				},
			})
		}
	}

	return models, nil
}

// Close cleans up provider resources
func (p *OpenAIProvider) Close() error {
	// No persistent connections to close for OpenAI
	return nil
}

// executeRequest executes a non-streaming request
func (p *OpenAIProvider) executeRequest(ctx context.Context, openAIReq openAIChatRequest) (*CompletionResponse, error) {
	// Marshal request
	reqBody, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to marshal request",
			Cause:    err,
		}
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to create request",
			Cause:    err,
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Execute request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeConnection,
			Message:  "failed to connect to OpenAI",
			Cause:    err,
		}
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, p.parseErrorResponse(resp.StatusCode, body)
	}

	// Parse response
	var openAIResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to decode response",
			Cause:    err,
		}
	}

	// Convert to CompletionResponse
	if len(openAIResp.Choices) == 0 {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "no choices returned in response",
		}
	}

	choice := openAIResp.Choices[0]
	return &CompletionResponse{
		ID:           openAIResp.ID,
		Model:        openAIResp.Model,
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage: Usage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
		Metadata: map[string]interface{}{
			"system_fingerprint": openAIResp.SystemFingerprint,
		},
		Created: time.Unix(openAIResp.Created, 0),
	}, nil
}

// convertMessages converts request messages to OpenAI format
func (p *OpenAIProvider) convertMessages(request CompletionRequest) []openAIMessage {
	messages := make([]openAIMessage, 0)
	
	// Add system prompt if provided
	if request.SystemPrompt != "" {
		messages = append(messages, openAIMessage{
			Role:    "system",
			Content: request.SystemPrompt,
		})
	}

	// Add request messages
	for _, msg := range request.Messages {
		messages = append(messages, openAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return messages
}

// selectModel chooses the model to use
func (p *OpenAIProvider) selectModel(requestModel string) string {
	if requestModel != "" {
		return requestModel
	}
	return p.model
}

// parseErrorResponse parses OpenAI error responses
func (p *OpenAIProvider) parseErrorResponse(statusCode int, body []byte) error {
	var openAIError openAIErrorResponse
	if err := json.Unmarshal(body, &openAIError); err == nil && openAIError.Error.Message != "" {
		errType := ErrTypeInternal
		switch statusCode {
		case http.StatusUnauthorized:
			errType = ErrTypeAuth
		case http.StatusTooManyRequests:
			errType = ErrTypeRateLimit
		case http.StatusRequestTimeout:
			errType = ErrTypeTimeout
		}
		
		return &ProviderError{
			Provider: p.Name(),
			Type:     errType,
			Message:  openAIError.Error.Message,
		}
	}

	return &ProviderError{
		Provider: p.Name(),
		Type:     ErrTypeInternal,
		Message:  fmt.Sprintf("OpenAI returned status %d: %s", statusCode, string(body)),
	}
}

// OpenAI API request/response structures

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float32         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	TopP        float32         `json:"top_p,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
	Choices           []openAIChoice `json:"choices"`
	Usage             openAIUsage    `json:"usage"`
}

type openAIChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIStreamResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []openAIStreamChoice `json:"choices"`
}

type openAIStreamChoice struct {
	Index        int                 `json:"index"`
	Delta        openAIStreamDelta   `json:"delta"`
	FinishReason string              `json:"finish_reason,omitempty"`
}

type openAIStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type openAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []openAIModel `json:"data"`
}

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}