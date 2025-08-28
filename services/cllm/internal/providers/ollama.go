package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaProvider implements the Provider interface for Ollama
type OllamaProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
	timeout    time.Duration
}

// NewOllamaProvider creates a new Ollama provider instance
func NewOllamaProvider(config ProviderConfig) (*OllamaProvider, error) {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}
	
	if config.Model == "" {
		config.Model = "codellama:7b"
	}
	
	if config.Timeout == 0 {
		config.Timeout = 120 * time.Second
	}

	return &OllamaProvider{
		baseURL: strings.TrimSuffix(config.BaseURL, "/"),
		model:   config.Model,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		timeout: config.Timeout,
	}, nil
}

// Name returns the provider name
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// IsAvailable checks if Ollama is running and accessible
func (p *OllamaProvider) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetCapabilities returns Ollama's capabilities
func (p *OllamaProvider) GetCapabilities() Capabilities {
	return Capabilities{
		SupportStreaming:    true,
		SupportFunctionCall: false,
		MaxContextLength:    4096,
		MaxOutputTokens:     2048,
		SupportedLanguages: []string{
			"java", "python", "javascript", "typescript", 
			"go", "rust", "c", "cpp", "csharp",
		},
	}
}

// Complete generates a completion for the given request
func (p *OllamaProvider) Complete(ctx context.Context, request CompletionRequest) (*CompletionResponse, error) {
	// Prepare the prompt
	prompt := p.buildPrompt(request)
	
	// Create Ollama API request
	ollamaReq := ollamaGenerateRequest{
		Model:  p.selectModel(request.Model),
		Prompt: prompt,
		Stream: false,
		Options: ollamaOptions{
			Temperature: request.Temperature,
			TopP:        request.TopP,
			NumPredict:  request.MaxTokens,
		},
	}

	// Marshal request
	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to marshal request",
			Cause:    err,
		}
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to create request",
			Cause:    err,
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeConnection,
			Message:  "failed to connect to Ollama",
			Cause:    err,
		}
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  fmt.Sprintf("Ollama returned status %d: %s", resp.StatusCode, string(body)),
		}
	}

	// Parse response
	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to decode response",
			Cause:    err,
		}
	}

	// Convert to CompletionResponse
	return &CompletionResponse{
		ID:           ollamaResp.CreatedAt,
		Model:        ollamaResp.Model,
		Content:      ollamaResp.Response,
		FinishReason: "stop",
		Usage: Usage{
			PromptTokens:     ollamaResp.PromptEvalCount,
			CompletionTokens: ollamaResp.EvalCount,
			TotalTokens:      ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
		},
		Metadata: map[string]interface{}{
			"eval_duration":        ollamaResp.EvalDuration,
			"prompt_eval_duration": ollamaResp.PromptEvalDuration,
			"total_duration":       ollamaResp.TotalDuration,
		},
		Created: time.Now(),
	}, nil
}

// CompleteStream generates a streaming completion
func (p *OllamaProvider) CompleteStream(ctx context.Context, request CompletionRequest) (<-chan StreamChunk, error) {
	// Prepare the prompt
	prompt := p.buildPrompt(request)
	
	// Create Ollama API request
	ollamaReq := ollamaGenerateRequest{
		Model:  p.selectModel(request.Model),
		Prompt: prompt,
		Stream: true,
		Options: ollamaOptions{
			Temperature: request.Temperature,
			TopP:        request.TopP,
			NumPredict:  request.MaxTokens,
		},
	}

	// Marshal request
	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to marshal request",
			Cause:    err,
		}
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to create request",
			Cause:    err,
		}
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeConnection,
			Message:  "failed to connect to Ollama",
			Cause:    err,
		}
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  fmt.Sprintf("Ollama returned status %d: %s", resp.StatusCode, string(body)),
		}
	}

	// Create channel for streaming
	chunks := make(chan StreamChunk, 10)

	// Start goroutine to read stream
	go func() {
		defer close(chunks)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for {
			var chunk ollamaStreamResponse
			if err := decoder.Decode(&chunk); err != nil {
				if err == io.EOF {
					break
				}
				chunks <- StreamChunk{
					Error: &ProviderError{
						Provider: p.Name(),
						Type:     ErrTypeInternal,
						Message:  "failed to decode stream chunk",
						Cause:    err,
					},
				}
				return
			}

			chunks <- StreamChunk{
				Delta:        chunk.Response,
				FinishReason: p.getFinishReason(chunk.Done),
			}

			if chunk.Done {
				break
			}
		}
	}()

	return chunks, nil
}

// ListModels returns available Ollama models
func (p *OllamaProvider) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to create request",
			Cause:    err,
		}
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeConnection,
			Message:  "failed to connect to Ollama",
			Cause:    err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  fmt.Sprintf("Ollama returned status %d: %s", resp.StatusCode, string(body)),
		}
	}

	var tagsResp ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, &ProviderError{
			Provider: p.Name(),
			Type:     ErrTypeInternal,
			Message:  "failed to decode response",
			Cause:    err,
		}
	}

	models := make([]Model, len(tagsResp.Models))
	for i, m := range tagsResp.Models {
		models[i] = Model{
			ID:          m.Name,
			Name:        m.Name,
			Description: fmt.Sprintf("Size: %d, Modified: %s", m.Size, m.ModifiedAt),
			Metadata: map[string]interface{}{
				"size":        m.Size,
				"digest":      m.Digest,
				"modified_at": m.ModifiedAt,
			},
		}
	}

	return models, nil
}

// Close cleans up provider resources
func (p *OllamaProvider) Close() error {
	// No persistent connections to close for Ollama
	return nil
}

// buildPrompt constructs the prompt from messages
func (p *OllamaProvider) buildPrompt(request CompletionRequest) string {
	var parts []string
	
	// Add system prompt if provided
	if request.SystemPrompt != "" {
		parts = append(parts, request.SystemPrompt)
	}

	// Add messages
	for _, msg := range request.Messages {
		switch msg.Role {
		case "system":
			parts = append(parts, msg.Content)
		case "user":
			parts = append(parts, "User: "+msg.Content)
		case "assistant":
			parts = append(parts, "Assistant: "+msg.Content)
		default:
			parts = append(parts, msg.Content)
		}
	}

	return strings.Join(parts, "\n\n")
}

// selectModel chooses the model to use
func (p *OllamaProvider) selectModel(requestModel string) string {
	if requestModel != "" {
		return requestModel
	}
	return p.model
}

// getFinishReason converts done flag to finish reason
func (p *OllamaProvider) getFinishReason(done bool) string {
	if done {
		return "stop"
	}
	return ""
}

// Ollama API request/response structures

type ollamaGenerateRequest struct {
	Model   string        `json:"model"`
	Prompt  string        `json:"prompt"`
	Stream  bool          `json:"stream"`
	Options ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float32 `json:"temperature,omitempty"`
	TopP        float32 `json:"top_p,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
	Stop        []string `json:"stop,omitempty"`
}

type ollamaGenerateResponse struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalCount          int    `json:"eval_count"`
	EvalDuration       int64  `json:"eval_duration"`
}

type ollamaStreamResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
}

type ollamaTagsResponse struct {
	Models []ollamaModel `json:"models"`
}

type ollamaModel struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
	Digest     string `json:"digest"`
}