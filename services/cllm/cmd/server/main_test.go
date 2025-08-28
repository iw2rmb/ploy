package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_StartAndStop(t *testing.T) {
	server, err := NewServer()
	require.NoError(t, err, "NewServer should not fail")
	
	// Use a channel to track server start status
	started := make(chan error, 1)
	
	// Start server in background
	go func() {
		started <- server.Start(":8082")
	}()
	
	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)
	
	// Test health endpoint
	resp, err := http.Get("http://localhost:8082/health")
	require.NoError(t, err, "Health endpoint should be accessible")
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	// Stop server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	err = server.Shutdown(ctx)
	require.NoError(t, err, "Server shutdown should not fail")
	
	// Check that server exited properly
	select {
	case serverErr := <-started:
		// Server should exit with nil or ErrServerClosed after shutdown
		if serverErr != nil && serverErr != http.ErrServerClosed {
			t.Errorf("Server exited with unexpected error: %v", serverErr)
		}
	case <-time.After(1 * time.Second):
		t.Error("Server did not exit within timeout")
	}
}

func TestServer_HealthEndpoint(t *testing.T) {
	server, err := NewServer()
	require.NoError(t, err)
	
	// This will fail until we implement the health endpoint
	handler := server.GetHandler()
	require.NotNil(t, handler, "Server should have a handler")
}

func TestServer_ReadyEndpoint(t *testing.T) {
	server, err := NewServer()
	require.NoError(t, err)
	
	// Start server for testing
	go server.Start(":8083")
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	
	time.Sleep(200 * time.Millisecond)
	
	// Test ready endpoint
	resp, err := http.Get("http://localhost:8083/ready")
	require.NoError(t, err)
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_GracefulShutdown(t *testing.T) {
	server, err := NewServer()
	require.NoError(t, err)
	
	// Start server
	go server.Start(":8084")
	time.Sleep(200 * time.Millisecond)
	
	// Shutdown should complete within timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	err = server.Shutdown(ctx)
	assert.NoError(t, err, "Graceful shutdown should complete within timeout")
}

func TestNewServer_ConfigLoadFailure(t *testing.T) {
	// Set invalid environment variable to force config load failure
	os.Setenv("CLLM_SERVER_PORT", "invalid")
	defer os.Unsetenv("CLLM_SERVER_PORT")
	
	_, err := NewServer()
	assert.Error(t, err, "NewServer should fail with invalid config")
}

func TestServer_DefaultAddress(t *testing.T) {
	server, err := NewServer()
	require.NoError(t, err)
	
	// Test that server uses default address when empty string passed
	go func() {
		err := server.Start("")
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Server start failed: %v", err)
		}
	}()
	
	time.Sleep(200 * time.Millisecond)
	
	// Test default port 8082
	resp, err := http.Get("http://localhost:8082/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}