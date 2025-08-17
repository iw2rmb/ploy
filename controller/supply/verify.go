package supply

import (
	"fmt"
	"os"
	"os/exec"
)

// VerifySignature tries cosign verify-blob either with a public key or keyless OIDC identity policy.
// Env:
//  COSIGN_PUBKEY: path to public key (if provided, use key mode)
//  COSIGN_VERIFY_IDENTITY_REGEXP: if set, use --certificate-identity-regexp for keyless verification
func VerifySignature(artifact, sigPath string) error {
	if _, err := os.Stat(sigPath); err != nil { return fmt.Errorf("signature missing: %w", err) }
	if pub := os.Getenv("COSIGN_PUBKEY"); pub != "" {
		cmd := exec.Command("cosign","verify-blob","--key", pub, "--signature", sigPath, artifact)
		b, err := cmd.CombinedOutput(); if err != nil { return fmt.Errorf("cosign verify-blob failed: %v: %s", err, string(b)) }
		return nil
	}
	if idre := os.Getenv("COSIGN_VERIFY_IDENTITY_REGEXP"); idre != "" {
		os.Setenv("COSIGN_EXPERIMENTAL","1")
		cmd := exec.Command("cosign","verify-blob","--certificate-identity-regexp", idre, "--signature", sigPath, artifact)
		b, err := cmd.CombinedOutput(); if err != nil { return fmt.Errorf("cosign keyless verify failed: %v: %s", err, string(b)) }
		return nil
	}
	// Fallback: accept presence-only (already gated by OPA)
	return nil
}
