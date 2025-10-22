// Package git contains Git helper utilities for node credential management.
package git

import (
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
)

// CredentialCache provides concurrency-safe storage for GitLab credentials.
type CredentialCache struct {
	mu     sync.RWMutex
	tokens map[string]gitlab.SignedToken
}

// NewCredentialCache constructs an empty credential cache.
func NewCredentialCache() *CredentialCache {
	return &CredentialCache{
		tokens: make(map[string]gitlab.SignedToken),
	}
}

// Set stores the token for subsequent Git operations.
func (c *CredentialCache) Set(token gitlab.SignedToken) {
	if c == nil {
		return
	}
	name := strings.TrimSpace(token.SecretName)
	if name == "" || strings.TrimSpace(token.Value) == "" {
		return
	}
	c.mu.Lock()
	c.tokens[name] = token
	c.mu.Unlock()
}

// Get returns the cached token for the provided secret.
func (c *CredentialCache) Get(secret string) (gitlab.SignedToken, bool) {
	if c == nil {
		return gitlab.SignedToken{}, false
	}
	c.mu.RLock()
	token, ok := c.tokens[strings.TrimSpace(secret)]
	c.mu.RUnlock()
	if !ok || strings.TrimSpace(token.Value) == "" {
		return gitlab.SignedToken{}, false
	}
	return token, true
}

// Flush removes the cached token for the provided secret.
func (c *CredentialCache) Flush(secret string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.tokens, strings.TrimSpace(secret))
	c.mu.Unlock()
}

// FlushAll removes all cached tokens.
func (c *CredentialCache) FlushAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	for key := range c.tokens {
		delete(c.tokens, key)
	}
	c.mu.Unlock()
}

// AuthorizationHeader returns a Git Authorization header for the cached token.
func (c *CredentialCache) AuthorizationHeader(secret string) (string, bool) {
	token, ok := c.Get(secret)
	if !ok {
		return "", false
	}
	return "Bearer " + strings.TrimSpace(token.Value), true
}
