package server

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	tempDir := t.TempDir()
	configPath, cleanup := createTestConfig(t, tempDir)
	defer cleanup()
	
	server, err := NewServer(configPath)
	require.NoError(t, err)
	assert.NotNil(t, server)
	assert.NotNil(t, server.app)
	assert.NotNil(t, server.config)
	assert.NotNil(t, server.authManager)
	assert.NotNil(t, server.sandboxManager)
}

func TestNewServer_InvalidConfigPath(t *testing.T) {
	_, err := NewServer("/nonexistent/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestServer_HealthEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	configPath, cleanup := createTestConfig(t, tempDir)
	defer cleanup()
	
	server, err := NewServer(configPath)
	require.NoError(t, err)
	
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	
	assert.Equal(t, "ok", result["status"])
	assert.NotEmpty(t, result["timestamp"])
	assert.Equal(t, "test-service", result["service"])
}

func TestServer_AnalyzeEndpoint_Success(t *testing.T) {
	tempDir := t.TempDir()
	configPath, cleanup := createTestConfig(t, tempDir)
	defer cleanup()
	
	server, err := NewServer(configPath)
	require.NoError(t, err)
	
	// Create test archive data
	archiveData := createTestArchiveData(t)
	
	// Create proper signature for the request
	privateKey, signature := createTestSignatureForData(t, archiveData)
	
	// Update server with matching public key
	publicKeyPath := savePublicKey(t, tempDir, &privateKey.PublicKey)
	server.authManager, err = NewAuthManager(publicKeyPath)
	require.NoError(t, err)
	
	req := httptest.NewRequest(http.MethodPost, "/analyze", bytes.NewReader(archiveData))
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("X-Client-ID", "test-client")
	req.Header.Set("X-Signature", signature)
	
	resp, err := server.app.Test(req, 30000) // 30 second timeout
	require.NoError(t, err)
	
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	
	assert.Equal(t, "success", result["status"])
	assert.NotEmpty(t, result["id"])
	assert.NotEmpty(t, result["timestamp"])
	assert.Contains(t, result, "result")
}

func TestServer_AnalyzeEndpoint_MissingContentType(t *testing.T) {
	tempDir := t.TempDir()
	configPath, cleanup := createTestConfig(t, tempDir)
	defer cleanup()
	
	server, err := NewServer(configPath)
	require.NoError(t, err)
	
	req := httptest.NewRequest(http.MethodPost, "/analyze", bytes.NewReader([]byte("test")))
	// Missing Content-Type header
	req.Header.Set("X-Client-ID", "test-client")
	req.Header.Set("X-Signature", "test-signature")
	
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	
	assert.Contains(t, result["error"], "Content-Type must be application/gzip")
}

func TestServer_AnalyzeEndpoint_AuthenticationFailure(t *testing.T) {
	tempDir := t.TempDir()
	configPath, cleanup := createTestConfig(t, tempDir)
	defer cleanup()
	
	server, err := NewServer(configPath)
	require.NoError(t, err)
	
	req := httptest.NewRequest(http.MethodPost, "/analyze", bytes.NewReader([]byte("test")))
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("X-Client-ID", "test-client")
	req.Header.Set("X-Signature", "invalid-signature")
	
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	
	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	
	assert.Contains(t, result["error"], "authentication failed")
}

func TestServer_AnalyzeEndpoint_MethodNotAllowed(t *testing.T) {
	tempDir := t.TempDir()
	configPath, cleanup := createTestConfig(t, tempDir)
	defer cleanup()
	
	server, err := NewServer(configPath)
	require.NoError(t, err)
	
	req := httptest.NewRequest(http.MethodGet, "/analyze", nil)
	
	resp, err := server.app.Test(req)
	require.NoError(t, err)
	
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestServer_StartAndShutdown(t *testing.T) {
	tempDir := t.TempDir()
	configPath, cleanup := createTestConfig(t, tempDir)
	defer cleanup()
	
	server, err := NewServer(configPath)
	require.NoError(t, err)
	
	// Test graceful shutdown
	go func() {
		time.Sleep(100 * time.Millisecond)
		err := server.Shutdown()
		assert.NoError(t, err)
	}()
	
	// This should start and then shutdown gracefully
	err = server.Start()
	// Start may return an error when shutdown, which is expected
	// The important thing is that shutdown works without hanging
}

// Helper functions

func createTestConfig(t *testing.T, tempDir string) (string, func()) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	publicKeyPath := savePublicKey(t, tempDir, &privateKey.PublicKey)
	
	configPath := filepath.Join(tempDir, "test-config.yaml")
	configContent := `
service:
  name: "test-service"
  port: 8080

executable:
  path: "echo"
  args: ["test output"]
  timeout: "5m"

security:
  auth_method: "public_key"
  public_key_path: "` + publicKeyPath + `"
  run_as_user: "testuser"
  max_memory: "512MB"
  max_cpu: "1.0"

input:
  formats: ["tar.gz", "tar", "zip"]
  allowed_extensions: [".py", ".pyw"]
  max_archive_size: "100MB"

output:
  format: "json"
  parser: "pylint_json"
`
	
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)
	
	return configPath, func() {
		os.RemoveAll(tempDir)
	}
}

func savePublicKey(t *testing.T, tempDir string, publicKey *rsa.PublicKey) string {
	publicKeyPath := filepath.Join(tempDir, "public.pem")
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(publicKey),
	})
	
	err := os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)
	
	return publicKeyPath
}

func createTestArchiveData(t *testing.T) []byte {
	// Create a simple test archive (for now just return dummy data)
	// In a real implementation, this would create a proper tar.gz archive
	return []byte("dummy-archive-data")
}

func createTestSignatureForData(t *testing.T, data []byte) (*rsa.PrivateKey, string) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	// Create signature using same method as client
	hash := sha256.Sum256(data)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	require.NoError(t, err)
	
	signatureB64 := base64.StdEncoding.EncodeToString(signature)
	return privateKey, signatureB64
}