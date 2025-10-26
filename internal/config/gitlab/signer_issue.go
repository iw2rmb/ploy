// This file focuses on GitLab signer token issuance and cache helpers.
package gitlab

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// IssueToken returns a short-lived token scoped to the requested permissions.
func (s *Signer) IssueToken(ctx context.Context, req IssueTokenRequest) (SignedToken, error) {
	name := strings.TrimSpace(req.SecretName)
	if name == "" {
		return SignedToken{}, errEmptySecretName
	}
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		return SignedToken{}, errors.New("gitlab signer: node_id required for issuance")
	}

	record, err := s.loadSecret(ctx, name)
	if err != nil {
		return SignedToken{}, err
	}

	requestedScopes := normalizeScopes(req.Scopes)
	if len(requestedScopes) == 0 {
		requestedScopes = record.Scopes
	}
	if err := ensureScopesAllowed(record.Scopes, requestedScopes); err != nil {
		return SignedToken{}, err
	}

	ttl := req.TTL
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	if ttl > s.maxTTL {
		return SignedToken{}, fmt.Errorf("gitlab signer: requested ttl %s exceeds max %s", ttl, s.maxTTL)
	}

	issuedAt := s.now().UTC()
	expiresAt := issuedAt.Add(ttl)

	apiKey, err := record.decrypt(ctx, s.cipher)
	if err != nil {
		return SignedToken{}, err
	}

	tokenID, err := generateTokenID()
	if err != nil {
		return SignedToken{}, err
	}

	value, err := mintToken(apiKey, requestedScopes, issuedAt, expiresAt, tokenID)
	if err != nil {
		return SignedToken{}, err
	}

	signed := SignedToken{
		SecretName: name,
		Value:      value,
		Scopes:     requestedScopes,
		IssuedAt:   issuedAt,
		ExpiresAt:  expiresAt,
		TokenID:    tokenID,
	}
	s.recordIssuedToken(name, nodeID, signed)

	return signed, nil
}

// recordIssuedToken caches issued token metadata for auditing and revocation.
func (s *Signer) recordIssuedToken(secret, nodeID string, token SignedToken) {
	evt := AuditEvent{
		Action:     AuditActionIssued,
		SecretName: secret,
		NodeID:     nodeID,
		TokenID:    token.TokenID,
		Timestamp:  token.IssuedAt,
		ExpiresAt:  token.ExpiresAt,
	}

	s.issuedMu.Lock()
	bySecret := s.ensureIssuedLocked(secret)
	bySecret[token.TokenID] = issuedToken{
		tokenID:   token.TokenID,
		nodeID:    nodeID,
		issuedAt:  token.IssuedAt,
		expiresAt: token.ExpiresAt,
	}
	s.issuedMu.Unlock()

	s.audit.Record(evt)
}

// ensureIssuedLocked lazily initialises the issued-token cache for a secret.
func (s *Signer) ensureIssuedLocked(secret string) map[string]issuedToken {
	if s.issued == nil {
		s.issued = make(map[string]map[string]issuedToken)
	}
	bySecret, ok := s.issued[secret]
	if !ok {
		bySecret = make(map[string]issuedToken)
		s.issued[secret] = bySecret
	}
	return bySecret
}

// popIssuedTokens drains cached tokens for a secret so they can be revoked.
func (s *Signer) popIssuedTokens(secret string) []issuedToken {
	s.issuedMu.Lock()
	defer s.issuedMu.Unlock()
	bySecret, ok := s.issued[secret]
	if !ok || len(bySecret) == 0 {
		return nil
	}
	result := make([]issuedToken, 0, len(bySecret))
	for _, tok := range bySecret {
		result = append(result, tok)
	}
	delete(s.issued, secret)
	return result
}

// requeueTokens stores tokens back in the cache when revocation fails.
func (s *Signer) requeueTokens(secret string, tokens []issuedToken) {
	s.issuedMu.Lock()
	defer s.issuedMu.Unlock()
	bySecret := s.ensureIssuedLocked(secret)
	for _, tok := range tokens {
		bySecret[tok.tokenID] = tok
	}
}
