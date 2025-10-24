package security

import (
	"context"
	"errors"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
)

// SignerVerifier adapts a gitlab.Signer to the TokenVerifier interface.
type SignerVerifier struct {
	Signer *gitlab.Signer
}

// Verify validates a token using the underlying gitlab signer.
func (s SignerVerifier) Verify(ctx context.Context, token string) (Principal, error) {
	if s.Signer == nil {
		return Principal{}, errors.New("gitlab signer verifier: signer not configured")
	}
	claims, err := s.Signer.ValidateToken(ctx, token)
	if err != nil {
		return Principal{}, err
	}
	return Principal{
		SecretName: claims.SecretName,
		TokenID:    claims.TokenID,
		Scopes:     claims.Scopes,
		IssuedAt:   claims.IssuedAt,
		ExpiresAt:  claims.ExpiresAt,
	}, nil
}
