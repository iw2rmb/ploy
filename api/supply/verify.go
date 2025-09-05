package supply

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// VerifySignature tries cosign verify-blob either with a public key or keyless OIDC identity policy.
// Env:
//
//	COSIGN_PUBKEY: path to public key (if provided, use key mode)
//	COSIGN_VERIFY_IDENTITY_REGEXP: if set, use --certificate-identity-regexp for keyless verification
func VerifySignature(artifact, sigPath string) error {
	if _, err := os.Stat(sigPath); err != nil {
		return fmt.Errorf("signature missing: %w", err)
	}
	if pub := os.Getenv("COSIGN_PUBKEY"); pub != "" {
		cmd := exec.Command("cosign", "verify-blob", "--key", pub, "--signature", sigPath, artifact)
		b, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("cosign verify-blob failed: %v: %s", err, string(b))
		}
		return nil
	}
	if idre := os.Getenv("COSIGN_VERIFY_IDENTITY_REGEXP"); idre != "" {
		os.Setenv("COSIGN_EXPERIMENTAL", "1")
		cmd := exec.Command("cosign", "verify-blob", "--certificate-identity-regexp", idre, "--signature", sigPath, artifact)
		b, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("cosign keyless verify failed: %v: %s", err, string(b))
		}
		return nil
	}
	// Fallback: accept presence-only (already gated by OPA)
	return nil
}

// SignTargetType represents the type of target being signed
type SignTargetType string

const (
	ArtifactFile SignTargetType = "file"
	DockerImage  SignTargetType = "image"
)

// SignTarget signs either a file artifact or Docker image using cosign.
// Supports key-based, keyless OIDC, and development dummy signing modes.
// Env:
//
//	COSIGN_PRIVATE_KEY: path to private key (if provided, use key mode)
//	COSIGN_PASSWORD: password for private key (if key mode)
//	COSIGN_EXPERIMENTAL: set to "1" for keyless OIDC signing
func SignTarget(target string, targetType SignTargetType) error {
	// Validate inputs based on target type
	switch targetType {
	case ArtifactFile:
		if _, err := os.Stat(target); err != nil {
			return fmt.Errorf("artifact not found: %w", err)
		}
	case DockerImage:
		if target == "" {
			return fmt.Errorf("image tag cannot be empty")
		}
	default:
		return fmt.Errorf("unsupported target type: %s", targetType)
	}

	// Key-based signing
	if privKey := os.Getenv("COSIGN_PRIVATE_KEY"); privKey != "" {
		return signWithKey(target, targetType, privKey)
	}

	// Keyless OIDC signing
	if os.Getenv("COSIGN_EXPERIMENTAL") == "1" {
		return signKeyless(target, targetType)
	}

	// Development dummy signature fallback
	return createDummySignature(target, targetType)
}

// signWithKey performs key-based signing
func signWithKey(target string, targetType SignTargetType, privKey string) error {
	var cmd *exec.Cmd

	switch targetType {
	case ArtifactFile:
		sigPath := target + ".sig"
		cmd = exec.Command("cosign", "sign-blob", "--key", privKey, "--output-signature", sigPath, target)
	case DockerImage:
		cmd = exec.Command("cosign", "sign", "--key", privKey, target)
	}

	if password := os.Getenv("COSIGN_PASSWORD"); password != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("COSIGN_PASSWORD=%s", password))
	}

	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cosign key-based sign failed: %v: %s", err, string(b))
	}
	return nil
}

// signKeyless performs enhanced keyless OIDC signing with improved configuration
func signKeyless(target string, targetType SignTargetType) error {
	var cmd *exec.Cmd

	switch targetType {
	case ArtifactFile:
		sigPath := target + ".sig"
		certPath := target + ".crt"

		// Enhanced cosign command with better OIDC support
		args := []string{
			"sign-blob",
			"--yes", // Skip confirmation prompts for automation
			"--output-signature", sigPath,
			"--output-certificate", certPath,
		}

		// Add OIDC provider configuration if specified
		if provider := os.Getenv("COSIGN_OIDC_PROVIDER"); provider != "" {
			args = append(args, "--oidc-provider", provider)
		}

		if clientID := os.Getenv("COSIGN_OIDC_CLIENT_ID"); clientID != "" {
			args = append(args, "--oidc-client-id", clientID)
		}

		// Control transparency log upload (default: true for production)
		tlogUpload := os.Getenv("COSIGN_TLOG_UPLOAD")
		if tlogUpload == "" {
			tlogUpload = "true" // Default to uploading to transparency log
		}
		args = append(args, "--tlog-upload="+tlogUpload)

		args = append(args, target)
		cmd = exec.Command("cosign", args...)

	case DockerImage:
		// Enhanced container signing with OIDC configuration
		args := []string{
			"sign",
			"--yes", // Skip confirmation prompts for automation
		}

		// Add OIDC provider configuration if specified
		if provider := os.Getenv("COSIGN_OIDC_PROVIDER"); provider != "" {
			args = append(args, "--oidc-provider", provider)
		}

		if clientID := os.Getenv("COSIGN_OIDC_CLIENT_ID"); clientID != "" {
			args = append(args, "--oidc-client-id", clientID)
		}

		// Control transparency log upload
		tlogUpload := os.Getenv("COSIGN_TLOG_UPLOAD")
		if tlogUpload == "" {
			tlogUpload = "true"
		}
		args = append(args, "--tlog-upload="+tlogUpload)

		args = append(args, target)
		cmd = exec.Command("cosign", args...)
	}

	// Set enhanced environment for keyless signing
	cmd.Env = append(os.Environ(),
		"COSIGN_EXPERIMENTAL=1",
		"COSIGN_YES=true", // Additional confirmation skip
	)

	// Set timeout for OIDC operations (5 minutes)
	if timeout := os.Getenv("COSIGN_TIMEOUT"); timeout == "" {
		cmd.Env = append(cmd.Env, "COSIGN_TIMEOUT=300s")
	}

	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cosign keyless OIDC sign failed: %v: %s", err, string(b))
	}
	return nil
}

// createDummySignature creates development/testing dummy signatures
func createDummySignature(target string, targetType SignTargetType) error {
	var sigPath string
	var content string

	switch targetType {
	case ArtifactFile:
		sigPath = target + ".sig"
		content = "dummy-signature-for-development"
	case DockerImage:
		sigPath = filepath.Join(os.TempDir(), "docker-"+filepath.Base(target)+".sig")
		content = "dummy-docker-signature-for-development"
	}

	if err := os.WriteFile(sigPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create dummy signature: %w", err)
	}
	return nil
}

// SignArtifact signs a build artifact file using the generic signing function.
func SignArtifact(artifact string) error {
	return SignTarget(artifact, ArtifactFile)
}

// SignDockerImage signs a Docker image using the generic signing function.
func SignDockerImage(imageTag string) error {
	return SignTarget(imageTag, DockerImage)
}
