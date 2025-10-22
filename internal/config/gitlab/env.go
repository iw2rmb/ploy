package gitlab

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	signerAESKeyEnv     = "PLOY_GITLAB_SIGNER_AES_KEY"
	signerDefaultTTLEnv = "PLOY_GITLAB_SIGNER_DEFAULT_TTL"
	signerMaxTTLEnv     = "PLOY_GITLAB_SIGNER_MAX_TTL"
)

// NewSignerFromEnv constructs a Signer using environment variables for keys and TTL overrides.
func NewSignerFromEnv(client *clientv3.Client) (*Signer, error) {
	keyData := strings.TrimSpace(os.Getenv(signerAESKeyEnv))
	if keyData == "" {
		return nil, fmt.Errorf("gitlab signer: %s is required", signerAESKeyEnv)
	}
	rawKey, err := base64.StdEncoding.DecodeString(keyData)
	if err != nil {
		return nil, fmt.Errorf("gitlab signer: decode %s: %w", signerAESKeyEnv, err)
	}
	cipher, err := NewAESCipher(rawKey)
	if err != nil {
		return nil, err
	}

	opts, err := signerOptionsFromEnv()
	if err != nil {
		return nil, err
	}
	return NewSigner(client, cipher, opts...)
}

// signerOptionsFromEnv builds Signer options from the TTL-related environment variables.
func signerOptionsFromEnv() ([]SignerOption, error) {
	var opts []SignerOption

	if raw := strings.TrimSpace(os.Getenv(signerDefaultTTLEnv)); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("gitlab signer: parse %s: %w", signerDefaultTTLEnv, err)
		}
		opts = append(opts, WithDefaultTTL(ttl))
	}
	if raw := strings.TrimSpace(os.Getenv(signerMaxTTLEnv)); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("gitlab signer: parse %s: %w", signerMaxTTLEnv, err)
		}
		opts = append(opts, WithMaxTTL(ttl))
	}
	return opts, nil
}
