package chttp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	client := NewClient("https://test.chttp.example.com", "test-client", privateKey)

	assert.NotNil(t, client)
	assert.Equal(t, "https://test.chttp.example.com", client.baseURL)
	assert.Equal(t, "test-client", client.clientID)
	assert.Equal(t, privateKey, client.privateKey)
	assert.NotNil(t, client.httpClient)
}

func TestClient_Analyze(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Mock CHTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/analyze", r.URL.Path)

		// Verify required headers
		assert.NotEmpty(t, r.Header.Get("X-Client-ID"))
		assert.NotEmpty(t, r.Header.Get("X-Signature"))
		assert.Equal(t, "application/gzip", r.Header.Get("Content-Type"))

		// Mock successful response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "analysis-123",
			"status": "success",
			"timestamp": "2025-08-26T10:30:00Z",
			"result": {
				"issues": [
					{
						"file": "test.py",
						"line": 10,
						"severity": "error",
						"rule": "syntax-error",
						"message": "Invalid syntax"
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-client", privateKey)

	// Create test archive data
	archiveData := []byte("test archive data")

	result, err := client.Analyze(context.Background(), archiveData)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "analysis-123", result.ID)
	assert.Equal(t, "success", result.Status)
	assert.Len(t, result.Result.Issues, 1)
	assert.Equal(t, "test.py", result.Result.Issues[0].File)
	assert.Equal(t, 10, result.Result.Issues[0].Line)
	assert.Equal(t, "error", result.Result.Issues[0].Severity)
}

func TestClient_Analyze_WithTimeout(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "success"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-client", privateKey)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	archiveData := []byte("test data")
	_, err = client.Analyze(ctx, archiveData)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")
}

func TestClient_Analyze_ServerError(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Internal server error"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-client", privateKey)

	archiveData := []byte("test data")
	_, err = client.Analyze(context.Background(), archiveData)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server returned status 500")
}

func TestClient_Analyze_InvalidJSON(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-client", privateKey)

	archiveData := []byte("test data")
	_, err = client.Analyze(context.Background(), archiveData)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestClient_signRequest(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	client := NewClient("https://test.example.com", "test-client", privateKey)

	data := []byte("test data to sign")
	signature, err := client.signRequest(data)

	require.NoError(t, err)
	assert.NotEmpty(t, signature)

	// Verify signature is base64 encoded
	assert.Regexp(t, `^[A-Za-z0-9+/]+=*$`, signature)
}

func TestClient_signRequest_EmptyData(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	client := NewClient("https://test.example.com", "test-client", privateKey)

	signature, err := client.signRequest([]byte{})

	require.NoError(t, err)
	assert.NotEmpty(t, signature)
}