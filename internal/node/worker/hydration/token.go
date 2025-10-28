package hydration

import (
	"context"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Token represents a short-lived credential used for repository access.
type Token struct {
	Value     string
	ExpiresAt time.Time
}

// Valid reports whether the token remains usable for the provided time instant.
func (t Token) Valid(now time.Time) bool {
	if strings.TrimSpace(t.Value) == "" {
		return false
	}
	if t.ExpiresAt.IsZero() {
		return true
	}
	// Refresh one minute before expiry to avoid race conditions.
	return now.Add(time.Minute).Before(t.ExpiresAt)
}

// TokenSource issues short-lived tokens for repository access.
type TokenSource interface {
	IssueToken(ctx context.Context, repo contracts.RepoMaterialization) (Token, error)
}
