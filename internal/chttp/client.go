package chttp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents a CHTTP client for communicating with CHTTP services
type Client struct {
	baseURL    string
	httpClient *http.Client
	privateKey *rsa.PrivateKey
	clientID   string
}

// AnalysisResult represents the response from a CHTTP analysis request
type AnalysisResult struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Timestamp string    `json:"timestamp"`
	Result    Result    `json:"result"`
	Error     string    `json:"error,omitempty"`
}

// Result represents the analysis results
type Result struct {
	Issues []Issue `json:"issues"`
}

// Issue represents a single analysis issue
type Issue struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column,omitempty"`
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
}

// NewClient creates a new CHTTP client
func NewClient(baseURL, clientID string, privateKey *rsa.PrivateKey) *Client {
	return &Client{
		baseURL:    baseURL,
		clientID:   clientID,
		privateKey: privateKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Minute, // Default timeout for analysis
		},
	}
}

// Analyze sends a codebase archive to the CHTTP service for analysis
func (c *Client) Analyze(ctx context.Context, archiveData []byte) (*AnalysisResult, error) {
	// Create request
	url := c.baseURL + "/analyze"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(archiveData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("X-Client-ID", c.clientID)

	// Sign request
	signature, err := c.signRequest(archiveData)
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}
	req.Header.Set("X-Signature", signature)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result AnalysisResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// signRequest signs the request data using the private key
func (c *Client) signRequest(data []byte) (string, error) {
	// Create SHA-256 hash of the data
	hash := sha256.Sum256(data)

	// Sign the hash using the private key
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign data: %w", err)
	}

	// Encode signature as base64
	return base64.StdEncoding.EncodeToString(signature), nil
}