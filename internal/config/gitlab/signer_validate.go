// This file contains token validation helpers for the GitLab signer.
package gitlab

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ValidateToken verifies the bearer token and returns its claims.
func (s *Signer) ValidateToken(ctx context.Context, token string) (TokenClaims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return TokenClaims{}, errors.New("gitlab signer: token required")
	}
	if !strings.HasPrefix(token, "gls_") {
		return TokenClaims{}, errors.New("gitlab signer: malformed token")
	}
	rawEnvelope, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "gls_"))
	if err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode token envelope: %w", err)
	}

	var envelope tokenEnvelope
	if err := json.Unmarshal(rawEnvelope, &envelope); err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode token payload: %w", err)
	}
	if envelope.Payload == "" || envelope.Signature == "" {
		return TokenClaims{}, errors.New("gitlab signer: token envelope incomplete")
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(envelope.Payload)
	if err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode payload body: %w", err)
	}
	var payload tokenPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode payload: %w", err)
	}
	if payload.IssuedAt == 0 || payload.ExpiresAt == 0 {
		return TokenClaims{}, errors.New("gitlab signer: token missing timestamps")
	}
	issuedAt := time.Unix(payload.IssuedAt, 0).UTC()
	expiresAt := time.Unix(payload.ExpiresAt, 0).UTC()
	if time.Now().UTC().After(expiresAt) {
		return TokenClaims{}, errors.New("gitlab signer: token expired")
	}

	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode signature: %w", err)
	}

	record, err := s.findTokenSecret(ctx, payload.TokenID, payloadJSON, signature)
	if err != nil {
		return TokenClaims{}, err
	}

	scopes := normalizeScopes(payload.Scopes)
	if err := ensureScopesAllowed(record.Scopes, scopes); err != nil {
		return TokenClaims{}, err
	}

	return TokenClaims{
		SecretName: record.SecretName,
		TokenID:    strings.TrimSpace(payload.TokenID),
		Scopes:     scopes,
		IssuedAt:   issuedAt,
		ExpiresAt:  expiresAt,
	}, nil
}

// findTokenSecret locates the secret record associated with a token.
func (s *Signer) findTokenSecret(ctx context.Context, tokenID string, payloadJSON, signature []byte) (secretRecord, error) {
	s.issuedMu.Lock()
	var cachedSecret string
	for secret, tokens := range s.issued {
		if _, ok := tokens[tokenID]; ok {
			cachedSecret = secret
			break
		}
	}
	s.issuedMu.Unlock()

	if cachedSecret != "" {
		record, err := s.loadSecret(ctx, cachedSecret)
		if err == nil {
			if ok, verifyErr := s.secretMatches(ctx, record, payloadJSON, signature); verifyErr == nil && ok {
				return record, nil
			}
		}
	}

	secrets, err := s.listSecrets(ctx)
	if err != nil {
		return secretRecord{}, err
	}
	for _, record := range secrets {
		match, matchErr := s.secretMatches(ctx, record, payloadJSON, signature)
		if matchErr != nil {
			continue
		}
		if match {
			return record, nil
		}
	}
	return secretRecord{}, errors.New("gitlab signer: token not recognized")
}

// secretMatches verifies the signature using the provided secret record.
func (s *Signer) secretMatches(ctx context.Context, record secretRecord, payloadJSON, signature []byte) (bool, error) {
	apiKey, err := record.decrypt(ctx, s.cipher)
	if err != nil {
		return false, err
	}
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write(payloadJSON)
	if !hmac.Equal(mac.Sum(nil), signature) {
		return false, nil
	}
	return true, nil
}

// listSecrets returns every signer secret stored in etcd.
func (s *Signer) listSecrets(ctx context.Context) ([]secretRecord, error) {
	resp, err := s.client.Get(ctx, s.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("gitlab signer: list secrets: %w", err)
	}
	records := make([]secretRecord, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		record, err := decodeSecretRecord(kv, s.prefix)
		if err != nil {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

// loadSecret fetches a single secret record by name.
func (s *Signer) loadSecret(ctx context.Context, name string) (secretRecord, error) {
	resp, err := s.client.Get(ctx, s.prefix+name, clientv3.WithLimit(1))
	if err != nil {
		return secretRecord{}, fmt.Errorf("gitlab signer: fetch secret: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return secretRecord{}, fmt.Errorf("gitlab signer: secret %q not found", name)
	}
	record, err := decodeSecretRecord(resp.Kvs[0], s.prefix)
	if err != nil {
		return secretRecord{}, err
	}
	return record, nil
}
