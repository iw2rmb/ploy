package gitlab

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type tokenEnvelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"sig"`
}

type tokenPayload struct {
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	Scopes    []string `json:"scp"`
	TokenID   string   `json:"tid"`
}

func generateTokenID() (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", fmt.Errorf("gitlab signer: generate token id: %w", err)
	}
	return hex.EncodeToString(id), nil
}

func mintToken(apiKey string, scopes []string, issuedAt, expiresAt time.Time, tokenID string) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", errors.New("gitlab signer: api key required for minting token")
	}
	payload := map[string]any{
		"iat": issuedAt.Unix(),
		"exp": expiresAt.Unix(),
		"scp": scopes,
	}
	if trimmed := strings.TrimSpace(tokenID); trimmed != "" {
		payload["tid"] = trimmed
	}

	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("gitlab signer: generate token nonce: %w", err)
	}
	payload["rnd"] = base64.RawURLEncoding.EncodeToString(nonce)

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("gitlab signer: encode token payload: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write(payloadJSON)
	signature := mac.Sum(nil)

	tokenEnvelope := map[string]string{
		"payload": base64.RawURLEncoding.EncodeToString(payloadJSON),
		"sig":     base64.RawURLEncoding.EncodeToString(signature),
	}

	envelopeJSON, err := json.Marshal(tokenEnvelope)
	if err != nil {
		return "", fmt.Errorf("gitlab signer: encode token envelope: %w", err)
	}
	return "gls_" + base64.RawURLEncoding.EncodeToString(envelopeJSON), nil
}
