// Package auth provides role-based request authorization for the server.
//
// It derives caller identity from the client certificate presented over mTLS
// and enforces access via a simple role allowlist. The recognized roles are:
//
//   - RoleControlPlane: control-plane callers (CLI and automation)
//   - RoleWorker: node agents pushing heartbeats/logs/diffs/artifacts
//   - RoleCLIAdmin: privileged CLI operations (e.g., PKI endpoints)
//
// Identity is extracted from the client certificate Subject OU(s) or, as a
// fallback, the Subject CN. Common aliases are normalized (e.g., "admin",
// "cliadmin" → RoleCLIAdmin; "control", "controlplane" → RoleControlPlane).
//
// When AllowInsecure is enabled in Options, the middleware permits plaintext
// requests and assigns DefaultRole. This is intended strictly for local tests
// and insecure development flows; production should require mTLS.
package auth

import (
	"context"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

// Role constants encode connection-level privileges.
const (
	RoleControlPlane = "control-plane"
	RoleWorker       = "worker"
	RoleCLIAdmin     = "cli-admin"
)

// Options configure the Authorizer.
//
// When AllowInsecure is true, requests without TLS are allowed and DefaultRole
// is assigned as the caller's role. In secure deployments, set AllowInsecure
// to false so that mutual TLS is mandatory.
type Options struct {
	AllowInsecure bool
	DefaultRole   string
	TokenSecret   string        // JWT signing secret for bearer token validation
	Querier       store.Querier // Database querier for token validation
}

// Authorizer enforces role-based access derived from client certificates.
// Use Middleware to wrap HTTP handlers with the required role allowlist.
type Authorizer struct {
	allowInsecure bool
	defaultRole   string
	tokenSecret   string        // JWT signing secret
	querier       store.Querier // Database for token validation
}

// Identity describes the caller extracted from the TLS certificate.
type Identity struct {
	Role       string
	CommonName string
	Serial     string
}

type identityKey struct{}

// NewAuthorizer constructs an Authorizer.
func NewAuthorizer(opts Options) *Authorizer {
	role := NormalizeRole(opts.DefaultRole)
	if role == "" {
		role = opts.DefaultRole
	}
	return &Authorizer{
		allowInsecure: opts.AllowInsecure,
		defaultRole:   role,
		tokenSecret:   opts.TokenSecret,
		querier:       opts.Querier,
	}
}

// IdentityFromContext returns the previously extracted identity, if any.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	if ctx == nil {
		return Identity{}, false
	}
	value := ctx.Value(identityKey{})
	if value == nil {
		return Identity{}, false
	}
	identity, ok := value.(Identity)
	return identity, ok
}

// Middleware enforces the provided role allowlist (empty slice permits any role while still requiring TLS).
func (a *Authorizer) Middleware(allowed ...string) func(http.Handler) http.Handler {
	normalized := allowlist(allowed)
	// Treat cli-admin as a superset of control-plane for authorization purposes.
	// If a route allows control-plane, cli-admin should also be allowed.
	if normalized != nil {
		if _, ok := normalized[RoleControlPlane]; ok {
			normalized[RoleCLIAdmin] = struct{}{}
		}
	}
	return func(next http.Handler) http.Handler {
		if next == nil {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			})
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, err := a.identityFromRequest(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			if len(normalized) > 0 {
				if _, ok := normalized[identity.Role]; !ok {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
			ctx := context.WithValue(r.Context(), identityKey{}, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (a *Authorizer) identityFromRequest(r *http.Request) (Identity, error) {
	// Try bearer token authentication first
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			return a.identityFromBearerToken(r.Context(), token)
		}
	}

	// Fall back to mTLS authentication
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		if a.allowInsecure {
			role := a.defaultRole
			if role == "" {
				role = RoleControlPlane
			}
			return Identity{Role: role}, nil
		}
		return Identity{}, errors.New("authentication required: provide Bearer token")
	}
	cert := r.TLS.PeerCertificates[0]
	role := extractRole(cert)
	if role == "" {
		return Identity{}, errors.New("auth: certificate missing role claim")
	}
	return Identity{
		Role:       role,
		CommonName: cert.Subject.CommonName,
		Serial:     cert.SerialNumber.String(),
	}, nil
}

func extractRole(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	for _, ou := range cert.Subject.OrganizationalUnit {
		role := NormalizeRole(strings.TrimPrefix(strings.TrimSpace(ou), "Ploy"))
		role = strings.TrimSpace(strings.TrimPrefix(role, "role="))
		if candidate := NormalizeRole(role); candidate != "" {
			return candidate
		}
	}
	if cn := strings.TrimSpace(cert.Subject.CommonName); cn != "" {
		role := cn
		// Prefer colon used by nodes (e.g., "node:<uuid>")
		if idx := strings.Index(cn, ":"); idx > 0 {
			role = cn[:idx]
		} else if idx := strings.Index(cn, "-"); idx > 0 {
			// Fallback to hyphen delimiter (e.g., "control-xyz").
			role = cn[:idx]
		}
		if candidate := NormalizeRole(role); candidate != "" {
			return candidate
		}
	}
	return ""
}

// identityFromBearerToken validates a JWT bearer token and extracts the identity.
func (a *Authorizer) identityFromBearerToken(ctx context.Context, tokenString string) (Identity, error) {
	// Validate JWT signature and extract claims
	claims, err := ValidateToken(tokenString, a.tokenSecret)
	if err != nil {
		return Identity{}, fmt.Errorf("invalid token: %w", err)
	}

	// Verify token is not expired
	if time.Now().After(claims.ExpiresAt.Time) {
		return Identity{}, errors.New("token expired")
	}

	// Check if token is revoked (query database)
	revoked, err := a.isTokenRevoked(ctx, claims.ID, claims.TokenType)
	if err != nil {
		return Identity{}, fmt.Errorf("check token revocation: %w", err)
	}
	if revoked {
		return Identity{}, errors.New("token revoked")
	}

	// Update last_used_at timestamp (async, don't block request)
	go a.updateTokenLastUsed(context.Background(), claims.ID, claims.TokenType)

	return Identity{
		Role:       claims.Role,
		CommonName: claims.ID, // Use token ID as identifier
		// ClusterID is in claims but not in Identity struct yet
	}, nil
}

// isTokenRevoked checks if a token has been revoked by querying the database.
func (a *Authorizer) isTokenRevoked(ctx context.Context, tokenID, tokenType string) (bool, error) {
	if a.querier == nil {
		// If no database configured, tokens cannot be revoked
		return false, nil
	}

	var err error
	switch tokenType {
	case TokenTypeAPI:
		_, err = a.querier.CheckAPITokenRevoked(ctx, tokenID)
	case TokenTypeBootstrap:
		_, err = a.querier.CheckBootstrapTokenRevoked(ctx, tokenID)
	default:
		return false, fmt.Errorf("unknown token type: %s", tokenType)
	}

	if err != nil {
		// If the query returns no rows, the token is not revoked
		if err == sql.ErrNoRows {
			return false, nil
		}
		// Check for pgx "no rows" error
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "02000" {
			return false, nil
		}
		return false, fmt.Errorf("query token revocation: %w", err)
	}

	// If we got a result, the token is revoked
	return true, nil
}

// updateTokenLastUsed updates the last_used_at timestamp for a token.
// This runs asynchronously and does not block the request.
func (a *Authorizer) updateTokenLastUsed(ctx context.Context, tokenID, tokenType string) {
	if a.querier == nil {
		return
	}

	var err error
	switch tokenType {
	case TokenTypeAPI:
		err = a.querier.UpdateAPITokenLastUsed(ctx, tokenID)
	case TokenTypeBootstrap:
		err = a.querier.UpdateBootstrapTokenLastUsed(ctx, tokenID)
	}

	if err != nil {
		// Log error but don't fail the request
		// TODO: Add proper logging
		_ = err
	}
}

// NormalizeRole normalizes a role string to one of the standard role constants.
// It accepts common aliases and returns the canonical role name.
func NormalizeRole(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "beacon", "control", "control-plane", "controlplane", "client":
		return RoleControlPlane
	case "worker", "node":
		return RoleWorker
	case "cli-admin", "cliadmin", "admin":
		return RoleCLIAdmin
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func allowlist(roles []string) map[string]struct{} {
	if len(roles) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		if normalized := NormalizeRole(role); normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out
}
