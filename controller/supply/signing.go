package supply

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SigningMode defines different signing approaches
type SigningMode string

const (
	// KeylessOIDC uses Sigstore keyless OIDC flow for production
	KeylessOIDC SigningMode = "keyless-oidc"
	// KeyBased uses local/KMS keys for signing
	KeyBased SigningMode = "key-based"
	// Development uses dummy signatures for development environments
	Development SigningMode = "development"
)

// SigningOptions configures the signing process
type SigningOptions struct {
	Mode          SigningMode
	KeyPath       string
	OIDCProvider  string
	OIDCClientID  string
	Environment   string
	TlogUpload    bool
	Timeout       time.Duration
}

// SigningResult contains information about the signing operation
type SigningResult struct {
	SignatureFile string
	Certificate   string
	TlogEntry     string
	Mode          SigningMode
	Timestamp     time.Time
}

// Signer provides cryptographic signing capabilities for artifacts
type Signer struct {
	options SigningOptions
}

// NewSigner creates a new signer with the specified options
func NewSigner(opts SigningOptions) *Signer {
	// Set defaults
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}
	if opts.OIDCClientID == "" {
		opts.OIDCClientID = "sigstore"
	}
	if opts.Mode == "" {
		opts.Mode = DetermineSigningMode(opts.Environment)
	}

	return &Signer{options: opts}
}

// DetermineSigningMode automatically determines the appropriate signing mode
func DetermineSigningMode(environment string) SigningMode {
	// Check for CI/CD environments that support OIDC
	if os.Getenv("GITHUB_ACTIONS") == "true" ||
		os.Getenv("GITLAB_CI") == "true" ||
		os.Getenv("BUILDKITE") == "true" ||
		environment == "production" ||
		environment == "staging" {
		return KeylessOIDC
	}

	// Check if cosign is available for development signing
	if _, err := exec.LookPath("cosign"); err == nil {
		return KeylessOIDC
	}

	return Development
}

// SignBlob signs a file artifact using cosign
func (s *Signer) SignBlob(ctx context.Context, filePath string) (*SigningResult, error) {
	if !fileExists(filePath) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	signatureFile := filePath + ".sig"
	certificateFile := filePath + ".crt"

	switch s.options.Mode {
	case KeylessOIDC:
		return s.signBlobKeyless(ctx, filePath, signatureFile, certificateFile)
	case KeyBased:
		return s.signBlobWithKey(ctx, filePath, signatureFile)
	case Development:
		return s.signBlobDevelopment(filePath, signatureFile)
	default:
		return nil, fmt.Errorf("unsupported signing mode: %s", s.options.Mode)
	}
}

// SignContainer signs a container image using cosign
func (s *Signer) SignContainer(ctx context.Context, imageRef string) (*SigningResult, error) {
	switch s.options.Mode {
	case KeylessOIDC:
		return s.signContainerKeyless(ctx, imageRef)
	case KeyBased:
		return s.signContainerWithKey(ctx, imageRef)
	case Development:
		return s.signContainerDevelopment(imageRef)
	default:
		return nil, fmt.Errorf("unsupported signing mode: %s", s.options.Mode)
	}
}

// signBlobKeyless implements keyless OIDC signing for file artifacts
func (s *Signer) signBlobKeyless(ctx context.Context, filePath, signatureFile, certificateFile string) (*SigningResult, error) {
	args := []string{
		"sign-blob",
		"--yes", // Skip confirmation prompts
		"--output-signature", signatureFile,
		"--output-certificate", certificateFile,
	}

	// Add OIDC configuration if specified
	if s.options.OIDCProvider != "" {
		args = append(args, "--oidc-provider", s.options.OIDCProvider)
	}
	if s.options.OIDCClientID != "" {
		args = append(args, "--oidc-client-id", s.options.OIDCClientID)
	}

	// Control transparency log upload
	args = append(args, fmt.Sprintf("--tlog-upload=%t", s.options.TlogUpload))

	args = append(args, filePath)

	cmd := exec.CommandContext(ctx, "cosign", args...)
	cmd.Env = append(os.Environ(),
		"COSIGN_EXPERIMENTAL=1", // Enable experimental features including keyless
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cosign sign-blob failed: %w\nOutput: %s", err, string(output))
	}

	result := &SigningResult{
		SignatureFile: signatureFile,
		Certificate:   certificateFile,
		Mode:          KeylessOIDC,
		Timestamp:     time.Now(),
	}

	// Extract transparency log entry from output if available
	if strings.Contains(string(output), "tlog entry created with index") {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "tlog entry created with index") {
				result.TlogEntry = strings.TrimSpace(line)
				break
			}
		}
	}

	return result, nil
}

// signContainerKeyless implements keyless OIDC signing for container images
func (s *Signer) signContainerKeyless(ctx context.Context, imageRef string) (*SigningResult, error) {
	args := []string{
		"sign",
		"--yes", // Skip confirmation prompts
	}

	// Add OIDC configuration if specified
	if s.options.OIDCProvider != "" {
		args = append(args, "--oidc-provider", s.options.OIDCProvider)
	}
	if s.options.OIDCClientID != "" {
		args = append(args, "--oidc-client-id", s.options.OIDCClientID)
	}

	// Control transparency log upload
	args = append(args, fmt.Sprintf("--tlog-upload=%t", s.options.TlogUpload))

	args = append(args, imageRef)

	cmd := exec.CommandContext(ctx, "cosign", args...)
	cmd.Env = append(os.Environ(),
		"COSIGN_EXPERIMENTAL=1", // Enable experimental features including keyless
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cosign sign failed: %w\nOutput: %s", err, string(output))
	}

	result := &SigningResult{
		Mode:      KeylessOIDC,
		Timestamp: time.Now(),
	}

	// Extract transparency log entry from output if available
	if strings.Contains(string(output), "tlog entry created with index") {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "tlog entry created with index") {
				result.TlogEntry = strings.TrimSpace(line)
				break
			}
		}
	}

	return result, nil
}

// signBlobWithKey implements key-based signing for file artifacts
func (s *Signer) signBlobWithKey(ctx context.Context, filePath, signatureFile string) (*SigningResult, error) {
	args := []string{
		"sign-blob",
		"--key", s.options.KeyPath,
		"--output-signature", signatureFile,
		filePath,
	}

	cmd := exec.CommandContext(ctx, "cosign", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cosign sign-blob with key failed: %w\nOutput: %s", err, string(output))
	}

	return &SigningResult{
		SignatureFile: signatureFile,
		Mode:          KeyBased,
		Timestamp:     time.Now(),
	}, nil
}

// signContainerWithKey implements key-based signing for container images
func (s *Signer) signContainerWithKey(ctx context.Context, imageRef string) (*SigningResult, error) {
	args := []string{
		"sign",
		"--key", s.options.KeyPath,
		imageRef,
	}

	cmd := exec.CommandContext(ctx, "cosign", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cosign sign with key failed: %w\nOutput: %s", err, string(output))
	}

	return &SigningResult{
		Mode:      KeyBased,
		Timestamp: time.Now(),
	}, nil
}

// signBlobDevelopment creates a dummy signature for development environments
func (s *Signer) signBlobDevelopment(filePath, signatureFile string) (*SigningResult, error) {
	dummySignature := fmt.Sprintf("dev-signature-%d", time.Now().Unix())
	
	if err := os.WriteFile(signatureFile, []byte(dummySignature), 0644); err != nil {
		return nil, fmt.Errorf("failed to create development signature: %w", err)
	}

	return &SigningResult{
		SignatureFile: signatureFile,
		Mode:          Development,
		Timestamp:     time.Now(),
	}, nil
}

// signContainerDevelopment creates a dummy signature for container images in development
func (s *Signer) signContainerDevelopment(imageRef string) (*SigningResult, error) {
	// In development mode, we just log that we would sign the container
	// No actual signature is created for containers in dev mode
	return &SigningResult{
		Mode:      Development,
		Timestamp: time.Now(),
	}, nil
}

// IsAvailable checks if cosign is available and functional
func IsAvailable() bool {
	_, err := exec.LookPath("cosign")
	return err == nil
}

// GetVersion returns the cosign version if available
func GetVersion() (string, error) {
	if !IsAvailable() {
		return "", fmt.Errorf("cosign not available")
	}

	cmd := exec.Command("cosign", "version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get cosign version: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}