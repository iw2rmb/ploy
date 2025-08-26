package auth

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/gofiber/fiber/v2"
)

// Manager handles authentication for CHTTP requests
type Manager struct {
	publicKey *rsa.PublicKey
}

// NewManager creates a new authentication manager with the given public key
func NewManager(publicKeyPath string) (*Manager, error) {
	// Read public key file
	keyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %w", err)
	}
	
	// Decode PEM block
	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	
	// Parse public key
	publicKey, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}
	
	return &Manager{
		publicKey: publicKey,
	}, nil
}

// ValidateRequest validates an incoming request's authentication
func (m *Manager) ValidateRequest(c *fiber.Ctx, requestData []byte) error {
	// Get client ID header
	clientID := c.Get("X-Client-ID")
	if clientID == "" {
		return fmt.Errorf("client ID header is required")
	}
	
	// Get signature header
	signature := c.Get("X-Signature")
	if signature == "" {
		return fmt.Errorf("signature header is required")
	}
	
	// Decode base64 signature
	signatureBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}
	
	// Verify signature
	hash := sha256.Sum256(requestData)
	err = rsa.VerifyPKCS1v15(m.publicKey, crypto.SHA256, hash[:], signatureBytes)
	if err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	
	// Store client ID in context for potential use by handlers
	c.Locals("client_id", clientID)
	
	return nil
}

// Middleware returns a Fiber middleware function that validates requests
func (m *Manager) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip authentication for health endpoint
		if c.Path() == "/health" {
			return c.Next()
		}
		
		// Read request body
		body := c.Body()
		
		// Validate authentication
		if err := m.ValidateRequest(c, body); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": fmt.Sprintf("authentication failed: %v", err),
			})
		}
		
		return c.Next()
	}
}