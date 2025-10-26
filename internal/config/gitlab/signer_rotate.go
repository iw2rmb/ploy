// This file contains secret rotation helpers for the GitLab signer.
package gitlab

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RotateSecret stores the provided API key encrypted in etcd.
func (s *Signer) RotateSecret(ctx context.Context, req RotateSecretRequest) (RotateSecretResult, error) {
	name := strings.TrimSpace(req.SecretName)
	if name == "" {
		return RotateSecretResult{}, errEmptySecretName
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		return RotateSecretResult{}, errEmptyAPIKey
	}
	scopes := normalizeScopes(req.Scopes)
	if len(scopes) == 0 {
		return RotateSecretResult{}, errors.New("gitlab signer: at least one scope required")
	}

	encodedScopes, err := json.Marshal(scopes)
	if err != nil {
		return RotateSecretResult{}, fmt.Errorf("gitlab signer: encode scopes: %w", err)
	}

	ciphertext, err := s.cipher.Encrypt(ctx, []byte(key))
	if err != nil {
		return RotateSecretResult{}, fmt.Errorf("gitlab signer: encrypt api key: %w", err)
	}

	now := s.now().UTC()
	record := secretRecord{
		SecretName: name,
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		Scopes:     scopes,
		ScopeJSON:  string(encodedScopes),
		UpdatedAt:  now.Format(time.RFC3339Nano),
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return RotateSecretResult{}, fmt.Errorf("gitlab signer: marshal secret record: %w", err)
	}

	resp, err := s.client.Put(ctx, s.prefix+name, string(payload))
	if err != nil {
		return RotateSecretResult{}, fmt.Errorf("gitlab signer: put secret: %w", err)
	}
	var revision int64
	if resp != nil && resp.Header != nil {
		revision = resp.Header.Revision
	}
	result := RotateSecretResult{
		Revision:  revision,
		UpdatedAt: now,
	}
	s.handleRotation(ctx, name)
	return result, nil
}

// handleRotation revokes and requeues tokens after a secret is updated.
func (s *Signer) handleRotation(ctx context.Context, secret string) {
	tokens := s.popIssuedTokens(secret)
	if len(tokens) == 0 {
		return
	}
	revoker := s.revoker
	if revoker == nil {
		return
	}

	revocable := make([]RevocableToken, 0, len(tokens))
	lookup := make(map[string]issuedToken, len(tokens))
	for _, tok := range tokens {
		revocable = append(revocable, RevocableToken{ID: tok.tokenID, NodeID: tok.nodeID})
		lookup[tok.tokenID] = tok
	}

	report := revoker.Revoke(ctx, secret, revocable)
	now := s.now().UTC()

	for _, revoked := range report.Revoked {
		s.audit.Record(AuditEvent{
			Action:     AuditActionRevoked,
			SecretName: secret,
			NodeID:     revoked.NodeID,
			TokenID:    revoked.ID,
			Timestamp:  now,
		})
		delete(lookup, revoked.ID)
	}

	if len(report.Failed) == 0 {
		return
	}

	var retry []issuedToken
	for _, failure := range report.Failed {
		orig, ok := lookup[failure.Token.ID]
		if ok {
			retry = append(retry, orig)
		}
		errMsg := ""
		if failure.Err != nil {
			errMsg = failure.Err.Error()
		}
		s.audit.Record(AuditEvent{
			Action:     AuditActionRevocationFailed,
			SecretName: secret,
			NodeID:     failure.Token.NodeID,
			TokenID:    failure.Token.ID,
			Timestamp:  now,
			Error:      errMsg,
		})
	}
	if len(retry) > 0 {
		s.requeueTokens(secret, retry)
	}
}
