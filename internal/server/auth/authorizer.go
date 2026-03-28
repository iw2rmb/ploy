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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5"
)

// Role represents an authentication role for access control.
type Role string

// Role constants encode connection-level privileges.
const (
	RoleControlPlane Role = "control-plane"
	RoleWorker       Role = "worker"
	RoleCLIAdmin     Role = "cli-admin"
)

// roleIncludes defines the role hierarchy. Each key role is implicitly
// granted access to routes that allow any of its included roles.
// For example, cli-admin is a superset of control-plane.
var roleIncludes = map[Role][]Role{
	RoleCLIAdmin: {RoleControlPlane},
}

// Options configure the Authorizer.
//
// When AllowInsecure is true, requests without TLS are allowed and DefaultRole
// is assigned as the caller's role. In secure deployments, set AllowInsecure
// to false so that mutual TLS is mandatory.
type Options struct {
	AllowInsecure bool
	DefaultRole   Role
	TokenSecret   string        // JWT signing secret for bearer token validation
	Querier       store.Querier // Database querier for token validation
	Logger        *slog.Logger  // Structured logger for auth events
}

// Authorizer enforces role-based access derived from client certificates.
// Use Middleware to wrap HTTP handlers with the required role allowlist.
type Authorizer struct {
	allowInsecure bool
	defaultRole   Role
	tokenSecret   string         // JWT signing secret
	querier       store.Querier  // Database for token validation
	logger        *slog.Logger   // Structured logger
	wg            sync.WaitGroup // tracks in-flight background work (e.g. token last-used updates)
}

// Identity describes the caller extracted from the TLS certificate.
type Identity struct {
	Role       Role
	CommonName string
	Serial     string
}

type identityKey struct{}
type queryTokenAllowedKey struct{}

// WithQueryTokenAllowed wraps a handler to indicate that query-parameter
// token authentication is permitted on this route. The flag is read by
// identityFromRequest to decide whether to accept auth_token query params.
func WithQueryTokenAllowed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), queryTokenAllowedKey{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isQueryTokenAllowed(r *http.Request) bool {
	v, _ := r.Context().Value(queryTokenAllowedKey{}).(bool)
	return v
}

// NewAuthorizer constructs an Authorizer.
func NewAuthorizer(opts Options) *Authorizer {
	role := opts.DefaultRole
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Authorizer{
		allowInsecure: opts.AllowInsecure,
		defaultRole:   role,
		tokenSecret:   opts.TokenSecret,
		querier:       opts.Querier,
		logger:        logger,
	}
}

// Wait blocks until all in-flight background work (e.g. token last-used
// updates) completes. Call during server shutdown for graceful drain.
func (a *Authorizer) Wait() { a.wg.Wait() }

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

// ContextWithIdentity returns a new context with the given identity attached.
// This is primarily intended for testing handlers that require caller identity.
func ContextWithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// Middleware enforces the provided role allowlist (empty slice permits any role while still requiring TLS).
func (a *Authorizer) Middleware(allowed ...Role) func(http.Handler) http.Handler {
	normalized := allowlist(allowed)
	// Expand the allowlist using the role hierarchy so that superroles
	// (e.g. cli-admin) are automatically permitted on routes that allow
	// any of their included roles (e.g. control-plane).
	for superRole, includes := range roleIncludes {
		for _, included := range includes {
			if _, ok := normalized[included]; ok {
				normalized[superRole] = struct{}{}
			}
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
				a.logger.Warn("auth: request denied",
					"method", r.Method,
					"path", r.URL.Path,
					"error", err)
				http.Error(w, "authentication failed", http.StatusUnauthorized)
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
			a.logger.Debug("auth: attempting bearer token authentication",
				"method", r.Method,
				"path", r.URL.Path,
				"token_prefix", token[:min(8, len(token))])
			return a.identityFromBearerToken(r.Context(), token)
		}
	}

	// Fallback for browser/OSC8 artifact links:
	// allow auth_token query parameter on routes registered with
	// RegisterRouteFuncAllowQueryToken (indicated via context flag).
	if r.Method == http.MethodGet && isQueryTokenAllowed(r) {
		if token := strings.TrimSpace(r.URL.Query().Get("auth_token")); token != "" {
			a.logger.Debug("auth: attempting query token authentication",
				"method", r.Method,
				"path", r.URL.Path,
				"token_prefix", token[:min(8, len(token))])
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
			a.logger.Debug("auth: using insecure mode default role",
				"method", r.Method,
				"path", r.URL.Path,
				"role", role)
			return Identity{Role: role}, nil
		}
		a.logger.Warn("auth: authentication required but no credentials provided",
			"method", r.Method,
			"path", r.URL.Path)
		return Identity{}, errors.New("authentication required: provide Bearer token")
	}
	cert := r.TLS.PeerCertificates[0]
	role := extractRole(cert)
	if role == "" {
		a.logger.Warn("auth: certificate missing role claim",
			"method", r.Method,
			"path", r.URL.Path,
			"cn", cert.Subject.CommonName)
		return Identity{}, errors.New("auth: certificate missing role claim")
	}
	a.logger.Debug("auth: authenticated via mTLS",
		"method", r.Method,
		"path", r.URL.Path,
		"role", role,
		"cn", cert.Subject.CommonName)
	return Identity{
		Role:       role,
		CommonName: cert.Subject.CommonName,
		Serial:     cert.SerialNumber.String(),
	}, nil
}

func extractRole(cert *x509.Certificate) Role {
	if cert == nil {
		return ""
	}
	for _, ou := range cert.Subject.OrganizationalUnit {
		ouStr := strings.TrimPrefix(strings.TrimSpace(ou), "Ploy")
		ouStr = strings.TrimPrefix(strings.TrimSpace(ouStr), "role=")
		if candidate := NormalizeRole(ouStr); candidate != "" {
			return candidate
		}
	}
	if cn := strings.TrimSpace(cert.Subject.CommonName); cn != "" {
		roleStr := cn
		// Prefer colon used by nodes (e.g., "node:<node_id>")
		if idx := strings.Index(cn, ":"); idx > 0 {
			roleStr = cn[:idx]
		} else if idx := strings.Index(cn, "-"); idx > 0 {
			// Fallback to hyphen delimiter (e.g., "control-xyz").
			roleStr = cn[:idx]
		}
		if candidate := NormalizeRole(roleStr); candidate != "" {
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
		a.logger.Warn("auth: bearer token validation failed",
			"error", err.Error(),
			"token_prefix", tokenString[:min(8, len(tokenString))])
		return Identity{}, fmt.Errorf("invalid token: %w", err)
	}

	// Verify token is not expired
	if time.Now().After(claims.ExpiresAt.Time) {
		a.logger.Warn("auth: bearer token expired",
			"token_id", claims.ID,
			"token_type", claims.TokenType,
			"expired_at", claims.ExpiresAt.Time,
			"role", claims.Role)
		return Identity{}, errors.New("token expired")
	}

	// Check if token is revoked (query database)
	revoked, err := a.isTokenRevoked(ctx, claims.ID, claims.TokenType)
	if err != nil {
		a.logger.Error("auth: failed to check token revocation",
			"error", err.Error(),
			"token_id", claims.ID,
			"token_type", claims.TokenType)
		return Identity{}, fmt.Errorf("check token revocation: %w", err)
	}
	if revoked {
		a.logger.Warn("auth: bearer token revoked",
			"token_id", claims.ID,
			"token_type", claims.TokenType,
			"role", claims.Role)
		return Identity{}, errors.New("token revoked")
	}

	// Update last_used_at/used_at asynchronously for all token types.
	// Tracked via WaitGroup so Wait() can drain in-flight updates on shutdown.
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.updateTokenLastUsed(context.Background(), claims.ID, claims.TokenType)
	}()

	a.logger.Info("auth: bearer token validated successfully",
		"token_id", claims.ID,
		"token_type", claims.TokenType,
		"role", claims.Role,
		"cluster_id", claims.ClusterID)

	return Identity{
		Role:       Role(claims.Role),
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
		if errors.Is(err, pgx.ErrNoRows) {
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
		// Log the error but don't fail the request — token last-used updates
		// are telemetry-only and should not block authentication.
		a.logger.Warn("token last-used update failed",
			"token_type", tokenType,
			"token_id", tokenID,
			"err", err,
		)
	}
}

// NormalizeRole normalizes a role string to one of the standard role constants.
// It accepts common aliases and returns the canonical role name.
// Returns empty Role if the value doesn't match any known role.
//
// Aliases exist because certificate OUs and CNs may use different naming
// conventions depending on when they were issued:
//
//	control-plane: "beacon" (legacy agent name), "control", "controlplane",
//	               "client" (generic TLS client)
//	worker:        "node" (cert CN prefix for node agents)
//	cli-admin:     "cliadmin", "admin"
func NormalizeRole(value string) Role {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "beacon", "control", "control-plane", "controlplane", "client":
		return RoleControlPlane
	case "worker", "node":
		return RoleWorker
	case "cli-admin", "cliadmin", "admin":
		return RoleCLIAdmin
	default:
		return ""
	}
}

func allowlist(roles []Role) map[Role]struct{} {
	if len(roles) == 0 {
		return nil
	}
	out := make(map[Role]struct{}, len(roles))
	for _, role := range roles {
		if role != "" {
			out[role] = struct{}{}
		}
	}
	return out
}
