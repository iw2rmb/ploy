package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	tempDir := t.TempDir()
	publicKeyPath := filepath.Join(tempDir, "public.pem")
	
	// Generate test keys
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	// Save public key
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)
	
	manager, err := NewManager(publicKeyPath)
	require.NoError(t, err)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.publicKey)
}

func TestNewManager_InvalidPath(t *testing.T) {
	_, err := NewManager("/nonexistent/public.pem")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read public key file")
}

func TestNewManager_InvalidPEM(t *testing.T) {
	tempDir := t.TempDir()
	publicKeyPath := filepath.Join(tempDir, "invalid.pem")
	
	err := os.WriteFile(publicKeyPath, []byte("invalid pem content"), 0644)
	require.NoError(t, err)
	
	_, err = NewManager(publicKeyPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode PEM block")
}

func TestManager_ValidateRequest(t *testing.T) {
	// Generate test keys
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	tempDir := t.TempDir()
	publicKeyPath := filepath.Join(tempDir, "public.pem")
	
	// Save public key
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)
	
	manager, err := NewManager(publicKeyPath)
	require.NoError(t, err)
	
	// Create test data and signature using the same method as client
	testData := []byte("test data for signing")
	signature, err := createTestSignature(privateKey, testData)
	require.NoError(t, err)
	
	// Create Fiber context with proper headers
	app := fiber.New()
	req := httptest.NewRequest("POST", "/analyze", nil)
	req.Header.Set("X-Client-ID", "test-client")
	req.Header.Set("X-Signature", signature)
	
	// Test using proper Fiber test method
	resp, err := app.Test(req)
	require.NoError(t, err)
	
	// Create context for direct manager testing
	c := app.AcquireCtx(nil)
	defer app.ReleaseCtx(c)
	
	// Set headers on context manually for manager testing
	c.Request().Header.Set("X-Client-ID", "test-client")
	c.Request().Header.Set("X-Signature", signature)
	
	// Test valid signature
	err = manager.ValidateRequest(c, testData)
	assert.NoError(t, err)
}

func TestManager_ValidateRequest_MissingClientID(t *testing.T) {
	tempDir := t.TempDir()
	publicKeyPath := filepath.Join(tempDir, "public.pem")
	
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)
	
	manager, err := NewManager(publicKeyPath)
	require.NoError(t, err)
	
	app := fiber.New()
	c := app.AcquireCtx(nil)
	defer app.ReleaseCtx(c)
	
	// Set headers - missing X-Client-ID
	c.Request().Header.Set("X-Signature", "some-signature")
	
	err = manager.ValidateRequest(c, []byte("test data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client ID header is required")
}

func TestManager_ValidateRequest_MissingSignature(t *testing.T) {
	tempDir := t.TempDir()
	publicKeyPath := filepath.Join(tempDir, "public.pem")
	
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)
	
	manager, err := NewManager(publicKeyPath)
	require.NoError(t, err)
	
	app := fiber.New()
	c := app.AcquireCtx(nil)
	defer app.ReleaseCtx(c)
	
	// Set headers - missing X-Signature
	c.Request().Header.Set("X-Client-ID", "test-client")
	
	err = manager.ValidateRequest(c, []byte("test data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature header is required")
}

func TestManager_ValidateRequest_InvalidSignature(t *testing.T) {
	tempDir := t.TempDir()
	publicKeyPath := filepath.Join(tempDir, "public.pem")
	
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)
	
	manager, err := NewManager(publicKeyPath)
	require.NoError(t, err)
	
	app := fiber.New()
	c := app.AcquireCtx(nil)
	defer app.ReleaseCtx(c)
	
	// Set headers with invalid signature
	c.Request().Header.Set("X-Client-ID", "test-client")
	c.Request().Header.Set("X-Signature", "invalid-signature")
	
	err = manager.ValidateRequest(c, []byte("test data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode signature")
}

func TestManager_ValidateRequest_WrongSignature(t *testing.T) {
	tempDir := t.TempDir()
	publicKeyPath := filepath.Join(tempDir, "public.pem")
	
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)
	
	manager, err := NewManager(publicKeyPath)
	require.NoError(t, err)
	
	// Create signature for different data
	wrongData := []byte("wrong data")
	signature, err := createTestSignature(privateKey, wrongData)
	require.NoError(t, err)
	
	app := fiber.New()
	c := app.AcquireCtx(nil)
	defer app.ReleaseCtx(c)
	
	// Set headers with wrong signature
	c.Request().Header.Set("X-Client-ID", "test-client")
	c.Request().Header.Set("X-Signature", signature)
	
	// Try to validate with different data
	err = manager.ValidateRequest(c, []byte("correct data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature verification failed")
}

func TestManager_Middleware(t *testing.T) {
	tempDir := t.TempDir()
	publicKeyPath := filepath.Join(tempDir, "public.pem")
	
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})
	err = os.WriteFile(publicKeyPath, publicKeyPEM, 0644)
	require.NoError(t, err)
	
	manager, err := NewManager(publicKeyPath)
	require.NoError(t, err)
	
	app := fiber.New()
	
	// Add authentication middleware
	app.Use(manager.Middleware())
	
	// Add test endpoint
	app.Post("/analyze", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "success"})
	})
	
	testData := []byte("test request data")
	signature, err := createTestSignature(privateKey, testData)
	require.NoError(t, err)
	
	// Test valid request
	req := httptest.NewRequest(http.MethodPost, "/analyze", nil)
	req.Header.Set("X-Client-ID", "test-client")
	req.Header.Set("X-Signature", signature)
	
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// Helper function to create test signatures using the same method as the client
func createTestSignature(privateKey *rsa.PrivateKey, data []byte) (string, error) {
	// This mirrors the signRequest method in internal/chttp/client.go
	hash := sha256.Sum256(data)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}